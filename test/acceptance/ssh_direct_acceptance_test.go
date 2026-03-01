//go:build acceptance

package acceptance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshDirectTimeout = 120 * time.Second
	sshDirectUser    = "ec2-user"
	sshDirectMarker  = "sshdirect_acceptance_marker"
)

// TestSSHDirectInstanceConnect uses the --instance-connect flag for ephemeral key auth.
func TestSSHDirectInstanceConnect(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, sshDirectTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+sshDirectMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --instance-connect exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}
}

// TestSSHDirectCustomPort uses the user@host:port target format.
func TestSSHDirectCustomPort(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s:22", sshDirectUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, sshDirectTimeout,
		"ssh-direct", "--no-host-key-check", "--instance-connect",
		"--exec", "echo "+sshDirectMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct custom port exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}
}

// generateTempKeyPair creates a temporary Ed25519 key pair and returns (privKeyPath, pubKeyPath).
// Ed25519 is required — EC2 Instance Connect does not accept ECDSA P-256.
func generateTempKeyPair(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	privKeyPath := writeEd25519PrivKey(t, dir, priv)
	pubKeyPath := writeSSHPubKey(t, dir, pub)
	return privKeyPath, pubKeyPath
}

func writeEd25519PrivKey(t *testing.T, dir string, key ed25519.PrivateKey) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatalf("marshal ed25519 private key: %v", err)
	}
	path := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return path
}

func writeSSHPubKey(t *testing.T, dir string, pub ed25519.PublicKey) string {
	t.Helper()
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal ssh public key: %v", err)
	}
	path := filepath.Join(dir, "id_ed25519.pub")
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(sshPub), 0o644); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	return path
}

// pushInstanceConnectKey reads a public key file and pushes it to EC2 Instance Connect
// using the AWS SDK directly (avoids spawning the CLI which starts a full SSH session).
func pushInstanceConnectKey(t *testing.T, i InfraOutputs, pubKeyPath string) {
	t.Helper()
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("pushInstanceConnectKey: read public key %s: %v", pubKeyPath, err)
	}
	pushKeyViaSDK(t, i.InstanceID, sshDirectUser, string(data))
}
