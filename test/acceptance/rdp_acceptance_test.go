//go:build acceptance && windows

// RDP acceptance tests: Windows-only because the rdp subcommand only exists in Windows builds
// and requires mstsc.exe.  Run these on a Windows GitHub Actions runner or a local Windows box.
package acceptance

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

const rdpTimeout = 120 * time.Second

// TestRDPTunnelEstablished starts the rdp command with a fixed local port, waits until
// the SSM tunnel is listening, verifies a TCP connection succeeds, then cancels the process.
// mstsc.exe is killed along with the parent process via exec.CommandContext.
func TestRDPTunnelEstablished(t *testing.T) {
	i := infraWindows(t)
	waitForSSMReady(t, i.WindowsInstanceID)
	registerSessionLeakCheck(t, i.WindowsInstanceID)

	localPort := freePort(t)
	cancel := startRDPCommand(t, i, localPort, false)
	defer cancel()

	waitForPort(t, localPort, rdpTimeout)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("TCP connect to RDP tunnel on port %d: %v", localPort, err)
	}
	conn.Close()
}

// TestRDPGetPassword verifies that --get-password retrieves the EC2 administrator password
// and the tunnel becomes available.  Skipped if RDPKeyPairFile is not set in infra outputs.
//
// Note: EC2 password generation takes up to 15 min after first launch; this test waits for it.
func TestRDPGetPassword(t *testing.T) {
	i := infraWindows(t)
	if i.RDPKeyPairFile == "" {
		t.Skip("rdp_key_pair_file not set in infra outputs; skipping --get-password test")
	}
	waitForSSMReady(t, i.WindowsInstanceID)
	registerSessionLeakCheck(t, i.WindowsInstanceID)
	waitForEC2Password(t, i.WindowsInstanceID, i.AWSRegion)

	localPort := freePort(t)
	cancel := startRDPCommand(t, i, localPort, true)
	defer cancel()

	waitForPort(t, localPort, rdpTimeout)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("TCP connect to RDP tunnel with --get-password on port %d: %v", localPort, err)
	}
	conn.Close()
}

// infraWindows returns infra outputs and fails the test if no Windows instance is configured.
func infraWindows(t *testing.T) InfraOutputs {
	t.Helper()
	i := infra(t)
	if i.WindowsInstanceID == "" {
		t.Skip("windows_instance_id not set in infra outputs (set create_windows_instance=true in Terraform)")
	}
	return i
}

// startRDPCommand launches ssm-session-client rdp in the background with a fixed local port.
// When getPassword is true it also passes --get-password and --key-pair-file.
func startRDPCommand(t *testing.T, i InfraOutputs, localPort int, getPassword bool) context.CancelFunc {
	t.Helper()
	args := []string{
		"--aws-region", i.AWSRegion,
		"rdp",
		"--local-port", strconv.Itoa(localPort),
		i.WindowsInstanceID,
	}
	if getPassword {
		args = append(args, "--get-password", "--key-pair-file", i.RDPKeyPairFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start rdp command: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		}
		cancel()
		cmd.Wait() //nolint:errcheck
	})

	return cancel
}

// waitForEC2Password polls GetPasswordData until the password is available or times out.
// EC2 Windows instances generate their administrator password within 4–15 minutes of launch.
func waitForEC2Password(t *testing.T, instanceID, region string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		t.Fatalf("load AWS config for EC2 password check: %v", err)
	}
	client := ec2.NewFromConfig(cfg)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	t.Log("waiting for EC2 Windows password to be generated (may take up to 15 min)...")
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for EC2 password on %s", instanceID)
		case <-ticker.C:
			out, err := client.GetPasswordData(ctx, &ec2.GetPasswordDataInput{
				InstanceId: &instanceID,
			})
			if err != nil {
				t.Logf("GetPasswordData error: %v", err)
				continue
			}
			if out.PasswordData != nil && *out.PasswordData != "" {
				t.Logf("EC2 Windows password is ready for %s", instanceID)
				return
			}
		}
	}
}
