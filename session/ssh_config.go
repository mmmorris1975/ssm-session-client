package session

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// SSHHostConfig holds the resolved configuration for a single SSH host.
type SSHHostConfig struct {
	HostName            string
	User                string
	Port                string
	IdentityFile        string
	StrictHostKeyCheck  string
	UserKnownHostsFile  string
	ConnectTimeout      string
	ServerAliveInterval string
	ServerAliveCountMax string
}

// sshConfigBlock represents a single Host block in an SSH config file.
type sshConfigBlock struct {
	patterns []string
	options  map[string]string
}

// ParseSSHConfig reads an SSH config file and returns the resolved settings
// for the given host. It processes Host directives and applies matching blocks
// in order (first match wins for each directive, per OpenSSH semantics).
func ParseSSHConfig(configFile, host string) *SSHHostConfig {
	result := &SSHHostConfig{}

	files := resolveConfigFiles(configFile)
	if len(files) == 0 {
		return result
	}

	for _, f := range files {
		blocks, err := parseSSHConfigFile(f)
		if err != nil {
			zap.S().Debugf("failed to parse SSH config %s: %v", f, err)
			continue
		}
		applyMatchingBlocks(blocks, host, result)
	}

	// Expand ~ in IdentityFile
	if result.IdentityFile != "" {
		result.IdentityFile = expandTilde(result.IdentityFile)
	}
	if result.UserKnownHostsFile != "" {
		result.UserKnownHostsFile = expandTilde(result.UserKnownHostsFile)
	}

	return result
}

// resolveConfigFiles returns the list of SSH config files to read.
// If configFile is specified, only that file is used. Otherwise,
// ~/.ssh/config is used.
func resolveConfigFiles(configFile string) []string {
	if configFile != "" {
		if _, err := os.Stat(configFile); err == nil {
			return []string{configFile}
		}
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	defaultConfig := filepath.Join(homeDir, ".ssh", "config")
	if _, err := os.Stat(defaultConfig); err == nil {
		return []string{defaultConfig}
	}

	return nil
}

// parseSSHConfigFile parses an SSH config file into a list of Host blocks.
func parseSSHConfigFile(path string) ([]sshConfigBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var blocks []sshConfigBlock
	var current *sshConfigBlock

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into keyword and argument
		key, val := splitConfigLine(line)
		if key == "" {
			continue
		}

		lowerKey := strings.ToLower(key)

		if lowerKey == "host" {
			// Start a new block
			block := sshConfigBlock{
				patterns: strings.Fields(val),
				options:  make(map[string]string),
			}
			blocks = append(blocks, block)
			current = &blocks[len(blocks)-1]
		} else if lowerKey == "match" {
			// Match blocks are more complex; skip them for now
			current = nil
		} else if current != nil {
			// Store option in current block (first occurrence wins)
			if _, exists := current.options[lowerKey]; !exists {
				current.options[lowerKey] = val
			}
		}
	}

	return blocks, scanner.Err()
}

// splitConfigLine splits an SSH config line into keyword and argument.
// Handles both "Keyword=Value" and "Keyword Value" formats.
func splitConfigLine(line string) (string, string) {
	// Handle Key=Value
	if key, val, ok := strings.Cut(line, "="); ok {
		return strings.TrimSpace(key), strings.TrimSpace(val)
	}

	// Handle Key Value (split on first whitespace)
	fields := strings.SplitN(line, " ", 2)
	if len(fields) < 2 {
		fields = strings.SplitN(line, "\t", 2)
	}
	if len(fields) < 2 {
		return fields[0], ""
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
}

// applyMatchingBlocks applies options from matching Host blocks to the result.
// Per OpenSSH semantics, for each directive the first obtained value is used.
func applyMatchingBlocks(blocks []sshConfigBlock, host string, result *SSHHostConfig) {
	for _, block := range blocks {
		if !hostMatchesPatterns(host, block.patterns) {
			continue
		}

		for key, val := range block.options {
			switch key {
			case "hostname":
				if result.HostName == "" {
					result.HostName = val
				}
			case "user":
				if result.User == "" {
					result.User = val
				}
			case "port":
				if result.Port == "" {
					result.Port = val
				}
			case "identityfile":
				if result.IdentityFile == "" {
					result.IdentityFile = val
				}
			case "stricthostkeychecking":
				if result.StrictHostKeyCheck == "" {
					result.StrictHostKeyCheck = val
				}
			case "userknownhostsfile":
				if result.UserKnownHostsFile == "" {
					result.UserKnownHostsFile = val
				}
			case "connecttimeout":
				if result.ConnectTimeout == "" {
					result.ConnectTimeout = val
				}
			case "serveraliveinterval":
				if result.ServerAliveInterval == "" {
					result.ServerAliveInterval = val
				}
			case "serveralivecountmax":
				if result.ServerAliveCountMax == "" {
					result.ServerAliveCountMax = val
				}
			}
		}
	}
}

// hostMatchesPatterns checks if a hostname matches any of the Host patterns.
// Supports * and ? wildcards per OpenSSH semantics.
func hostMatchesPatterns(host string, patterns []string) bool {
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = pattern[1:]
		}

		matched := matchSSHPattern(host, pattern)

		if negated && matched {
			return false
		}
		if !negated && matched {
			return true
		}
	}
	return false
}

// matchSSHPattern matches a hostname against an SSH-style glob pattern.
// Supports * (match any sequence) and ? (match single character).
func matchSSHPattern(s, pattern string) bool {
	return matchGlob(s, pattern)
}

// matchGlob implements simple glob matching with * and ? wildcards.
func matchGlob(s, pattern string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Skip consecutive stars
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchGlob(s[i:], pattern) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			s = s[1:]
			pattern = pattern[1:]
		default:
			if len(s) == 0 || s[0] != pattern[0] {
				return false
			}
			s = s[1:]
			pattern = pattern[1:]
		}
	}
	return len(s) == 0
}

// expandTilde expands a leading ~ to the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(homeDir, path[1:])
}
