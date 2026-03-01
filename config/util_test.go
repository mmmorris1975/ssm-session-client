package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSSHPublicKey_DefaultPaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	// Check if any default key exists
	ed25519Path := filepath.Join(homeDir, ".ssh", "id_ed25519.pub")
	rsaPath := filepath.Join(homeDir, ".ssh", "id_rsa.pub")

	hasEd25519 := fileExists(ed25519Path)
	hasRSA := fileExists(rsaPath)

	if !hasEd25519 && !hasRSA {
		t.Skip("no SSH public keys found at default paths")
	}

	key, err := FindSSHPublicKey()
	if err != nil {
		t.Fatalf("FindSSHPublicKey() error: %v", err)
	}
	if key == "" {
		t.Error("FindSSHPublicKey() returned empty key")
	}
}

func TestFindSSHPublicKey_CustomPath(t *testing.T) {
	// Create a temp SSH key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pub")
	keyContent := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG... test@example.com\n"

	if err := os.WriteFile(keyPath, []byte(keyContent), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Save and restore singleton state
	oldKeyFile := singleFlags.SSHPublicKeyFile
	singleFlags.SSHPublicKeyFile = keyPath
	defer func() { singleFlags.SSHPublicKeyFile = oldKeyFile }()

	key, err := FindSSHPublicKey()
	if err != nil {
		t.Fatalf("FindSSHPublicKey() error: %v", err)
	}
	if key != keyContent {
		t.Errorf("FindSSHPublicKey() = %q, want %q", key, keyContent)
	}
}

func TestFindSSHPublicKey_NonExistentCustomPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	// Check if default keys exist (they would be found as fallback)
	ed25519Path := filepath.Join(homeDir, ".ssh", "id_ed25519.pub")
	rsaPath := filepath.Join(homeDir, ".ssh", "id_rsa.pub")

	if fileExists(ed25519Path) || fileExists(rsaPath) {
		t.Skip("default SSH keys exist, fallback would succeed")
	}

	oldKeyFile := singleFlags.SSHPublicKeyFile
	singleFlags.SSHPublicKeyFile = "/nonexistent/path/key.pub"
	defer func() { singleFlags.SSHPublicKeyFile = oldKeyFile }()

	_, err = FindSSHPublicKey()
	if err == nil {
		t.Error("FindSSHPublicKey() should return error for nonexistent path with no fallback")
	}
}

func TestIsSSMSessionManagerPluginInstalled(t *testing.T) {
	// This test just verifies the function doesn't panic
	// The result depends on whether the plugin is installed
	_ = IsSSMSessionManagerPluginInstalled()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
