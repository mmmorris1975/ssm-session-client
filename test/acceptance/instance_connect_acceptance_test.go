//go:build acceptance

package acceptance

import (
	"strings"
	"testing"
)

// TestInstanceConnectPushKey verifies that pushing an ephemeral public key via
// EC2 Instance Connect succeeds (using the SDK helper, not the CLI command which
// starts a full SSH session).
func TestInstanceConnectPushKey(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)

	_, pubKeyPath := generateTempKeyPair(t)

	// Use the SDK directly to push the key — this does NOT open an SSM session.
	pushInstanceConnectKey(t, i, pubKeyPath)
}

// TestInstanceConnectThenSSHDirect verifies the full Instance Connect flow:
// the CLI generates an ephemeral key, pushes it via EC2 Instance Connect,
// and uses it for SSH authentication — all in one atomic --instance-connect call.
func TestInstanceConnectThenSSHDirect(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstanceID
	stdout, stderr, code := runCmdWithRetry(t, sshDirectTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "id", target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --instance-connect exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectUser) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectUser, stdout, stderr)
	}
}
