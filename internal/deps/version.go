// Package deps provides minimum-version checking for external binary
// dependencies (dolt, bd). Each dependency defines a minimum compatible
// version constant and a Check function that verifies the installed binary.
package deps

import (
	"strconv"
	"strings"
)

// CompareVersions compares two semver strings.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) int {
	aParts := ParseVersion(a)
	bParts := ParseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// ParseVersion parses a "X.Y.Z" numeric string into [3]int.
// Non-numeric parts silently default to 0. Callers should validate input
// via regex before calling.
func ParseVersion(v string) [3]int {
	var parts [3]int
	split := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(split); i++ {
		parts[i], _ = strconv.Atoi(split[i])
	}
	return parts
}
