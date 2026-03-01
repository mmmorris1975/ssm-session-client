//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestPortForwardingToSSHPort forwards a local port to port 22 on the test instance and verifies
// that a TCP connection can be established through the tunnel.
func TestPortForwardingToSSHPort(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22) // blocks until port is accepting connections

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to forwarded port %d: %v", localPort, err)
	}
	// Read the SSH banner before closing. Closing immediately without any I/O
	// can leave the mux stream partially open, causing "closed pipe" errors and
	// leaked SSM sessions.
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	}
	conn.Close()
}

// TestPortForwardingMultipleConnections verifies that multiple concurrent TCP connections
// can be established through the same port-forwarding session (requires SSM agent >= 3.0.196.0).
func TestPortForwardingMultipleConnections(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22) // blocks until port is accepting connections

	const conns = 3
	errs := make(chan error, conns)
	for range conns {
		go func() {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
			if err != nil {
				errs <- err
				return
			}
			// Read the SSH banner to ensure the smux stream actually
			// exchanges data before closing. Immediately closing without
			// any I/O can leave the agent's smux in a partially-open state.
			buf := make([]byte, 256)
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, _ := c.Read(buf)
			if n == 0 {
				c.Close()
				errs <- fmt.Errorf("no data received from SSH port")
				return
			}
			c.Close()
			errs <- nil
		}()
	}

	for range conns {
		if err := <-errs; err != nil {
			t.Errorf("concurrent connection failed: %v", err)
		}
	}
}

// TestPortForwardingToRDPPort forwards a local port to port 3389 on the Windows test instance
// and verifies a TCP connection can be made through the tunnel.
// Skipped unless a Windows instance is configured (create_windows_instance=true in Terraform).
func TestPortForwardingToRDPPort(t *testing.T) {
	i := infra(t)
	if i.WindowsInstanceID == "" {
		t.Skip("windows_instance_id not set in infra outputs (set create_windows_instance=true in Terraform)")
	}
	waitForSSMReady(t, i.WindowsInstanceID)
	terminateAllSessions(t, i.WindowsInstanceID)
	registerSessionLeakCheck(t, i.WindowsInstanceID)

	localPort := freePort(t)
	winInfra := i
	winInfra.InstanceID = i.WindowsInstanceID
	startPortForwarder(t, winInfra, localPort, 3389) // blocks until port is accepting connections

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to forwarded RDP port %d: %v", localPort, err)
	}
	conn.Close()
}

// startPortForwarder launches ssm-session-client port-forwarding in the background.
// It registers a t.Cleanup to send SIGINT for graceful shutdown.
// The function blocks until the local TCP port is accepting connections, retrying the
// entire subprocess if the handshake hangs or the process exits prematurely.
func startPortForwarder(t *testing.T, i InfraOutputs, localPort, remotePort int) {
	t.Helper()
	target := fmt.Sprintf("%s:%d", i.InstanceID, remotePort)
	args := []string{
		"--config", "/dev/null",
		"--log-level", "debug",
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=false",
		"port-forwarding", target, strconv.Itoa(localPort),
	}

	const maxAttempts = 3
	const handshakeTimeout = 30 * time.Second

	var (
		cmd       *exec.Cmd
		cancel    context.CancelFunc
		exited    chan struct{}
		stderrBuf strings.Builder
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			t.Logf("port-forwarding attempt %d/%d after 5s cooldown...", attempt, maxAttempts)
			time.Sleep(5 * time.Second)
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		cancel = cancelFn
		stderrBuf.Reset()

		cmd = exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
		cmd.Stderr = &stderrBuf
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatalf("start port-forwarding: %v", err)
		}

		exited = make(chan struct{})
		go func() {
			cmd.Wait() //nolint:errcheck
			close(exited)
		}()

		// Poll the port until it opens or we time out.
		if portReady(localPort, handshakeTimeout, exited) {
			break // success — port is accepting connections
		}

		// Port never opened. Kill the process and maybe retry.
		t.Logf("port-forwarding attempt %d: port %d not ready after %s (stderr: %s)",
			attempt, localPort, handshakeTimeout, stderrBuf.String())
		cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			cancel()
			<-exited
		}
		cancel()

		if attempt == maxAttempts {
			t.Fatalf("port-forwarding failed to open port %d after %d attempts", localPort, maxAttempts)
		}
	}

	t.Cleanup(func() {
		// Send SIGINT first so the binary's signal handler can call TerminateSession
		// to cleanly close the SSM session. Only cancel the context (SIGKILL) as a fallback.
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			cancel()
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
			}
		}
		cancel()
		if s := stderrBuf.String(); s != "" {
			t.Logf("port-forwarding stderr: %s", s)
		}
	})
}

// portReady polls until a TCP connection to localhost:port succeeds, the deadline expires,
// or the process exits (signalled via the exited channel).
func portReady(port int, timeout time.Duration, exited <-chan struct{}) bool {
	deadline := time.After(timeout)
	addr := fmt.Sprintf("localhost:%d", port)
	for {
		select {
		case <-deadline:
			return false
		case <-exited:
			// Process died before the port opened — no point waiting.
			return false
		default:
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				conn.Close()
				return true
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
