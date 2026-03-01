package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var addProxyCommandCmd = &cobra.Command{
	Use:   "add-proxy-command [user@]target",
	Short: "Add SSH config entry with SSM proxy command",
	Long: `Add an entry to ~/.ssh/config that configures SSH to connect to the target
instance through ssm-session-client as a proxy command.

Examples:
  ssm-session-client ssh add-proxy-command ec2-user@i-0123456789abcdef0
  ssm-session-client ssh add-proxy-command ubuntu@i-0123456789abcdef0 --config /path/to/config.yaml`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := addProxyCommand(args[0]); err != nil {
			zap.S().Fatal(err)
		}
	},
}

func init() {
	ssmSshCmd.AddCommand(addProxyCommandCmd)
}

func addProxyCommand(target string) error {
	return upsertSSHConfigEntry(target, "ssh")
}

// parseUserHost extracts user and host from a [user@]host[:port] string.
func parseUserHost(target string) (user, host string) {
	user = "ec2-user"
	host = target

	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		user = parts[0]
		host = parts[1]
	}

	// Strip port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return user, host
}

// buildProxyCommand builds the ProxyCommand string for the given subcommand.
func buildProxyCommand(subcommand, user string) (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to determine executable path: %w", err)
	}

	if configFile != "" {
		return fmt.Sprintf("%s --config %s %s %s@%%h", execPath, configFile, subcommand, user), nil
	}
	return fmt.Sprintf("%s %s %s@%%h", execPath, subcommand, user), nil
}

// upsertSSHConfigEntry adds or updates an SSH config entry for the target
// using the given subcommand (e.g. "ssh" or "instance-connect") as the proxy.
func upsertSSHConfigEntry(target, subcommand string) error {
	user, host := parseUserHost(target)

	proxyCmd, err := buildProxyCommand(subcommand, user)
	if err != nil {
		return err
	}

	entry := fmt.Sprintf("# Added by ssm-session-client\nHost %s\n  User %s\n  ProxyCommand %s\n", host, user, proxyCmd)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	sshConfigPath := filepath.Join(homeDir, ".ssh", "config")

	// Ensure ~/.ssh directory exists
	sshDir := filepath.Dir(sshConfigPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", sshDir, err)
	}

	existing, err := os.ReadFile(sshConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", sshConfigPath, err)
	}

	updated := replaceOrAppendHostBlock(string(existing), host, entry)

	if err := os.WriteFile(sshConfigPath, []byte(updated), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", sshConfigPath, err)
	}

	fmt.Printf("Added SSH config entry for host %s to %s\n", host, sshConfigPath)
	return nil
}

// replaceOrAppendHostBlock replaces an existing Host block for the given host,
// or appends the new entry if no matching block exists. It detects blocks by
// looking for a "# Added by ssm-session-client" comment followed by a
// "Host <host>" line.
func replaceOrAppendHostBlock(content, host, newEntry string) string {
	lines := strings.Split(content, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		// Look for our marker comment followed by a matching Host line
		if strings.TrimSpace(lines[i]) == "# Added by ssm-session-client" && i+1 < len(lines) {
			hostLine := strings.TrimSpace(lines[i+1])
			if hostLine == fmt.Sprintf("Host %s", host) {
				// Skip the old block: marker, Host line, and indented option lines
				i += 2
				for i < len(lines) && strings.HasPrefix(lines[i], "  ") {
					i++
				}
				// Skip trailing blank line after block
				if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
					i++
				}
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}

	// Trim trailing empty lines before appending
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}

	out := strings.Join(result, "\n")
	if out != "" {
		out += "\n\n"
	}
	out += newEntry

	return out
}
