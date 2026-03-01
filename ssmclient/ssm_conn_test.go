package ssmclient

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// mockReadWriteCloser simulates a data channel for testing the bridge pattern.
// It uses two net.Pipe connections internally so bidirectional I/O works correctly.
type mockReadWriteCloser struct {
	conn net.Conn
}

func (m *mockReadWriteCloser) Read(p []byte) (int, error)  { return m.conn.Read(p) }
func (m *mockReadWriteCloser) Write(p []byte) (int, error) { return m.conn.Write(p) }
func (m *mockReadWriteCloser) Close() error                { return m.conn.Close() }

// newSSMConnFromReadWriter creates the same net.Pipe bridge as NewSSMConn but
// accepts a plain io.ReadWriteCloser for testing purposes.
func newSSMConnFromReadWriter(c io.ReadWriteCloser) net.Conn {
	localConn, pipeConn := net.Pipe()

	go func() {
		io.Copy(pipeConn, c) //nolint:errcheck
		pipeConn.Close()
	}()

	go func() {
		io.Copy(c, pipeConn) //nolint:errcheck
		pipeConn.Close()
	}()

	return localConn
}

// TestSSMConnBridgeClientToServer verifies data flows from the SSH client side
// through the bridge to the "data channel" (remote) side.
func TestSSMConnBridgeClientToServer(t *testing.T) {
	// Simulate the remote side of the SSM data channel using a net.Pipe.
	dcRemote, dcLocal := net.Pipe()
	defer dcRemote.Close()

	mock := &mockReadWriteCloser{conn: dcLocal}
	sshConn := newSSMConnFromReadWriter(mock)
	defer sshConn.Close()

	want := []byte("hello from SSH client")
	var wg sync.WaitGroup
	wg.Add(1)

	// Remote side reads what the SSH client sends.
	go func() {
		defer wg.Done()
		buf := make([]byte, len(want))
		n, err := io.ReadFull(dcRemote, buf)
		if err != nil {
			t.Errorf("remote read error: %v", err)
			return
		}
		if string(buf[:n]) != string(want) {
			t.Errorf("remote got %q, want %q", buf[:n], want)
		}
	}()

	if _, err := sshConn.Write(want); err != nil {
		t.Fatalf("sshConn write error: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out waiting for data to arrive at remote")
	}
}

// TestSSMConnBridgeServerToClient verifies data flows from the "data channel"
// (remote) side through the bridge to the SSH client side.
func TestSSMConnBridgeServerToClient(t *testing.T) {
	dcRemote, dcLocal := net.Pipe()
	defer dcRemote.Close()

	mock := &mockReadWriteCloser{conn: dcLocal}
	sshConn := newSSMConnFromReadWriter(mock)
	defer sshConn.Close()

	want := []byte("hello from SSM agent")
	var wg sync.WaitGroup
	wg.Add(1)

	// SSH client side reads what the remote sends.
	go func() {
		defer wg.Done()
		buf := make([]byte, len(want))
		n, err := io.ReadFull(sshConn, buf)
		if err != nil {
			t.Errorf("sshConn read error: %v", err)
			return
		}
		if string(buf[:n]) != string(want) {
			t.Errorf("sshConn got %q, want %q", buf[:n], want)
		}
	}()

	if _, err := dcRemote.Write(want); err != nil {
		t.Fatalf("remote write error: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out waiting for data to arrive at SSH client")
	}
}

// TestSSMConnBridgeBidirectional verifies concurrent data flow in both directions.
func TestSSMConnBridgeBidirectional(t *testing.T) {
	dcRemote, dcLocal := net.Pipe()
	defer dcRemote.Close()

	mock := &mockReadWriteCloser{conn: dcLocal}
	sshConn := newSSMConnFromReadWriter(mock)
	defer sshConn.Close()

	clientMsg := []byte("from client")
	serverMsg := []byte("from server")

	var wg sync.WaitGroup
	wg.Add(4)

	// Client → server
	go func() {
		defer wg.Done()
		if _, err := sshConn.Write(clientMsg); err != nil {
			t.Errorf("client write: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, len(clientMsg))
		if _, err := io.ReadFull(dcRemote, buf); err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if string(buf) != string(clientMsg) {
			t.Errorf("server got %q, want %q", buf, clientMsg)
		}
	}()

	// Server → client
	go func() {
		defer wg.Done()
		if _, err := dcRemote.Write(serverMsg); err != nil {
			t.Errorf("server write: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, len(serverMsg))
		if _, err := io.ReadFull(sshConn, buf); err != nil {
			t.Errorf("client read: %v", err)
			return
		}
		if string(buf) != string(serverMsg) {
			t.Errorf("client got %q, want %q", buf, serverMsg)
		}
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}

// TestSSMConnBridgeClosePropagates verifies that closing the data channel
// causes the SSH client conn to receive an EOF.
func TestSSMConnBridgeClosePropagates(t *testing.T) {
	dcRemote, dcLocal := net.Pipe()

	mock := &mockReadWriteCloser{conn: dcLocal}
	sshConn := newSSMConnFromReadWriter(mock)
	defer sshConn.Close()

	// Close the "remote" end to trigger EOF propagation.
	dcRemote.Close()

	// The bridge should propagate the closure; read on sshConn should return an error.
	buf := make([]byte, 1)
	done := make(chan error, 1)
	go func() {
		_, err := sshConn.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error after remote close, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out waiting for close propagation")
	}
}
