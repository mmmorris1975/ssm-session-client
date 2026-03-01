//go:build windows
// +build windows

package ssmclient

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureStdinErrorHandling(t *testing.T) {
	// Save and restore the package-level variables
	origInModeSaved := origInMode
	origOutModeSaved := origOutMode
	defer func() {
		origInMode = origInModeSaved
		origOutMode = origOutModeSaved
	}()

	// Test that configureStdin works without error
	// (We can't fully test without access to a real console, but we can verify it doesn't panic)
	err := configureStdin()
	if err != nil {
		// On CI or non-console environments, this may fail, which is expected
		t.Logf("configureStdin failed (expected in non-console environment): %v", err)
	}
}

func TestCleanupIdempotency(t *testing.T) {
	// Save and restore the package-level variables
	origInModeSaved := origInMode
	origOutModeSaved := origOutMode
	defer func() {
		origInMode = origInModeSaved
		origOutMode = origOutModeSaved
	}()

	// Set some non-zero values to test restoration
	origInMode = 0x1234
	origOutMode = 0x5678

	// Test that cleanup can be called multiple times without error
	err := cleanup()
	if err != nil {
		t.Errorf("cleanup() returned error: %v", err)
	}

	// Call again to test idempotency
	err = cleanup()
	if err != nil {
		t.Errorf("cleanup() second call returned error: %v", err)
	}
}

func TestGetWinSize(t *testing.T) {
	rows, cols, err := getWinSize()
	if err != nil {
		// On CI or non-console environments, this may fail
		t.Logf("getWinSize failed (expected in non-console environment): %v", err)
		return
	}

	// If successful, verify reasonable dimensions
	if rows == 0 || cols == 0 {
		t.Errorf("getWinSize returned zero dimensions: rows=%d, cols=%d", rows, cols)
	}

	// Console size should be within reasonable bounds
	if rows > 1000 || cols > 1000 {
		t.Errorf("getWinSize returned unusually large dimensions: rows=%d, cols=%d", rows, cols)
	}
}

func TestConsoleModeFlags(t *testing.T) {
	// Verify that our constants match the expected Windows API values
	tests := []struct {
		name     string
		constant uint32
		expected uint32
	}{
		{"ENABLE_ECHO_INPUT", ENABLE_ECHO_INPUT, 0x0004},
		{"ENABLE_LINE_INPUT", ENABLE_LINE_INPUT, 0x0002},
		{"ENABLE_PROCESSED_INPUT", ENABLE_PROCESSED_INPUT, 0x0001},
		{"ENABLE_VIRTUAL_TERMINAL_INPUT", ENABLE_VIRTUAL_TERMINAL_INPUT, 0x0200},
		{"ENABLE_VIRTUAL_TERMINAL_PROCESSING", ENABLE_VIRTUAL_TERMINAL_PROCESSING, 0x0004},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = 0x%04X, expected 0x%04X", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestStdHandleRetrieval(t *testing.T) {
	// Test that we can retrieve standard handles
	handles := []struct {
		name   string
		handle uint32
	}{
		{"STD_INPUT_HANDLE", windows.STD_INPUT_HANDLE},
		{"STD_OUTPUT_HANDLE", windows.STD_OUTPUT_HANDLE},
	}

	for _, h := range handles {
		t.Run(h.name, func(t *testing.T) {
			handle, err := windows.GetStdHandle(h.handle)
			if err != nil {
				t.Logf("GetStdHandle(%s) failed (expected in non-console environment): %v", h.name, err)
				return
			}
			if handle == 0 || handle == windows.InvalidHandle {
				t.Errorf("GetStdHandle(%s) returned invalid handle: %v", h.name, handle)
			}
		})
	}
}
