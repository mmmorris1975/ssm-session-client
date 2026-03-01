//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSSHProxyByInstanceID uses the system ssh(1) with ssm-session-client as a ProxyCommand.
// The test is skipped if the ssh binary is not found on PATH.
// An ephemeral Ed25519 key is pushed via EC2 Instance Connect so that ssh can authenticate.
func TestSSHProxyByInstanceID(t *testing.T) {
	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh binary not found on PATH; skipping proxy test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Push an ephemeral key so the system ssh client can authenticate.
	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	proxyCmd := fmt.Sprintf("%s --aws-region %s ssh %%h", binaryPath, i.AWSRegion)
	sshArgs := []string{
		"-i", keyPath,
		"-o", "ProxyCommand=" + proxyCmd,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
		fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID),
		"echo", sshDirectMarker,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, sshBin, sshArgs...) //nolint:gosec
	out, runErr := cmd.CombinedOutput()

	if runErr != nil {
		t.Fatalf("ssh proxy command failed: %v\noutput:\n%s", runErr, out)
	}
	if !strings.Contains(string(out), sshDirectMarker) {
		t.Errorf("expected %q in output\ngot:\n%s", sshDirectMarker, out)
	}
}
