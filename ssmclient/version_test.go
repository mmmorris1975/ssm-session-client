package ssmclient

import "testing"

func TestAgentVersionGte(t *testing.T) {
	tests := []struct {
		name         string
		agentVersion string
		minVersion   string
		want         bool
	}{
		// Exact matches
		{"equal simple", "1.0.0", "1.0.0", true},
		{"equal complex", "3.0.196.0", "3.0.196.0", true},

		// Agent version greater
		{"major greater", "2.0.0", "1.0.0", true},
		{"minor greater", "1.2.0", "1.1.0", true},
		{"patch greater", "1.0.5", "1.0.3", true},
		{"build greater", "3.0.196.5", "3.0.196.0", true},

		// Agent version less
		{"major less", "1.0.0", "2.0.0", false},
		{"minor less", "1.1.0", "1.2.0", false},
		{"patch less", "1.0.3", "1.0.5", false},
		{"build less", "3.0.196.0", "3.0.196.5", false},

		// Different length versions
		{"agent shorter equal", "1.0", "1.0.0", true},
		{"agent shorter less", "1.0", "1.0.1", false},
		{"min shorter equal", "1.0.0", "1.0", true},
		{"min shorter greater", "1.0.1", "1.0", true},

		// Real-world AWS SSM agent versions
		{"mux support exact", "3.0.196.0", "3.0.196.0", true},
		{"mux support greater", "3.1.500.0", "3.0.196.0", true},
		{"mux support less", "3.0.100.0", "3.0.196.0", false},
		{"mux support newer", "3.2.0.0", "3.0.196.0", true},
		{"keepalive disable", "3.1.1511.0", "3.1.1511.0", true},
		{"keepalive newer", "3.2.0.0", "3.1.1511.0", true},
		{"keepalive older", "3.1.1500.0", "3.1.1511.0", false},

		// Edge cases
		{"empty agent", "", "1.0.0", false},
		{"empty min", "1.0.0", "", false},
		{"both empty", "", "", false},
		{"invalid agent", "abc", "1.0.0", false},
		{"invalid min", "1.0.0", "xyz", false},
		{"invalid component", "1.x.0", "1.0.0", false},

		// Zero versions
		{"zero version", "0.0.0", "0.0.0", true},
		{"zero greater", "1.0.0", "0.0.0", true},
		{"zero less", "0.0.0", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentVersionGte(tt.agentVersion, tt.minVersion)
			if got != tt.want {
				t.Errorf("agentVersionGte(%q, %q) = %v, want %v",
					tt.agentVersion, tt.minVersion, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{"simple", "1.0.0", []int{1, 0, 0}},
		{"complex", "3.0.196.0", []int{3, 0, 196, 0}},
		{"single", "1", []int{1}},
		{"two parts", "1.2", []int{1, 2}},
		{"many parts", "1.2.3.4.5", []int{1, 2, 3, 4, 5}},
		{"large numbers", "999.888.777", []int{999, 888, 777}},
		{"empty", "", nil},
		{"invalid text", "abc", nil},
		{"partial invalid", "1.x.3", nil},
		{"negative", "1.-2.3", nil},
		{"trailing dot", "1.0.", nil},
		{"leading dot", ".1.0", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.input)
			if !intSliceEqual(got, tt.want) {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// intSliceEqual compares two int slices for equality
func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
