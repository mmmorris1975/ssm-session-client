package session

import (
	"fmt"
	"strconv"
	"strings"
)

// SSHArgs holds the parsed OpenSSH-compatible command-line arguments.
type SSHArgs struct {
	User           string            // -l flag or user@ prefix
	Host           string            // positional destination (after extracting user@)
	Port           int               // -p flag (default 22)
	IdentityFile   string            // -i flag
	ConfigFile     string            // -F flag
	DisablePTY     bool              // -T flag
	Options        map[string]string // -o key=value pairs
	Command        string            // trailing command after destination (joined)
	Verbose        int               // -v count (1, 2, or 3)
	NoCommand      bool              // -N flag
	DynamicForward string            // -D flag (ignored, stored for compat)
	ForwardAgent   bool              // -A flag
	ExitOnForward  bool              // -f flag (background, ignored)
	ForcePTY       bool              // -t flag
	Subsystem      bool              // -s flag
}

// ParseSSHArgs parses OpenSSH-compatible command-line arguments.
// It handles the subset of flags that VSCode Remote SSH uses.
func ParseSSHArgs(args []string) (*SSHArgs, error) {
	result := &SSHArgs{
		Port:    22,
		Options: make(map[string]string),
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "--" {
			i++
			break
		}

		if !strings.HasPrefix(arg, "-") {
			// First non-flag argument is the destination
			break
		}

		switch {
		// Flags that take a value argument
		case arg == "-o":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -o requires an argument")
			}
			key, val, err := parseSSHOption(args[i])
			if err != nil {
				return nil, err
			}
			result.Options[key] = val

		case arg == "-i":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -i requires an argument")
			}
			result.IdentityFile = args[i]

		case arg == "-F":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -F requires an argument")
			}
			result.ConfigFile = args[i]

		case arg == "-l":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -l requires an argument")
			}
			result.User = args[i]

		case arg == "-p":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -p requires an argument")
			}
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid port %q: %w", args[i], err)
			}
			result.Port = p

		case arg == "-D":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("option -D requires an argument")
			}
			result.DynamicForward = args[i]

		// Boolean flags
		case arg == "-T":
			result.DisablePTY = true
		case arg == "-t":
			result.ForcePTY = true
		case arg == "-N":
			result.NoCommand = true
		case arg == "-A":
			result.ForwardAgent = true
		case arg == "-f":
			result.ExitOnForward = true
		case arg == "-s":
			result.Subsystem = true

		// Verbosity flags
		case arg == "-v":
			result.Verbose = 1
		case arg == "-vv":
			result.Verbose = 2
		case arg == "-vvv":
			result.Verbose = 3

		// Compound single-letter boolean flags (e.g., -Tv, -TN)
		case len(arg) > 2 && arg[0] == '-' && arg[1] != '-':
			for _, ch := range arg[1:] {
				switch ch {
				case 'T':
					result.DisablePTY = true
				case 't':
					result.ForcePTY = true
				case 'N':
					result.NoCommand = true
				case 'A':
					result.ForwardAgent = true
				case 'f':
					result.ExitOnForward = true
				case 'v':
					result.Verbose++
				case 's':
					result.Subsystem = true
				default:
					// Ignore unknown boolean flags for forward compatibility
				}
			}

		default:
			// Ignore unknown flags for forward compatibility
			// If the next arg looks like a value (not a flag, not a host), skip it
		}

		i++
	}

	// Remaining args: destination [command ...]
	if i < len(args) {
		dest := args[i]
		i++

		// Parse user@host from destination
		if strings.Contains(dest, "@") {
			parts := strings.SplitN(dest, "@", 2)
			if result.User == "" {
				result.User = parts[0]
			}
			result.Host = parts[1]
		} else {
			result.Host = dest
		}

		// Everything after destination is the remote command
		if i < len(args) {
			result.Command = strings.Join(args[i:], " ")
		}
	}

	if result.Host == "" {
		return nil, fmt.Errorf("no destination host specified")
	}

	return result, nil
}

// parseSSHOption parses a "key=value" or "key value" SSH option string.
func parseSSHOption(opt string) (string, string, error) {
	// Handle key=value format
	if key, val, ok := strings.Cut(opt, "="); ok {
		return key, val, nil
	}
	// Handle "key value" format (single token means boolean true)
	parts := strings.Fields(opt)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	if len(parts) == 1 {
		return parts[0], "yes", nil
	}
	return "", "", fmt.Errorf("invalid SSH option format: %q", opt)
}

// HasVersionFlag checks if any of the arguments is -V (version query).
func HasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-V" {
			return true
		}
		// Also check compound flags like -TV
		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
			for _, ch := range arg[1:] {
				if ch == 'V' {
					return true
				}
			}
		}
	}
	return false
}

// GetOption returns the value of an SSH option (case-insensitive key match).
func (a *SSHArgs) GetOption(key string) (string, bool) {
	lower := strings.ToLower(key)
	for k, v := range a.Options {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
}
