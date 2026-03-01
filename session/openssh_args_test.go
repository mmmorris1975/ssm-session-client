package session

import (
	"testing"
)

func TestParseSSHArgs_BasicHost(t *testing.T) {
	args, err := ParseSSHArgs([]string{"myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.Host != "myhost" {
		t.Errorf("expected host=myhost, got %q", args.Host)
	}
	if args.Port != 22 {
		t.Errorf("expected port=22, got %d", args.Port)
	}
}

func TestParseSSHArgs_UserAtHost(t *testing.T) {
	args, err := ParseSSHArgs([]string{"ubuntu@i-0123456789abcdef0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.User != "ubuntu" {
		t.Errorf("expected user=ubuntu, got %q", args.User)
	}
	if args.Host != "i-0123456789abcdef0" {
		t.Errorf("expected host=i-0123456789abcdef0, got %q", args.Host)
	}
}

func TestParseSSHArgs_VSCodeTypical(t *testing.T) {
	// Typical VSCode Remote SSH invocation
	args, err := ParseSSHArgs([]string{
		"-T",
		"-D", "12345",
		"-o", "ConnectTimeout=15",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=/tmp/known_hosts",
		"-o", "ClearAllForwardings=yes",
		"-o", "RemoteCommand=none",
		"-F", "/tmp/vscode-ssh-config",
		"my-ec2-host",
		"bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !args.DisablePTY {
		t.Error("expected DisablePTY=true")
	}
	if args.DynamicForward != "12345" {
		t.Errorf("expected DynamicForward=12345, got %q", args.DynamicForward)
	}
	if args.ConfigFile != "/tmp/vscode-ssh-config" {
		t.Errorf("expected ConfigFile=/tmp/vscode-ssh-config, got %q", args.ConfigFile)
	}
	if args.Host != "my-ec2-host" {
		t.Errorf("expected Host=my-ec2-host, got %q", args.Host)
	}
	if args.Command != "bash" {
		t.Errorf("expected Command=bash, got %q", args.Command)
	}

	if val, ok := args.GetOption("ConnectTimeout"); !ok || val != "15" {
		t.Errorf("expected ConnectTimeout=15, got %q", val)
	}
	if val, ok := args.GetOption("StrictHostKeyChecking"); !ok || val != "accept-new" {
		t.Errorf("expected StrictHostKeyChecking=accept-new, got %q", val)
	}
}

func TestParseSSHArgs_PortAndLoginUser(t *testing.T) {
	args, err := ParseSSHArgs([]string{"-p", "2222", "-l", "admin", "myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.Port != 2222 {
		t.Errorf("expected port=2222, got %d", args.Port)
	}
	if args.User != "admin" {
		t.Errorf("expected user=admin, got %q", args.User)
	}
}

func TestParseSSHArgs_IdentityFile(t *testing.T) {
	args, err := ParseSSHArgs([]string{"-i", "/path/to/key", "myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.IdentityFile != "/path/to/key" {
		t.Errorf("expected IdentityFile=/path/to/key, got %q", args.IdentityFile)
	}
}

func TestParseSSHArgs_CompoundFlags(t *testing.T) {
	args, err := ParseSSHArgs([]string{"-TN", "myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !args.DisablePTY {
		t.Error("expected DisablePTY=true")
	}
	if !args.NoCommand {
		t.Error("expected NoCommand=true")
	}
}

func TestParseSSHArgs_Verbosity(t *testing.T) {
	args, err := ParseSSHArgs([]string{"-vvv", "myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.Verbose != 3 {
		t.Errorf("expected Verbose=3, got %d", args.Verbose)
	}
}

func TestParseSSHArgs_CommandWithArgs(t *testing.T) {
	args, err := ParseSSHArgs([]string{"myhost", "ls", "-la", "/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.Command != "ls -la /tmp" {
		t.Errorf("expected Command='ls -la /tmp', got %q", args.Command)
	}
}

func TestParseSSHArgs_NoHost(t *testing.T) {
	_, err := ParseSSHArgs([]string{"-T"})
	if err == nil {
		t.Error("expected error for missing host")
	}
}

func TestParseSSHArgs_UserAtHostWithLFlag(t *testing.T) {
	// -l flag should take precedence over user@host
	args, err := ParseSSHArgs([]string{"-l", "admin", "ubuntu@myhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// -l was set first, so it takes precedence
	if args.User != "admin" {
		t.Errorf("expected user=admin (-l takes precedence), got %q", args.User)
	}
}

func TestParseSSHArgs_MultipleOptions(t *testing.T) {
	args, err := ParseSSHArgs([]string{
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		"myhost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, ok := args.GetOption("ServerAliveInterval"); !ok || val != "60" {
		t.Errorf("expected ServerAliveInterval=60, got %q", val)
	}
	if val, ok := args.GetOption("ServerAliveCountMax"); !ok || val != "3" {
		t.Errorf("expected ServerAliveCountMax=3, got %q", val)
	}
}

func TestGetOption_CaseInsensitive(t *testing.T) {
	args := &SSHArgs{
		Options: map[string]string{
			"StrictHostKeyChecking": "no",
		},
	}

	if val, ok := args.GetOption("stricthostkeychecking"); !ok || val != "no" {
		t.Errorf("expected case-insensitive match, got %q, ok=%v", val, ok)
	}
}
