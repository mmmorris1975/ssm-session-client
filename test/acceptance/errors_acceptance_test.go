//go:build acceptance

package acceptance

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestInvalidTarget verifies that a garbage target string produces a non-zero exit code
// and a useful error message on stderr.
func TestInvalidTarget(t *testing.T) {
	_, stderr, code := runCmd(t, 30*time.Second, "shell", "not-a-valid-target-!!!")
	if code == 0 {
		t.Fatal("expected non-zero exit code for invalid target, got 0")
	}
	if stderr == "" {
		t.Error("expected error message on stderr for invalid target, got nothing")
	}
}

// TestMissingRegion verifies that running without an AWS region configured produces
// a non-zero exit and a clear error message on stderr.
func TestMissingRegion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Deliberately omit --aws-region and strip all region env vars.
	cmd := exec.CommandContext(ctx, binaryPath, "shell", "i-0000000000000000a") //nolint:gosec
	regionKeys := []string{"AWS_REGION", "AWS_DEFAULT_REGION", "SSC_AWS_REGION"}
	cmd.Env = append(filteredEnv(regionKeys...), "HOME="+os.Getenv("HOME"))

	var errBuf strings.Builder
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if runErr == nil {
		t.Fatal("expected non-zero exit code when region is missing, got 0")
	}
	if errBuf.String() == "" {
		t.Error("expected error on stderr when region is missing, got nothing")
	}
}

// TestSessionTermination starts an SSH-direct session, sends SIGINT to trigger
// graceful cleanup (TerminateSession), and verifies that no SSM sessions are leaked.
func TestSessionTermination(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	// Capture the baseline before starting the session so the cleanup check
	// only counts sessions opened by this test.
	registerSessionLeakCheck(t, i.InstanceID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, //nolint:gosec
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=false",
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "sleep 60",
		sshDirectUser+"@"+i.InstanceID,
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start session: %v", err)
	}

	// Wait for the SSM session to be fully established.
	time.Sleep(15 * time.Second)

	// Send SIGINT to allow the signal handler to call TerminateSession + Close.
	if cmd.Process != nil {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	// Give the process up to 5 seconds to terminate cleanly.
	done := make(chan struct{})
	go func() {
		cmd.Wait() //nolint:errcheck
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Force-kill if it didn't exit cleanly.
		cancel()
		<-done
	}
}

// filteredEnv returns os.Environ() with any entries whose key matches excludeKeys removed.
func filteredEnv(excludeKeys ...string) []string {
	exclude := make(map[string]bool, len(excludeKeys))
	for _, k := range excludeKeys {
		exclude[k] = true
	}

	var env []string
	for _, kv := range os.Environ() {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if !exclude[key] {
			env = append(env, kv)
		}
	}
	return env
}
