//go:build acceptance

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	// ssoTimeout covers SSO credential retrieval from cache + SSM session setup.
	ssoTimeout = 5 * time.Minute
	// ssoInteractiveTimeout is the default wait for a human to complete the browser flow.
	ssoInteractiveTimeout = 10 * time.Minute
)

// awsCredentialEnvKeys lists the AWS SDK environment variables that carry static
// credentials. These take precedence over the profile credential chain and must be
// absent when testing SSO-based authentication.
var awsCredentialEnvKeys = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_SESSION_EXPIRATION",
}

// ssoEnv returns the current process environment with static credential env vars
// removed and AWS_PROFILE forced to the given SSO profile. This ensures the binary
// cannot fall back to ambient credentials and must resolve credentials via SSO.
func ssoEnv(profile string) []string {
	remove := make(map[string]bool, len(awsCredentialEnvKeys))
	for _, k := range awsCredentialEnvKeys {
		remove[k] = true
	}
	remove["AWS_PROFILE"] = true // will be re-added explicitly below

	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !remove[key] {
			env = append(env, kv)
		}
	}
	env = append(env, "AWS_PROFILE="+profile)
	return env
}

// TestSSOLoginWithCachedToken verifies that the --sso-login and --aws-profile flags
// authenticate via SSO and successfully establish an SSM session.
//
// The test assumes valid SSO credentials are already cached (e.g. after running
// "aws sso login --profile <profile>" beforehand). If credentials are expired or
// absent the binary will attempt the device-code flow and the test will fail when
// the timeout elapses without user interaction.
//
// Required env vars:
//   - SSC_SSO_TEST_PROFILE: name of an AWS CLI profile with SSO configuration.
func TestSSOLoginWithCachedToken(t *testing.T) {
	profile := requireEnv(t, "SSC_SSO_TEST_PROFILE")
	// Set AWS_PROFILE early so that waitForSSMReady and registerSessionLeakCheck
	// use the SSO profile's cached credentials rather than ambient credentials.
	t.Setenv("AWS_PROFILE", profile)
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, ssoTimeout,
		"--sso-login", "--aws-profile", profile,
		"ssh-direct", "--no-host-key-check", "--instance-connect",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("SSO ssh-direct exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestSSOLoginInteractive performs a full interactive SSO device-code login.
// The binary prints the verification URL to stderr; the tester must open it in a
// browser and approve the request before the timeout expires. The URL appears
// immediately in the test output because stderr is streamed in real time.
//
// Note: --sso-open-browser is intentionally NOT used here because browser
// launching is unreliable when the binary runs as a subprocess of go test.
//
// This test is intentionally excluded from routine CI runs. Enable it only for
// manual runs that have a human available to complete the browser flow.
//
// Required env vars:
//   - SSC_SSO_TEST_PROFILE:     name of an AWS CLI profile with SSO configuration.
//   - SSC_SSO_TEST_INTERACTIVE: any non-empty value enables this test.
//
// Optional env vars:
//   - SSC_SSO_TEST_TIMEOUT: wait budget in minutes (default 10).
func TestSSOLoginInteractive(t *testing.T) {
	profile := requireEnv(t, "SSC_SSO_TEST_PROFILE")
	requireEnv(t, "SSC_SSO_TEST_INTERACTIVE")

	i := infra(t)

	timeout := ssoInteractiveTimeout
	if v := os.Getenv("SSC_SSO_TEST_TIMEOUT"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins > 0 {
			timeout = time.Duration(mins) * time.Minute
		}
	}

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)

	// Phase 1: complete the SSO browser flow.
	// The URL is streamed to stderr in real time — open it in a browser and approve.
	// Once authenticated the token is written to the local SSO cache.
	t.Logf("Phase 1: SSO authentication for profile %q (timeout %s)", profile, timeout)
	t.Log("Open the URL printed to stderr in your browser to complete authentication.")
	_, authStderr, authCode := runCmdStreaming(t, timeout, ssoEnv(profile),
		"--sso-login", "--aws-profile", profile,
		"ssh-direct", "--no-host-key-check", "--instance-connect",
		"--exec", "echo sso_auth_ok", target,
	)
	if authCode != 0 {
		t.Fatalf("SSO authentication phase failed (exit %d):\n%s", authCode, authStderr)
	}

	// Phase 2: token is now cached; perform pre-flight checks using those credentials.
	t.Setenv("AWS_PROFILE", profile)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Phase 3: run the assertion command with the cached token (no browser prompt).
	stdout, stderr, code := runCmdStreaming(t, ssoTimeout, ssoEnv(profile),
		"--sso-login", "--aws-profile", profile,
		"ssh-direct", "--no-host-key-check", "--instance-connect",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("SSO ssh-direct exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// runCmdStreaming is like runCmd but tees stderr to os.Stderr in real time so
// that interactive tests (e.g. SSO device-code flow) can display the
// authorization URL immediately without waiting for the command to finish.
// env overrides the subprocess environment; pass nil to inherit the current env.
func runCmdStreaming(t *testing.T, timeout time.Duration, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fullArgs := append([]string{"--aws-region", globalInfraOutputs.AWSRegion}, args...)
	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...) //nolint:gosec
	if env != nil {
		cmd.Env = env
	}

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

	runErr := cmd.Run()
	stdout, stderr = outBuf.String(), errBuf.String()

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return stdout, stderr, exitErr.ExitCode()
		}
		return stdout, stderr, -1
	}
	return stdout, stderr, 0
}
