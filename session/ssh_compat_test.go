package session

import (
	"testing"
)

func TestIsSSHCompatMode_SSHFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "typical VSCode invocation",
			args: []string{"/usr/local/bin/ssm-session-client", "-T", "-D", "12345", "-o", "ConnectTimeout=15", "myhost"},
			want: true,
		},
		{
			name: "symlink as ssh",
			args: []string{"/usr/local/bin/ssh", "myhost"},
			want: true,
		},
		{
			name: "bare ssh name",
			args: []string{"ssh", "myhost"},
			want: true,
		},
		{
			name: "cobra subcommand ssh-direct",
			args: []string{"ssm-session-client", "ssh-direct", "ec2-user@i-abc123"},
			want: false,
		},
		{
			name: "cobra flag --help",
			args: []string{"ssm-session-client", "--help"},
			want: false,
		},
		{
			name: "no args",
			args: []string{"ssm-session-client"},
			want: false,
		},
		{
			name: "cobra subcommand shell",
			args: []string{"ssm-session-client", "shell", "i-abc123"},
			want: false,
		},
		{
			name: "ssh with -i flag",
			args: []string{"ssm-session-client", "-i", "/path/to/key", "myhost"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSSHCompatMode(tt.args)
			if got != tt.want {
				t.Errorf("IsSSHCompatMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestMergeSSHUser(t *testing.T) {
	tests := []struct {
		name     string
		argsUser string
		cfgUser  string
		want     string
	}{
		{"CLI user", "admin", "ec2-user", "admin"},
		{"config user", "", "ec2-user", "ec2-user"},
		{"default", "", "", "ec2-user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &SSHArgs{User: tt.argsUser}
			cfg := &SSHHostConfig{User: tt.cfgUser}
			got := mergeSSHUser(args, cfg)
			if got != tt.want {
				t.Errorf("mergeSSHUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeSSHPort(t *testing.T) {
	tests := []struct {
		name    string
		argPort int
		cfgPort string
		want    int
	}{
		{"CLI port", 2222, "", 2222},
		{"config port", 22, "3333", 3333},
		{"default", 22, "", 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &SSHArgs{Port: tt.argPort}
			cfg := &SSHHostConfig{Port: tt.cfgPort}
			got := mergeSSHPort(args, cfg)
			if got != tt.want {
				t.Errorf("mergeSSHPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveHostKeyCheck(t *testing.T) {
	tests := []struct {
		name    string
		optVal  string
		cfgVal  string
		want    bool
	}{
		{"no option", "", "", false},
		{"-o no", "no", "", true},
		{"-o accept-new", "accept-new", "", true},
		{"-o yes", "yes", "", false},
		{"config no", "", "no", true},
		{"CLI overrides config", "yes", "no", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &SSHArgs{Options: make(map[string]string)}
			if tt.optVal != "" {
				args.Options["StrictHostKeyChecking"] = tt.optVal
			}
			cfg := &SSHHostConfig{StrictHostKeyCheck: tt.cfgVal}
			got := resolveHostKeyCheck(args, cfg)
			if got != tt.want {
				t.Errorf("resolveHostKeyCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveConnectTimeout(t *testing.T) {
	args := &SSHArgs{Options: map[string]string{"ConnectTimeout": "30"}}
	cfg := &SSHHostConfig{}
	got := resolveConnectTimeout(args, cfg)
	if got != 30 {
		t.Errorf("expected 30, got %d", got)
	}
}

func TestResolveKnownHostsFile(t *testing.T) {
	args := &SSHArgs{Options: map[string]string{"UserKnownHostsFile": "/tmp/known_hosts"}}
	cfg := &SSHHostConfig{}
	got := resolveKnownHostsFile(args, cfg)
	if got != "/tmp/known_hosts" {
		t.Errorf("expected /tmp/known_hosts, got %q", got)
	}
}

func TestResolveKnownHostsFile_DevNull(t *testing.T) {
	args := &SSHArgs{Options: map[string]string{"UserKnownHostsFile": "/dev/null"}}
	cfg := &SSHHostConfig{}
	got := resolveKnownHostsFile(args, cfg)
	if got != "" {
		t.Errorf("expected empty for /dev/null, got %q", got)
	}
}
