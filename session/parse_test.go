package session

import (
	"testing"
)

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		defaultUser string
		defaultPort int
		wantUser    string
		wantHost    string
		wantPort    int
		wantErr     bool
	}{
		{
			name:        "full format user@host:port",
			target:      "admin@10.0.0.1:2222",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "admin",
			wantHost:    "10.0.0.1",
			wantPort:    2222,
			wantErr:     false,
		},
		{
			name:        "host:port only",
			target:      "10.0.0.1:8080",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "ec2-user",
			wantHost:    "10.0.0.1",
			wantPort:    8080,
			wantErr:     false,
		},
		{
			name:        "user@host only",
			target:      "admin@10.0.0.1",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "admin",
			wantHost:    "10.0.0.1",
			wantPort:    22,
			wantErr:     false,
		},
		{
			name:        "host only",
			target:      "10.0.0.1",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "ec2-user",
			wantHost:    "10.0.0.1",
			wantPort:    22,
			wantErr:     false,
		},
		{
			name:        "hostname with default port",
			target:      "myserver.example.com",
			defaultUser: "",
			defaultPort: 443,
			wantUser:    "",
			wantHost:    "myserver.example.com",
			wantPort:    443,
			wantErr:     false,
		},
		{
			name:        "instance ID as host",
			target:      "i-1234567890abcdef0",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "ec2-user",
			wantHost:    "i-1234567890abcdef0",
			wantPort:    22,
			wantErr:     false,
		},
		{
			name:        "user@instance-id:port",
			target:      "ubuntu@i-1234567890abcdef0:22",
			defaultUser: "ec2-user",
			defaultPort: 22,
			wantUser:    "ubuntu",
			wantHost:    "i-1234567890abcdef0",
			wantPort:    22,
			wantErr:     false,
		},
		{
			name:        "empty user default",
			target:      "10.0.0.1:22",
			defaultUser: "",
			defaultPort: 22,
			wantUser:    "",
			wantHost:    "10.0.0.1",
			wantPort:    22,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, host, port, err := ParseHostPort(tt.target, tt.defaultUser, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHostPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestParseHostPort_InvalidPort(t *testing.T) {
	_, _, _, err := ParseHostPort("host:99999", "", 22)
	if err == nil {
		t.Error("expected error for invalid port")
	}
}
