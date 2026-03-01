//go:build windows

package ssmclient

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestBuildRDPFileContent(t *testing.T) {
	tests := []struct {
		name      string
		localPort int
		username  string
		password  string
		wantParts []string
	}{
		{
			name:      "standard port and username",
			localPort: 13389,
			username:  "Administrator",
			password:  "TestPass123",
			wantParts: []string{
				"full address:s:localhost:13389",
				"username:s:Administrator",
				"password 51:b:",
				"authentication level:i:0",
				"prompt for credentials on client:i:1",
			},
		},
		{
			name:      "non-default port",
			localPort: 55000,
			username:  "domain\\user",
			password:  "P@ssw0rd",
			wantParts: []string{
				"full address:s:localhost:55000",
				"username:s:domain\\user",
				"password 51:b:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := buildRDPFileContent(tt.localPort, tt.username, tt.password)
			if err != nil {
				t.Fatalf("buildRDPFileContent: %v", err)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(content, part) {
					t.Errorf("expected RDP content to contain %q, got:\n%s", part, content)
				}
			}
		})
	}
}

func TestEncryptRDPPassword(t *testing.T) {
	encrypted, err := encryptRDPPassword("TestPassword123")
	if err != nil {
		t.Fatalf("encryptRDPPassword: %v", err)
	}
	if encrypted == "" {
		t.Error("encrypted password should not be empty")
	}
	// DPAPI output is a hex string — must be even length and only hex chars
	if len(encrypted)%2 != 0 {
		t.Errorf("hex string has odd length: %d", len(encrypted))
	}
	for _, c := range encrypted {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			t.Errorf("unexpected character in hex output: %c", c)
			break
		}
	}
}

func TestWriteUTF16LEFile(t *testing.T) {
	f, err := os.CreateTemp("", "rdp-test-*.rdp")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer os.Remove(f.Name())

	content := "full address:s:localhost:13389\r\nusername:s:Administrator\r\n"
	if err := writeUTF16LEFile(f, content); err != nil {
		f.Close()
		t.Fatalf("writeUTF16LEFile: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	// First two bytes must be UTF-16 LE BOM
	if len(data) < 2 {
		t.Fatal("file too short")
	}
	if data[0] != 0xFF || data[1] != 0xFE {
		t.Errorf("expected UTF-16 LE BOM (FF FE), got %02X %02X", data[0], data[1])
	}

	// Decode the rest as UTF-16 LE and compare
	body := data[2:]
	if len(body)%2 != 0 {
		t.Fatal("UTF-16 body has odd number of bytes")
	}
	u16 := make([]uint16, len(body)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(body[i*2:])
	}
	decoded := string(utf16.Decode(u16))
	if decoded != content {
		t.Errorf("decoded content mismatch\nwant: %q\ngot:  %q", content, decoded)
	}
}
