//go:build acceptance

// Package acceptance contains end-to-end acceptance tests for ssm-session-client.
// Run with: go test ./test/acceptance/... -tags acceptance -v -timeout 20m
package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
)

// InfraOutputs holds the values from test/infra/outputs.json (or TF_OUTPUT_* env vars).
type InfraOutputs struct {
	InstanceID        string `json:"instance_id"`
	InstancePrivateIP string `json:"instance_private_ip"`
	InstanceTagName   string `json:"instance_tag_name"`
	AWSRegion         string `json:"aws_region"`
	AliasTagKey       string `json:"test_alias_tag_key"`
	AliasTagValue     string `json:"test_alias_tag_value"`
	DNSHostname       string `json:"dns_hostname"`
	KMSKeyARN         string `json:"kms_key_arn"`
	// Windows / RDP fields (populated when create_windows_instance=true).
	WindowsInstanceID string `json:"windows_instance_id"`
	RDPKeyPairFile    string `json:"rdp_key_pair_file"`
}

var (
	// globalInfraOutputs is populated once in TestMain.
	globalInfraOutputs InfraOutputs
	// binaryPath is the path to the compiled ssm-session-client binary.
	binaryPath string
)

// TestMain builds the binary and loads infrastructure outputs before running tests.
func TestMain(m *testing.M) {
	var err error

	globalInfraOutputs, err = loadInfraOutputs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: load infra outputs: %v\n", err)
		os.Exit(1)
	}

	binaryPath, err = buildBinaryOnce()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: build binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Remove(binaryPath) //nolint:errcheck
	os.Exit(code)
}

// infra returns the global InfraOutputs, failing the test if the instance ID is empty.
func infra(t *testing.T) InfraOutputs {
	t.Helper()
	if globalInfraOutputs.InstanceID == "" {
		t.Fatal("infra outputs not loaded: instance_id is empty")
	}
	return globalInfraOutputs
}

// loadInfraOutputs reads infra details from TF_OUTPUT_* env vars or outputs.json.
func loadInfraOutputs() (InfraOutputs, error) {
	if id := os.Getenv("TF_OUTPUT_INSTANCE_ID"); id != "" {
		return infraFromEnv(), nil
	}

	_, thisFile, _, _ := runtime.Caller(0)
	outputsPath := filepath.Join(filepath.Dir(thisFile), "..", "infra", "outputs.json")
	if p := os.Getenv("ACCEPTANCE_OUTPUTS"); p != "" {
		outputsPath = p
	}

	data, err := os.ReadFile(outputsPath)
	if err != nil {
		return InfraOutputs{}, fmt.Errorf("no TF_OUTPUT_* env vars and cannot read %s: %w", outputsPath, err)
	}

	var out InfraOutputs
	if err := json.Unmarshal(data, &out); err != nil {
		return InfraOutputs{}, fmt.Errorf("parse %s: %w", outputsPath, err)
	}
	return out, nil
}

// infraFromEnv constructs InfraOutputs from TF_OUTPUT_* environment variables.
func infraFromEnv() InfraOutputs {
	return InfraOutputs{
		InstanceID:        os.Getenv("TF_OUTPUT_INSTANCE_ID"),
		InstancePrivateIP: os.Getenv("TF_OUTPUT_INSTANCE_PRIVATE_IP"),
		InstanceTagName:   os.Getenv("TF_OUTPUT_INSTANCE_TAG_NAME"),
		AWSRegion:         os.Getenv("TF_OUTPUT_AWS_REGION"),
		AliasTagKey:       os.Getenv("TF_OUTPUT_TEST_ALIAS_TAG_KEY"),
		AliasTagValue:     os.Getenv("TF_OUTPUT_TEST_ALIAS_TAG_VALUE"),
		DNSHostname:       os.Getenv("TF_OUTPUT_DNS_HOSTNAME"),
		KMSKeyARN:         os.Getenv("TF_OUTPUT_KMS_KEY_ARN"),
		WindowsInstanceID: os.Getenv("TF_OUTPUT_WINDOWS_INSTANCE_ID"),
		RDPKeyPairFile:    os.Getenv("TF_OUTPUT_RDP_KEY_PAIR_FILE"),
	}
}

// buildBinaryOnce compiles the CLI binary into a temp file and returns its path.
func buildBinaryOnce() (string, error) {
	name := "ssm-session-client-acceptance-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(os.TempDir(), name)

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	cmd := exec.Command("go", "build", "-o", out, ".") //nolint:gosec
	cmd.Dir = repoRoot

	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %w\n%s", err, output)
	}
	return out, nil
}

// runCmd executes the test binary with the given arguments and an implicit --aws-region.
func runCmd(t *testing.T, timeout time.Duration, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fullArgs := append([]string{"--aws-region", globalInfraOutputs.AWSRegion}, args...)
	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...) //nolint:gosec

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

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

// runCmdWithRetry is like runCmd but retries once after a brief delay when the
// command fails with "SSM handshake failed: EOF". This transient error occurs
// when the SSM agent is temporarily at its session limit.
func runCmdWithRetry(t *testing.T, timeout time.Duration, args ...string) (string, string, int) {
	t.Helper()
	stdout, stderr, code := runCmd(t, timeout, args...)
	if code != 0 && strings.Contains(stderr, "SSM handshake failed: EOF") {
		t.Log("retrying after SSM handshake EOF (transient SSM agent issue)...")
		time.Sleep(5 * time.Second)
		stdout, stderr, code = runCmd(t, timeout, args...)
	}
	return stdout, stderr, code
}

// freePort returns an available TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// waitForPort polls until a TCP listener appears on the given port or the deadline passes.
// It connects to "localhost" (not "127.0.0.1") so it works regardless of whether the
// listener bound to IPv4 (127.0.0.1) or IPv6 (::1).
func waitForPort(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("localhost:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("port %d not available after %s", port, timeout)
}

// requireEnv returns the value of the named env var or skips the test if unset.
func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("env var %s not set; skipping test", key)
	}
	return v
}

// pushKeyViaSDK pushes a public key to EC2 Instance Connect using the AWS SDK
// directly. This avoids spawning the CLI binary (which starts a full SSH session).
func pushKeyViaSDK(t *testing.T, instanceID, user, pubKeyContent string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Fatalf("pushKeyViaSDK: load AWS config: %v", err)
	}
	client := ec2instanceconnect.NewFromConfig(cfg)
	_, err = client.SendSSHPublicKey(ctx, &ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(instanceID),
		InstanceOSUser: aws.String(user),
		SSHPublicKey:   aws.String(pubKeyContent),
	})
	if err != nil {
		t.Fatalf("pushKeyViaSDK: SendSSHPublicKey: %v", err)
	}
}

