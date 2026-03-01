package ssmclient

import (
	"strconv"
	"strings"
)

// agentVersionGte returns true if agentVersion >= minVersion using semantic version comparison.
// Versions are compared as dot-separated integers (e.g., "3.0.196.0" >= "3.0.0.0").
// Returns false if either version cannot be parsed.
func agentVersionGte(agentVersion, minVersion string) bool {
	agentParts := parseVersion(agentVersion)
	minParts := parseVersion(minVersion)

	// If either version is empty or invalid, return false (conservative)
	if len(agentParts) == 0 || len(minParts) == 0 {
		return false
	}

	// Pad shorter version with zeros for comparison
	maxLen := len(agentParts)
	if len(minParts) > maxLen {
		maxLen = len(minParts)
	}

	for i := 0; i < maxLen; i++ {
		agentVal := 0
		if i < len(agentParts) {
			agentVal = agentParts[i]
		}

		minVal := 0
		if i < len(minParts) {
			minVal = minParts[i]
		}

		if agentVal > minVal {
			return true
		}
		if agentVal < minVal {
			return false
		}
		// Equal, continue to next component
	}

	// All components equal
	return true
}

// parseVersion splits a version string like "3.0.196.0" into []int{3, 0, 196, 0}.
// Returns an empty slice if parsing fails or if any component is negative.
func parseVersion(v string) []int {
	if v == "" {
		return nil
	}

	parts := strings.Split(v, ".")
	result := make([]int, 0, len(parts))

	for _, p := range parts {
		num, err := strconv.Atoi(p)
		if err != nil || num < 0 {
			// Invalid version component or negative number, return empty
			return nil
		}
		result = append(result, num)
	}

	return result
}
