package ssmclient

import (
	"errors"
	"net"
	"testing"
)

// mockResolver implements TargetResolver for testing.
type mockResolver struct {
	result string
	err    error
}

func (m *mockResolver) Resolve(_ string) (string, error) {
	return m.result, m.err
}

func TestResolveTargetChain_InstanceID(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantID  string
		wantErr bool
	}{
		{"standard instance ID", "i-1234567890abcdef0", "i-1234567890abcdef0", false},
		{"short instance ID", "i-12345678", "i-12345678", false},
		{"managed instance ID", "mi-1234567890abcdef0", "mi-1234567890abcdef0", false},
		{"uppercase hex", "i-ABCDEF12", "i-ABCDEF12", false},
		{"mixed case hex", "i-aAbBcCdD", "i-aAbBcCdD", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTargetChain(tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveTargetChain(%q) error = %v, wantErr %v", tt.target, err, tt.wantErr)
				return
			}
			if got != tt.wantID {
				t.Errorf("ResolveTargetChain(%q) = %q, want %q", tt.target, got, tt.wantID)
			}
		})
	}
}

func TestResolveTargetChain_NotInstanceID(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"hostname", "web-server-01"},
		{"IP address", "10.0.0.1"},
		{"tag format", "Name:myserver"},
		{"too short", "i-1234"},
		{"no hex after prefix", "i-zzzzzzzz"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No resolvers provided, so non-instance-ID targets should fail
			_, err := ResolveTargetChain(tt.target)
			if err == nil {
				t.Errorf("ResolveTargetChain(%q) should return error for non-instance-ID", tt.target)
			}
		})
	}
}

func TestResolveTargetChain_WhitespaceHandling(t *testing.T) {
	got, err := ResolveTargetChain("  i-1234567890abcdef0  ")
	// ResolveTarget calls TrimSpace, but ResolveTargetChain does not
	// The regex won't match with spaces
	if err == nil {
		t.Errorf("ResolveTargetChain with spaces: got %q, expected error", got)
	}
}

func TestResolveTargetChain_FallbackChain(t *testing.T) {
	failing := &mockResolver{err: errors.New("not found")}
	succeeding := &mockResolver{result: "i-found123456789"}

	got, err := ResolveTargetChain("some-hostname", failing, succeeding)
	if err != nil {
		t.Fatalf("ResolveTargetChain() error: %v", err)
	}
	if got != "i-found123456789" {
		t.Errorf("ResolveTargetChain() = %q, want %q", got, "i-found123456789")
	}
}

func TestResolveTargetChain_AllResolversFail(t *testing.T) {
	r1 := &mockResolver{err: errors.New("fail 1")}
	r2 := &mockResolver{err: errors.New("fail 2")}

	_, err := ResolveTargetChain("some-hostname", r1, r2)
	if !errors.Is(err, ErrNoInstanceFound) {
		t.Errorf("expected ErrNoInstanceFound, got: %v", err)
	}
}

func TestResolveTargetChain_FirstResolverWins(t *testing.T) {
	first := &mockResolver{result: "i-first1234567890"}
	second := &mockResolver{result: "i-second123456789"}

	got, err := ResolveTargetChain("some-hostname", first, second)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "i-first1234567890" {
		t.Errorf("got %q, want %q", got, "i-first1234567890")
	}
}

func TestIsPrivateAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		private bool
	}{
		// RFC 1918 - 10.0.0.0/8
		{"10.0.0.0", "10.0.0.0", true},
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},

		// RFC 1918 - 172.16.0.0/12
		{"172.16.0.0", "172.16.0.0", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"172.15.255.255 (not private)", "172.15.255.255", false},
		{"172.32.0.0 (not private)", "172.32.0.0", false},

		// RFC 1918 - 192.168.0.0/16
		{"192.168.0.0", "192.168.0.0", true},
		{"192.168.0.1", "192.168.0.1", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"192.167.0.0 (not private)", "192.167.0.0", false},

		// RFC 6598 - 100.64.0.0/10
		{"100.64.0.0", "100.64.0.0", true},
		{"100.64.0.1", "100.64.0.1", true},
		{"100.127.255.255", "100.127.255.255", true},
		{"100.63.255.255 (not private)", "100.63.255.255", false},
		{"100.128.0.0 (not private)", "100.128.0.0", false},

		// Public addresses
		{"8.8.8.8 (public)", "8.8.8.8", false},
		{"1.1.1.1 (public)", "1.1.1.1", false},
		{"203.0.113.1 (public)", "203.0.113.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.addr)
			if ip == nil {
				t.Fatalf("failed to parse IP: %s", tt.addr)
			}
			got := isPrivateAddr(ip)
			if got != tt.private {
				t.Errorf("isPrivateAddr(%s) = %v, want %v", tt.addr, got, tt.private)
			}
		})
	}
}

func TestTagResolver_InvalidFormat(t *testing.T) {
	// TagResolver expects "key:value" format
	resolver := &TagResolver{&EC2Resolver{}}

	tests := []struct {
		name   string
		target string
	}{
		{"no colon", "justahostname"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(tt.target)
			if !errors.Is(err, ErrInvalidTargetFormat) {
				t.Errorf("TagResolver.Resolve(%q) error = %v, want ErrInvalidTargetFormat", tt.target, err)
			}
		})
	}
}

func TestTagResolver_ValidFormat(t *testing.T) {
	// Test that the tag format is correctly parsed (we can't test the EC2 call without mocking)
	resolver := &TagResolver{&EC2Resolver{}}

	// This will fail because we don't have a real EC2 client,
	// but it should NOT fail with ErrInvalidTargetFormat
	_, err := resolver.Resolve("Name:myserver")
	if errors.Is(err, ErrInvalidTargetFormat) {
		t.Error("TagResolver.Resolve('Name:myserver') should not return ErrInvalidTargetFormat")
	}
}

func TestIPResolver_InvalidInput(t *testing.T) {
	resolver := &IPResolver{&EC2Resolver{}}

	tests := []struct {
		name   string
		target string
	}{
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.Resolve(tt.target)
			if err == nil {
				t.Errorf("IPResolver.Resolve(%q) should return error", tt.target)
			}
		})
	}
}

func TestDNSResolver_NonExistentDomain(t *testing.T) {
	resolver := NewDNSResolver()

	_, err := resolver.Resolve("nonexistent.invalid.test.example.")
	if err == nil {
		t.Error("DNSResolver.Resolve should return error for nonexistent domain")
	}
}

func TestResolveTargetChain_NoResolvers(t *testing.T) {
	// With no resolvers and non-instance-ID input
	_, err := ResolveTargetChain("some-hostname")
	if !errors.Is(err, ErrNoInstanceFound) {
		t.Errorf("expected ErrNoInstanceFound, got: %v", err)
	}
}

func TestResolveTargetChain_InstanceIDWithLeadingM(t *testing.T) {
	// Test managed instance IDs (mi-prefix)
	got, err := ResolveTargetChain("mi-0123456789abcdef0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "mi-0123456789abcdef0" {
		t.Errorf("got %q, want %q", got, "mi-0123456789abcdef0")
	}
}
