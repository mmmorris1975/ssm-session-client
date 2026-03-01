package session

import (
	"context"
	"strconv"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// RunSSHCompat is the entry point for OpenSSH-compatible mode.
// It parses OpenSSH-style arguments, reads SSH config, merges settings,
// and establishes an SSH-over-SSM session.
func RunSSHCompat(osArgs []string) error {
	args, err := ParseSSHArgs(osArgs)
	if err != nil {
		return err
	}

	// Map verbosity to log level
	applyVerbosity(args.Verbose)

	// Read SSH config file and resolve host settings
	hostCfg := ParseSSHConfig(args.ConfigFile, args.Host)

	// Resolve the effective target (HostName from config, or the host as-is)
	target := args.Host
	if hostCfg.HostName != "" {
		target = hostCfg.HostName
	}

	// Merge user: CLI -l flag > user@host > config file > default
	user := mergeSSHUser(args, hostCfg)

	// Merge port: CLI -p flag > config file > default
	port := mergeSSHPort(args, hostCfg)

	// Merge identity file: CLI -i flag > config file
	keyFile := args.IdentityFile
	if keyFile == "" && hostCfg.IdentityFile != "" {
		keyFile = hostCfg.IdentityFile
	}

	// Apply SSH options from -o flags and config
	noHostKeyCheck := resolveHostKeyCheck(args, hostCfg)
	knownHostsFile := resolveKnownHostsFile(args, hostCfg)
	connectTimeout := resolveConnectTimeout(args, hostCfg)

	// Determine exec command
	execCommand := args.Command

	// Determine PTY mode: -T disables, -t forces
	disablePTY := args.DisablePTY

	zap.S().Debugf("SSH compat: target=%s user=%s port=%d disablePTY=%v cmd=%q",
		target, user, port, disablePTY, execCommand)

	// Initialize the AWS client (respects SSC_ env vars and config file)
	InitializeClient()

	// Build AWS configs
	ssmcfg, err := BuildAWSConfig(context.Background(), "ssm")
	if err != nil {
		return err
	}

	tgt, err := ssmclient.ResolveTarget(target, ssmcfg)
	if err != nil {
		return err
	}

	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		return err
	}

	opts := &ssmclient.SSHDirectInput{
		Target:             tgt,
		User:               user,
		RemotePort:         port,
		KeyFile:            keyFile,
		NoHostKeyCheck:     noHostKeyCheck,
		ExecCommand:        execCommand,
		DisablePTY:         disablePTY,
		KnownHostsFile:     knownHostsFile,
		ConnectTimeoutSecs: connectTimeout,
		DynamicForward:     args.DynamicForward,
	}

	// Handle instance-connect if configured via env or app config
	if config.Flags().UseInstanceConnect {
		if err := prepareInstanceConnect(context.Background(), tgt, user, opts); err != nil {
			return err
		}
	}

	return ssmclient.SSHDirectSession(ssmMessagesCfg, opts)
}

// mergeSSHUser determines the effective SSH username from CLI args and config.
func mergeSSHUser(args *SSHArgs, cfg *SSHHostConfig) string {
	if args.User != "" {
		return args.User
	}
	if cfg.User != "" {
		return cfg.User
	}
	return "ec2-user"
}

// mergeSSHPort determines the effective SSH port from CLI args and config.
func mergeSSHPort(args *SSHArgs, cfg *SSHHostConfig) int {
	// If -p was explicitly set (not default)
	if args.Port != 22 {
		return args.Port
	}
	if cfg.Port != "" {
		if p, err := strconv.Atoi(cfg.Port); err == nil {
			return p
		}
	}
	return 22
}

// resolveHostKeyCheck determines the StrictHostKeyChecking behavior.
func resolveHostKeyCheck(args *SSHArgs, cfg *SSHHostConfig) bool {
	// CLI -o takes precedence
	if val, ok := args.GetOption("StrictHostKeyChecking"); ok {
		return isHostKeyCheckDisabled(val)
	}
	// Config file
	if cfg.StrictHostKeyCheck != "" {
		return isHostKeyCheckDisabled(cfg.StrictHostKeyCheck)
	}
	return false
}

// isHostKeyCheckDisabled returns true if the value means "skip host key checking".
func isHostKeyCheckDisabled(val string) bool {
	lower := strings.ToLower(val)
	return lower == "no" || lower == "accept-new"
}

// resolveKnownHostsFile determines the custom known_hosts file path.
func resolveKnownHostsFile(args *SSHArgs, cfg *SSHHostConfig) string {
	if val, ok := args.GetOption("UserKnownHostsFile"); ok {
		// /dev/null means "don't use known_hosts" - handled by NoHostKeyCheck
		if val == "/dev/null" {
			return ""
		}
		return expandTilde(val)
	}
	if cfg.UserKnownHostsFile != "" {
		return expandTilde(cfg.UserKnownHostsFile)
	}
	return ""
}

// resolveConnectTimeout determines the connection timeout.
func resolveConnectTimeout(args *SSHArgs, cfg *SSHHostConfig) int {
	if val, ok := args.GetOption("ConnectTimeout"); ok {
		if t, err := strconv.Atoi(val); err == nil {
			return t
		}
	}
	if cfg.ConnectTimeout != "" {
		if t, err := strconv.Atoi(cfg.ConnectTimeout); err == nil {
			return t
		}
	}
	return 0
}

// applyVerbosity sets the log level based on the -v flag count.
func applyVerbosity(level int) {
	switch {
	case level >= 3:
		_ = config.SetLogLevel("debug")
	case level == 2:
		_ = config.SetLogLevel("debug")
	case level == 1:
		_ = config.SetLogLevel("info")
	}
}

// IsSSHCompatMode checks if the given os.Args indicate OpenSSH-compatible mode.
// This is true when the first argument after the program name starts with "-"
// (not a cobra subcommand) or when the binary is invoked via a symlink named "ssh".
func IsSSHCompatMode(args []string) bool {
	if len(args) < 2 {
		return false
	}

	// Check if binary name ends with "ssh" (symlink mode)
	binName := args[0]
	if strings.HasSuffix(binName, "/ssh") || binName == "ssh" {
		return true
	}

	// Check if first arg looks like an SSH flag (starts with -)
	// and is NOT a known cobra flag (--help, --version, --config, etc.)
	firstArg := args[1]
	if !strings.HasPrefix(firstArg, "-") {
		return false
	}

	// Known cobra subcommands and persistent flags start with --
	// SSH flags are single-dash single-letter: -T, -o, -i, -p, etc.
	if strings.HasPrefix(firstArg, "--") {
		return false
	}

	// Single-dash flags that look like SSH flags
	return true
}
