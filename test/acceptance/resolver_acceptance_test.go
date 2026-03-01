//go:build acceptance

package acceptance

import (
	"strings"
	"testing"
	"time"
)

const resolverTimeout = 90 * time.Second

// TestResolveByTag verifies the tag-resolver strategy against the real EC2 API.
// Uses the shell command because ssh-direct's user@host[:port] target format
// conflicts with the tag Key:Value format. The shell command passes the raw
// target to ResolveTarget which handles tags correctly.
// We verify that the process does NOT fail with a resolution error.
func TestResolveByTag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	// The shell command in non-TTY mode doesn't cleanly terminate SSM sessions,
	// so we skip the leak checker and clean up sessions after the test instead.
	t.Cleanup(func() { terminateAllSessions(t, i.InstanceID) })

	target := i.AliasTagKey + ":" + i.AliasTagValue
	_, stderr, code := runCmd(t, resolverTimeout, "shell", target)
	// The shell command will likely fail because stdin is not a TTY, but that's OK.
	// We only care that target resolution succeeded (no resolution error in stderr).
	if code != 0 {
		lower := strings.ToLower(stderr)
		if strings.Contains(lower, "no instances") || strings.Contains(lower, "could not resolve") ||
			strings.Contains(lower, "not found") {
			t.Fatalf("tag resolver failed: %s", stderr)
		}
		// Non-zero exit from TTY/session issue is acceptable — resolution worked.
		t.Logf("shell exited %d (expected for non-TTY stdin); stderr: %s", code, stderr)
	}
}

// TestResolveByIP verifies the IP-resolver strategy against the real EC2 API.
// Uses ssh-direct since IP addresses don't conflict with host:port parsing.
func TestResolveByIP(t *testing.T) {
	i := infra(t)
	if i.InstancePrivateIP == "" {
		t.Skip("instance_private_ip not set in infra outputs")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstancePrivateIP
	stdout, stderr, code := runCmdWithRetry(t, resolverTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("IP resolver: ssh-direct exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("IP resolver: expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestResolveMultipleInstances expects an error when a tag matches more than one instance.
// Skipped unless TF_OUTPUT_MULTI_TAG_KEY / TF_OUTPUT_MULTI_TAG_VALUE env vars are set.
func TestResolveMultipleInstances(t *testing.T) {
	tagKey := requireEnv(t, "TF_OUTPUT_MULTI_TAG_KEY")
	tagVal := requireEnv(t, "TF_OUTPUT_MULTI_TAG_VALUE")

	target := tagKey + ":" + tagVal
	_, stderr, code := runCmd(t, resolverTimeout, "shell", target)

	if code == 0 {
		t.Fatal("expected non-zero exit for multi-instance tag, got 0")
	}
	lower := strings.ToLower(stderr)
	if !strings.Contains(lower, "multiple") {
		t.Errorf("expected 'multiple' in error output\nstderr:\n%s", stderr)
	}
}

// TestResolveNotFound expects an error when the target cannot be resolved.
func TestResolveNotFound(t *testing.T) {
	_, stderr, code := runCmd(t, 30*time.Second, "shell", "Name:does-not-exist-9f3a2b1c")
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown target, got 0")
	}
	if stderr == "" {
		t.Error("expected error message on stderr for unknown target, got nothing")
	}
}
