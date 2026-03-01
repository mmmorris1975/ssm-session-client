package cmd

import (
	"testing"
)

func TestReplaceOrAppendHostBlock_NewEntry(t *testing.T) {
	existing := "Host bastion\n  User admin\n  HostName 10.0.0.1\n"
	entry := "# Added by ssm-session-client\nHost i-0123456789abcdef0\n  User ec2-user\n  ProxyCommand ssm-session-client ssh ec2-user@%h\n"

	result := replaceOrAppendHostBlock(existing, "i-0123456789abcdef0", entry)

	if !contains(result, "Host bastion") {
		t.Error("expected existing Host bastion to be preserved")
	}
	if !contains(result, "Host i-0123456789abcdef0") {
		t.Error("expected new host entry to be appended")
	}
}

func TestReplaceOrAppendHostBlock_OverrideExisting(t *testing.T) {
	existing := `Host bastion
  User admin

# Added by ssm-session-client
Host i-0123456789abcdef0
  User old-user
  ProxyCommand old-command

Host other
  User test
`
	newEntry := "# Added by ssm-session-client\nHost i-0123456789abcdef0\n  User ec2-user\n  ProxyCommand ssm-session-client ssh ec2-user@%h\n"

	result := replaceOrAppendHostBlock(existing, "i-0123456789abcdef0", newEntry)

	if !contains(result, "Host bastion") {
		t.Error("expected Host bastion to be preserved")
	}
	if !contains(result, "Host other") {
		t.Error("expected Host other to be preserved")
	}
	if contains(result, "old-user") {
		t.Error("expected old entry to be replaced")
	}
	if !contains(result, "ProxyCommand ssm-session-client ssh ec2-user@%h") {
		t.Error("expected new proxy command to be present")
	}
}

func TestReplaceOrAppendHostBlock_EmptyFile(t *testing.T) {
	entry := "# Added by ssm-session-client\nHost i-abc\n  User ec2-user\n  ProxyCommand cmd ssh ec2-user@%h\n"

	result := replaceOrAppendHostBlock("", "i-abc", entry)

	if !contains(result, "Host i-abc") {
		t.Error("expected host entry in result")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
