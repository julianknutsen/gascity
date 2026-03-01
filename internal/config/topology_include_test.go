package config

import (
	"strings"
	"testing"
)

func TestIsRemoteInclude(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Local paths — not remote.
		{"../maintenance", false},
		{"topologies/gastown", false},
		{"/absolute/path/to/topo", false},
		{"//city-root-relative", false},

		// SSH shorthand.
		{"git@github.com:org/repo.git", true},
		{"git@github.com:org/repo.git//topo#v1.0", true},

		// SSH scheme.
		{"ssh://git@github.com/org/repo.git", true},

		// HTTPS.
		{"https://github.com/org/repo.git", true},
		{"https://github.com/org/repo.git#main", true},

		// HTTP.
		{"http://internal.example.com/repo.git", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isRemoteInclude(tt.input)
			if got != tt.want {
				t.Errorf("isRemoteInclude(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRemoteInclude(t *testing.T) {
	tests := []struct {
		input       string
		wantSource  string
		wantSubpath string
		wantRef     string
	}{
		// SSH with subpath and ref.
		{
			"git@github.com:org/infra.git//topology#v1.0",
			"git@github.com:org/infra.git",
			"topology",
			"v1.0",
		},
		// HTTPS with ref only.
		{
			"https://github.com/org/repo.git#main",
			"https://github.com/org/repo.git",
			"",
			"main",
		},
		// SSH bare (no subpath, no ref).
		{
			"git@github.com:org/repo.git",
			"git@github.com:org/repo.git",
			"",
			"",
		},
		// HTTPS with subpath and ref.
		{
			"https://github.com/org/mono.git//packages/topo#v2.0",
			"https://github.com/org/mono.git",
			"packages/topo",
			"v2.0",
		},
		// SSH scheme URL with subpath.
		{
			"ssh://git@github.com/org/repo.git//sub/path",
			"ssh://git@github.com/org/repo.git",
			"sub/path",
			"",
		},
		// Ref with no subpath (HTTPS).
		{
			"https://github.com/org/repo.git#feature-branch",
			"https://github.com/org/repo.git",
			"",
			"feature-branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			source, subpath, ref := parseRemoteInclude(tt.input)
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if subpath != tt.wantSubpath {
				t.Errorf("subpath = %q, want %q", subpath, tt.wantSubpath)
			}
			if ref != tt.wantRef {
				t.Errorf("ref = %q, want %q", ref, tt.wantRef)
			}
		})
	}
}

func TestIncludeCacheName(t *testing.T) {
	tests := []struct {
		source     string
		wantPrefix string // slug prefix before the hash
	}{
		{"git@github.com:org/infra.git", "infra-"},
		{"https://github.com/org/repo.git", "repo-"},
		{"ssh://git@github.com/org/mytools.git", "mytools-"},
		{"https://github.com/org/mono.git", "mono-"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := includeCacheName(tt.source)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("includeCacheName(%q) = %q, want prefix %q", tt.source, got, tt.wantPrefix)
			}
			// Should contain a hex hash suffix (12 hex chars).
			suffix := got[len(tt.wantPrefix):]
			if len(suffix) != 12 {
				t.Errorf("hash suffix length = %d, want 12", len(suffix))
			}
		})
	}

	// Deterministic: same input → same output.
	a := includeCacheName("git@github.com:org/repo.git")
	b := includeCacheName("git@github.com:org/repo.git")
	if a != b {
		t.Errorf("not deterministic: %q != %q", a, b)
	}

	// Unique: different inputs → different outputs.
	c := includeCacheName("git@github.com:org/other.git")
	if a == c {
		t.Errorf("collision: %q == %q for different sources", a, c)
	}
}
