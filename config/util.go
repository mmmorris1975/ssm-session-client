package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"go.uber.org/zap"
)

func FindSSHPublicKey() (string, error) {
	var pubKeyPath string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	pubKeyPaths := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
	}
	if singleFlags.SSHPublicKeyFile != "" {
		pubKeyPaths = append([]string{singleFlags.SSHPublicKeyFile}, pubKeyPaths...)
	}

	for _, path := range pubKeyPaths {
		if _, err := os.Stat(path); err == nil {
			pubKeyPath = path
			zap.S().Info("Found SSH public key at:", pubKeyPath)
			break
		}
	}

	if pubKeyPath == "" {
		return "", fmt.Errorf("no SSH public key found")
	}

	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}

	return string(pubKey), nil
}

// FindSSHPrivateKey searches for an SSH private key file. If overridePath is
// non-empty it is checked first. Otherwise it checks SSHKeyFile from config,
// then the standard ~/.ssh/id_ed25519 and ~/.ssh/id_rsa paths.
func FindSSHPrivateKey(overridePath string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var paths []string
	if overridePath != "" {
		paths = append(paths, overridePath)
	}
	if singleFlags.SSHKeyFile != "" {
		paths = append(paths, singleFlags.SSHKeyFile)
	}
	paths = append(paths,
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_rsa"),
	)

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			zap.S().Info("Found SSH private key at:", path)
			return path, nil
		}
	}

	return "", fmt.Errorf("no SSH private key found")
}

func IsSSMSessionManagerPluginInstalled() bool {
	pluginPath, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		zap.S().Info("Session Manager Plugin is not installed.")
		return false
	}
	zap.S().Info("Session Manager Plugin found at: ", pluginPath)
	return true
}
