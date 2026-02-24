package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// rigRoute pairs a bead prefix with the absolute directory it lives in.
type rigRoute struct {
	Prefix string
	AbsDir string
}

// routeEntry is a single line in routes.jsonl — maps a prefix to a relative path.
type routeEntry struct {
	Prefix string `json:"prefix"`
	Path   string `json:"path"`
}

// deriveBeadsPrefix computes a short bead ID prefix from a rig/city name.
// Ported from gastown/internal/rig/manager.go:deriveBeadsPrefix.
//
// Algorithm:
//  1. Strip -py, -go suffixes
//  2. Split on - or _
//  3. If single word, try splitting compound word (camelCase, etc.)
//  4. If 2+ parts: first letter of each part
//  5. If 1 part and ≤3 chars: use as-is
//  6. If 1 part and >3 chars: first 2 chars
func deriveBeadsPrefix(name string) string {
	name = strings.TrimSuffix(name, "-py")
	name = strings.TrimSuffix(name, "-go")

	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	if len(parts) == 1 {
		parts = splitCompoundWord(parts[0])
	}

	if len(parts) >= 2 {
		var prefix strings.Builder
		for _, p := range parts {
			if len(p) > 0 {
				prefix.WriteByte(p[0])
			}
		}
		return strings.ToLower(prefix.String())
	}

	if len(name) <= 3 {
		return strings.ToLower(name)
	}
	return strings.ToLower(name[:2])
}

// splitCompoundWord splits a camelCase or PascalCase word into parts.
// e.g. "myFrontend" → ["my", "Frontend"], "GasCity" → ["Gas", "City"]
func splitCompoundWord(word string) []string {
	if word == "" {
		return []string{word}
	}
	var parts []string
	start := 0
	runes := []rune(word)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	parts = append(parts, string(runes[start:]))
	if len(parts) <= 1 {
		return []string{word}
	}
	return parts
}

// generateRoutesFor computes the route entries for a single rig, given all
// known rigs. Each route is a relative path from `from` to every rig
// (including itself as ".").
func generateRoutesFor(from rigRoute, all []rigRoute) []routeEntry {
	routes := make([]routeEntry, 0, len(all))
	for _, to := range all {
		rel, err := filepath.Rel(from.AbsDir, to.AbsDir)
		if err != nil {
			// Fallback: use absolute path (shouldn't happen with valid paths).
			rel = to.AbsDir
		}
		routes = append(routes, routeEntry{Prefix: to.Prefix, Path: rel})
	}
	return routes
}

// writeAllRoutes generates and writes routes.jsonl for every rig. Each rig
// gets a routes.jsonl in its .beads/ directory mapping all known prefixes
// to relative paths.
func writeAllRoutes(rigs []rigRoute) error {
	for _, rig := range rigs {
		routes := generateRoutesFor(rig, rigs)
		if err := writeRoutesFile(rig.AbsDir, routes); err != nil {
			return fmt.Errorf("writing routes for %q: %w", rig.Prefix, err)
		}
	}
	return nil
}

// writeRoutesFile writes routes.jsonl to <dir>/.beads/routes.jsonl.
func writeRoutesFile(dir string, routes []routeEntry) error {
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return fmt.Errorf("creating .beads dir: %w", err)
	}

	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, r := range routes {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encoding route %q: %w", r.Prefix, err)
		}
	}

	path := filepath.Join(beadsDir, "routes.jsonl")
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}
