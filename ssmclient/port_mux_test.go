package ssmclient

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/xtaci/smux"
)

// TestMuxBidirectionalDataFlow tests that data flows correctly in both directions through smux.
func TestMuxBidirectionalDataFlow(t *testing.T) {
	// Create a pair of connected pipes to simulate a network connection
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Start smux server
	serverSession, err := smux.Server(serverConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux server: %v", err)
	}
	defer serverSession.Close()

	// Start smux client
	clientSession, err := smux.Client(clientConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux client: %v", err)
	}
	defer clientSession.Close()

	// Test data
	clientToServer := []byte("hello from client")
	serverToClient := []byte("hello from server")

	var wg sync.WaitGroup
	wg.Add(2)

	// Server side: accept stream, echo data back
	go func() {
		defer wg.Done()
		stream, err := serverSession.AcceptStream()
		if err != nil {
			t.Errorf("server failed to accept stream: %v", err)
			return
		}
		defer stream.Close()

		// Read from client
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			t.Errorf("server failed to read: %v", err)
			return
		}

		if string(buf[:n]) != string(clientToServer) {
			t.Errorf("server received %q, expected %q", buf[:n], clientToServer)
		}

		// Write to client
		if _, err := stream.Write(serverToClient); err != nil {
			t.Errorf("server failed to write: %v", err)
		}
	}()

	// Client side: open stream, send data, receive response
	go func() {
		defer wg.Done()
		stream, err := clientSession.OpenStream()
		if err != nil {
			t.Errorf("client failed to open stream: %v", err)
			return
		}
		defer stream.Close()

		// Write to server
		if _, err := stream.Write(clientToServer); err != nil {
			t.Errorf("client failed to write: %v", err)
			return
		}

		// Read from server
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			t.Errorf("client failed to read: %v", err)
			return
		}

		if string(buf[:n]) != string(serverToClient) {
			t.Errorf("client received %q, expected %q", buf[:n], serverToClient)
		}
	}()

	// Wait for both sides to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("test timed out")
	}
}

// TestMuxMultipleConcurrentStreams tests that multiple streams can operate concurrently.
func TestMuxMultipleConcurrentStreams(t *testing.T) {
	// Create a pair of connected pipes
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Start smux server
	serverSession, err := smux.Server(serverConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux server: %v", err)
	}
	defer serverSession.Close()

	// Start smux client
	clientSession, err := smux.Client(clientConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux client: %v", err)
	}
	defer clientSession.Close()

	numStreams := 5
	var wg sync.WaitGroup
	wg.Add(numStreams * 2) // Server + client for each stream

	// Server side: accept and echo for each stream
	for i := 0; i < numStreams; i++ {
		go func(streamNum int) {
			defer wg.Done()
			stream, err := serverSession.AcceptStream()
			if err != nil {
				t.Errorf("server failed to accept stream %d: %v", streamNum, err)
				return
			}
			defer stream.Close()

			// Echo data back
			buf := make([]byte, 1024)
			n, err := stream.Read(buf)
			if err != nil {
				t.Errorf("server stream %d failed to read: %v", streamNum, err)
				return
			}

			if _, err := stream.Write(buf[:n]); err != nil {
				t.Errorf("server stream %d failed to write: %v", streamNum, err)
			}
		}(i)
	}

	// Client side: open streams and send data
	for i := 0; i < numStreams; i++ {
		go func(streamNum int) {
			defer wg.Done()
			stream, err := clientSession.OpenStream()
			if err != nil {
				t.Errorf("client failed to open stream %d: %v", streamNum, err)
				return
			}
			defer stream.Close()

			testData := []byte("stream " + string(rune('0'+streamNum)))

			if _, err := stream.Write(testData); err != nil {
				t.Errorf("client stream %d failed to write: %v", streamNum, err)
				return
			}

			buf := make([]byte, 1024)
			n, err := stream.Read(buf)
			if err != nil {
				t.Errorf("client stream %d failed to read: %v", streamNum, err)
				return
			}

			if string(buf[:n]) != string(testData) {
				t.Errorf("client stream %d received %q, expected %q", streamNum, buf[:n], testData)
			}
		}(i)
	}

	// Wait for all streams to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("test timed out")
	}
}

// TestHandleMuxConnectionCancellation tests that mux connections are properly closed on context cancellation.
func TestHandleMuxConnectionCancellation(t *testing.T) {
	// Create a pair of connected pipes
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Start smux server and client
	serverSession, err := smux.Server(serverConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux server: %v", err)
	}
	defer serverSession.Close()

	clientSession, err := smux.Client(clientConn, nil)
	if err != nil {
		t.Fatalf("failed to create smux client: %v", err)
	}
	defer clientSession.Close()

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create a mock TCP connection using pipes
	tcpServer, tcpClient := net.Pipe()
	defer tcpServer.Close()
	defer tcpClient.Close()

	// Start handleMuxConnection in a goroutine
	done := make(chan struct{})
	go func() {
		handleMuxConnection(ctx, clientSession, tcpClient)
		close(done)
	}()

	// Server side: accept the stream
	go func() {
		stream, err := serverSession.AcceptStream()
		if err != nil {
			return
		}
		defer stream.Close()

		// Just wait for stream to close
		io.Copy(io.Discard, stream)
	}()

	// Give it a moment to establish
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for handleMuxConnection to return with timeout
	select {
	case <-done:
		// Success - function returned on cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("handleMuxConnection did not return after context cancellation")
	}
}

// TestMuxConfigKeepAliveDisabled verifies that keepalive can be disabled based on agent version.
func TestMuxConfigKeepAliveDisabled(t *testing.T) {
	tests := []struct {
		name         string
		agentVersion string
		wantDisabled bool
	}{
		{"old agent", "3.0.196.0", false},
		{"exact threshold", "3.1.1511.0", true},
		{"newer agent", "3.2.0.0", true},
		{"much older", "2.0.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := smux.DefaultConfig()

			// Apply the same logic as in startMuxPortForwarding
			if agentVersionGte(tt.agentVersion, "3.1.1511.0") {
				config.KeepAliveDisabled = true
			}

			if config.KeepAliveDisabled != tt.wantDisabled {
				t.Errorf("KeepAliveDisabled = %v, want %v for agent version %s",
					config.KeepAliveDisabled, tt.wantDisabled, tt.agentVersion)
			}
		})
	}
}
