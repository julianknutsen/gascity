package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveBeadsPrefix(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"my-frontend", "mf"},
		{"backend", "ba"},
		{"tower-of-hanoi", "toh"},
		{"api", "api"},
		{"ab", "ab"},
		{"a", "a"},
		{"my-frontend-go", "mf"},   // strip -go suffix
		{"data-pipeline-py", "dp"}, // strip -py suffix
		{"my_project", "mp"},       // underscore split
		{"x", "x"},                 // single char
		{"myFrontend", "mf"},       // camelCase split
		{"GasCity", "gc"},          // PascalCase split
		{"toolbox", "to"},          // single word >3 chars
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBeadsPrefix(tt.name)
			if got != tt.want {
				t.Errorf("deriveBeadsPrefix(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSplitCompoundWord(t *testing.T) {
	tests := []struct {
		word string
		want []string
	}{
		{"myFrontend", []string{"my", "Frontend"}},
		{"GasCity", []string{"Gas", "City"}},
		{"simple", []string{"simple"}},
		{"ABC", []string{"ABC"}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := splitCompoundWord(tt.word)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCompoundWord(%q) = %v, want %v", tt.word, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCompoundWord(%q)[%d] = %q, want %q", tt.word, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGenerateRoutesFor(t *testing.T) {
	all := []rigRoute{
		{Prefix: "mc", AbsDir: "/home/user/my-city"},
		{Prefix: "fe", AbsDir: "/home/user/projects/my-frontend"},
		{Prefix: "be", AbsDir: "/home/user/projects/my-backend"},
	}

	t.Run("from HQ", func(t *testing.T) {
		routes := generateRoutesFor(all[0], all)
		if len(routes) != 3 {
			t.Fatalf("len(routes) = %d, want 3", len(routes))
		}
		// Self-route should be "."
		if routes[0].Path != "." {
			t.Errorf("self route path = %q, want %q", routes[0].Path, ".")
		}
		if routes[0].Prefix != "mc" {
			t.Errorf("self route prefix = %q, want %q", routes[0].Prefix, "mc")
		}
		// Route to frontend
		if routes[1].Prefix != "fe" {
			t.Errorf("routes[1].Prefix = %q, want %q", routes[1].Prefix, "fe")
		}
		if routes[1].Path != "../projects/my-frontend" {
			t.Errorf("routes[1].Path = %q, want %q", routes[1].Path, "../projects/my-frontend")
		}
	})

	t.Run("from frontend", func(t *testing.T) {
		routes := generateRoutesFor(all[1], all)
		if len(routes) != 3 {
			t.Fatalf("len(routes) = %d, want 3", len(routes))
		}
		// Route to HQ
		if routes[0].Prefix != "mc" {
			t.Errorf("routes[0].Prefix = %q, want %q", routes[0].Prefix, "mc")
		}
		if routes[0].Path != "../../my-city" {
			t.Errorf("routes[0].Path = %q, want %q", routes[0].Path, "../../my-city")
		}
		// Self-route
		if routes[1].Path != "." {
			t.Errorf("self route path = %q, want %q", routes[1].Path, ".")
		}
		// Route to backend (sibling)
		if routes[2].Path != "../my-backend" {
			t.Errorf("routes[2].Path = %q, want %q", routes[2].Path, "../my-backend")
		}
	})
}

func TestWriteAllRoutes(t *testing.T) {
	dir := t.TempDir()
	hqDir := filepath.Join(dir, "my-city")
	feDir := filepath.Join(dir, "projects", "frontend")

	if err := os.MkdirAll(hqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(feDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rigs := []rigRoute{
		{Prefix: "mc", AbsDir: hqDir},
		{Prefix: "fe", AbsDir: feDir},
	}

	if err := writeAllRoutes(rigs); err != nil {
		t.Fatalf("writeAllRoutes() error = %v", err)
	}

	// Check HQ routes file exists and has correct content.
	hqRoutes := filepath.Join(hqDir, ".beads", "routes.jsonl")
	data, err := os.ReadFile(hqRoutes)
	if err != nil {
		t.Fatalf("reading HQ routes: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("HQ routes has %d lines, want 2", len(lines))
	}

	var entry routeEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("parsing HQ route line 0: %v", err)
	}
	if entry.Prefix != "mc" || entry.Path != "." {
		t.Errorf("HQ self route = {%q, %q}, want {%q, %q}", entry.Prefix, entry.Path, "mc", ".")
	}

	// Check frontend routes file.
	feRoutes := filepath.Join(feDir, ".beads", "routes.jsonl")
	data, err = os.ReadFile(feRoutes)
	if err != nil {
		t.Fatalf("reading frontend routes: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("frontend routes has %d lines, want 2", len(lines))
	}
}

func TestWriteAllRoutes_Idempotent(t *testing.T) {
	dir := t.TempDir()
	rigs := []rigRoute{
		{Prefix: "mc", AbsDir: dir},
	}

	// Write twice — second should overwrite cleanly.
	if err := writeAllRoutes(rigs); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeAllRoutes(rigs); err != nil {
		t.Fatalf("second write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".beads", "routes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 route line after idempotent write, got %d", len(lines))
	}
}
