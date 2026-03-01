//go:build acceptance

package acceptance

import (
	"strings"
	"testing"
	"time"
)

const (
	// shellTimeout covers SSM session setup plus command execution.
	shellTimeout = 60 * time.Second
	shellMarker  = "ssm_acceptance_marker"
)

// TestShellByInstanceID verifies SSM session + target resolution via instance ID.
// Uses ssh-direct which avoids the TTY requirement of the shell command.
func TestShellByInstanceID(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstanceID
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestShellByTag verifies target resolution via tag (Name:<value>).
// Uses the shell command because ssh-direct's user@host[:port] format
// conflicts with the tag Key:Value format. The shell command passes the
// raw target to ResolveTarget which handles tags correctly.
func TestShellByTag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	// The shell command in non-TTY mode doesn't cleanly terminate SSM sessions,
	// so we skip the leak checker and clean up sessions after the test instead.
	t.Cleanup(func() { terminateAllSessions(t, i.InstanceID) })

	target := i.AliasTagKey + ":" + i.AliasTagValue
	_, stderr, code := runCmd(t, shellTimeout, "shell", target)
	// The shell command may fail due to non-TTY stdin, but target resolution should succeed.
	if code != 0 {
		lower := strings.ToLower(stderr)
		if strings.Contains(lower, "no instances") || strings.Contains(lower, "could not resolve") ||
			strings.Contains(lower, "not found") || strings.Contains(lower, "invalid") {
			t.Fatalf("tag resolution failed: %s", stderr)
		}
		t.Logf("shell exited %d (expected for non-TTY stdin); tag resolution succeeded", code)
	}
}

// TestShellByAlias verifies target resolution via a named alias (--alias flag).
func TestShellByAlias(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	// The shell command in non-TTY mode doesn't cleanly terminate SSM sessions.
	t.Cleanup(func() { terminateAllSessions(t, i.InstanceID) })

	aliasFlag := "test-alias=" + i.AliasTagKey + ":" + i.AliasTagValue
	_, stderr, code := runCmd(t, shellTimeout, "--alias", aliasFlag, "shell", "test-alias")
	// Same as TestShellByTag: shell may fail on TTY but alias resolution should work.
	if code != 0 {
		lower := strings.ToLower(stderr)
		if strings.Contains(lower, "no instances") || strings.Contains(lower, "could not resolve") ||
			strings.Contains(lower, "not found") || strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "alias") {
			t.Fatalf("alias resolution failed: %s", stderr)
		}
		t.Logf("shell exited %d (expected for non-TTY stdin); alias resolution succeeded", code)
	}
}

// TestShellByPrivateIP verifies target resolution via the instance's private IP.
func TestShellByPrivateIP(t *testing.T) {
	i := infra(t)
	if i.InstancePrivateIP == "" {
		t.Skip("instance_private_ip not set in infra outputs")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstancePrivateIP
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct by private IP exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestShellByDNSTXT verifies target resolution via a DNS hostname whose TXT record
// holds the instance ID.
func TestShellByDNSTXT(t *testing.T) {
	i := infra(t)
	if i.DNSHostname == "" {
		t.Skip("dns_hostname not set in infra outputs (set create_dns_record=true in Terraform)")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.DNSHostname
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct by DNS TXT exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}
