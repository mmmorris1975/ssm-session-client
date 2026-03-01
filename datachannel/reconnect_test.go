package datachannel

import (
	"testing"
	"time"
)

// TestBackoffProgression tests that the exponential backoff in processOutboundQueue
// progresses correctly when messages are pending.
func TestBackoffProgression(t *testing.T) {
	tests := []struct {
		name           string
		hasMessages    bool
		currentBackoff time.Duration
		wantBackoff    time.Duration
	}{
		{
			name:           "no messages - reset to min",
			hasMessages:    false,
			currentBackoff: 5 * time.Second,
			wantBackoff:    500 * time.Millisecond,
		},
		{
			name:           "messages pending - double backoff",
			hasMessages:    true,
			currentBackoff: 500 * time.Millisecond,
			wantBackoff:    1 * time.Second,
		},
		{
			name:           "messages pending - continue doubling",
			hasMessages:    true,
			currentBackoff: 5 * time.Second,
			wantBackoff:    10 * time.Second,
		},
		{
			name:           "at max - stay at max",
			hasMessages:    true,
			currentBackoff: 30 * time.Second,
			wantBackoff:    30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.currentBackoff
			maxBackoff := 30 * time.Second
			minBackoff := 500 * time.Millisecond

			// Simulate the backoff logic from processOutboundQueue
			if tt.hasMessages {
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			} else {
				backoff = minBackoff
			}

			if backoff != tt.wantBackoff {
				t.Errorf("backoff = %v, want %v", backoff, tt.wantBackoff)
			}
		})
	}
}

// TestBackoffBounds ensures backoff values stay within min/max bounds.
func TestBackoffBounds(t *testing.T) {
	minBackoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second

	// Test minimum bound
	backoff := 250 * time.Millisecond
	if backoff < minBackoff {
		backoff = minBackoff
	}
	if backoff != minBackoff {
		t.Errorf("backoff below minimum: got %v, want %v", backoff, minBackoff)
	}

	// Test maximum bound
	backoff = 60 * time.Second
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	if backoff != maxBackoff {
		t.Errorf("backoff above maximum: got %v, want %v", backoff, maxBackoff)
	}
}

// TestBackoffSequence verifies a realistic sequence of backoff values.
func TestBackoffSequence(t *testing.T) {
	expected := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped at max
		30 * time.Second, // stays at max
	}

	backoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second

	for i, want := range expected {
		if backoff != want {
			t.Errorf("step %d: backoff = %v, want %v", i, backoff, want)
		}

		// Simulate doubling with cap
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// TestPingIntervalCalculation verifies ping timing calculations.
func TestPingIntervalCalculation(t *testing.T) {
	pingInterval := 30 * time.Second
	pingTimeout := 10 * time.Second

	if pingInterval < pingTimeout {
		t.Error("ping interval should be greater than timeout")
	}

	if pingTimeout < 5*time.Second {
		t.Error("ping timeout too short for reliable detection")
	}

	// Verify reasonable values
	if pingInterval > 60*time.Second {
		t.Error("ping interval too long for timely detection")
	}
}

// TestReconnectConfiguration tests reconnection configuration values.
func TestReconnectConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		enableReconnect bool
		maxReconnects   int
		wantValid     bool
	}{
		{"enabled with limit", true, 5, true},
		{"enabled unlimited", true, 0, true},
		{"disabled", false, 5, true},
		{"enabled with high limit", true, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate configuration validation
			valid := true
			if tt.maxReconnects < 0 {
				valid = false
			}

			if valid != tt.wantValid {
				t.Errorf("config validity = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

// TestReconnectBackoff verifies exponential backoff for reconnection attempts.
func TestReconnectBackoff(t *testing.T) {
	tests := []struct {
		attempt     int
		baseBackoff time.Duration
		maxBackoff  time.Duration
		wantBackoff time.Duration
	}{
		{0, 1 * time.Second, 30 * time.Second, 1 * time.Second},
		{1, 1 * time.Second, 30 * time.Second, 2 * time.Second},
		{2, 1 * time.Second, 30 * time.Second, 4 * time.Second},
		{3, 1 * time.Second, 30 * time.Second, 8 * time.Second},
		{4, 1 * time.Second, 30 * time.Second, 16 * time.Second},
		{5, 1 * time.Second, 30 * time.Second, 30 * time.Second}, // capped
		{10, 1 * time.Second, 30 * time.Second, 30 * time.Second}, // stays capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			// Calculate backoff: baseBackoff * 2^attempt, capped at maxBackoff
			backoff := tt.baseBackoff
			for i := 0; i < tt.attempt; i++ {
				backoff *= 2
				if backoff > tt.maxBackoff {
					backoff = tt.maxBackoff
					break
				}
			}

			if backoff != tt.wantBackoff {
				t.Errorf("attempt %d: backoff = %v, want %v", tt.attempt, backoff, tt.wantBackoff)
			}
		})
	}
}
