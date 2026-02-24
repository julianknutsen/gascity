package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

func TestDoRigAdd_Basic(t *testing.T) {
	// Set up a minimal city directory.
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test-city"

[[agents]]
name = "mayor"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a rig directory.
	rigPath := filepath.Join(t.TempDir(), "my-frontend")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Skip dolt/bd operations.
	t.Setenv("GC_DOLT", "skip")
	t.Setenv("GC_BEADS", "file")

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigAdd returned %d, stderr: %s", code, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Adding rig 'my-frontend'") {
		t.Errorf("output missing rig name: %s", output)
	}
	if !strings.Contains(output, "Prefix: mf") {
		t.Errorf("output missing prefix: %s", output)
	}
	if !strings.Contains(output, "Rig added.") {
		t.Errorf("output missing completion: %s", output)
	}

	// Verify rig.toml was created.
	rigTomlPath := filepath.Join(cityPath, "rigs", "my-frontend", "rig.toml")
	if _, err := os.Stat(rigTomlPath); err != nil {
		t.Errorf("rig.toml not created: %v", err)
	}

	// Verify city.toml was updated with [[rigs]] entry.
	data, err := os.ReadFile(filepath.Join(cityPath, "city.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "my-frontend") {
		t.Errorf("city.toml should contain rig name:\n%s", data)
	}
}

func TestDoRigAdd_DuplicateName(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test-city"

[[agents]]
name = "mayor"

[[rigs]]
name = "frontend"
path = "/some/path"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	rigPath := filepath.Join(t.TempDir(), "frontend")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GC_DOLT", "skip")
	t.Setenv("GC_BEADS", "file")

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doRigAdd should fail for duplicate, got code %d", code)
	}
	if !strings.Contains(stderr.String(), "already registered") {
		t.Errorf("stderr should mention duplicate: %s", stderr.String())
	}
}

func TestDoRigAdd_NotADirectory(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test"

[[agents]]
name = "mayor"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a file, not a directory.
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, filePath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure for non-directory, got code %d", code)
	}
}

func TestDoRigAdd_RoutesGenerated(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "my-city"

[[agents]]
name = "mayor"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	rigPath := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GC_DOLT", "skip")
	t.Setenv("GC_BEADS", "file")

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigAdd returned %d, stderr: %s", code, stderr.String())
	}

	// Verify routes.jsonl was created for city.
	cityRoutes := filepath.Join(cityPath, ".beads", "routes.jsonl")
	if _, err := os.Stat(cityRoutes); err != nil {
		t.Errorf("city routes.jsonl not created: %v", err)
	}

	// Verify routes.jsonl was created for rig.
	rigRoutes := filepath.Join(rigPath, ".beads", "routes.jsonl")
	if _, err := os.Stat(rigRoutes); err != nil {
		t.Errorf("rig routes.jsonl not created: %v", err)
	}
}

func TestDoRigList_WithRigs(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create .beads/metadata.json for HQ.
	beadsDir := filepath.Join(cityPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	rigPath := filepath.Join(t.TempDir(), "my-frontend")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	cityToml := `[workspace]
name = "test-city"

[[agents]]
name = "mayor"

[[rigs]]
name = "my-frontend"
path = "` + rigPath + `"
prefix = "fe"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigList(fsys.OSFS{}, cityPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigList returned %d, stderr: %s", code, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "test-city (HQ)") {
		t.Errorf("output missing HQ: %s", output)
	}
	if !strings.Contains(output, "Prefix: tc") {
		t.Errorf("output missing HQ prefix: %s", output)
	}
	if !strings.Contains(output, "Beads:  initialized") {
		t.Errorf("output missing HQ beads status: %s", output)
	}
	if !strings.Contains(output, "my-frontend") {
		t.Errorf("output missing rig name: %s", output)
	}
	if !strings.Contains(output, "Prefix: fe") {
		t.Errorf("output missing rig prefix: %s", output)
	}
	if !strings.Contains(output, "not initialized") {
		t.Errorf("output missing rig beads status: %s", output)
	}
}

func TestDoRigList_Empty(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := `[workspace]
name = "test-city"

[[agents]]
name = "mayor"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigList(fsys.OSFS{}, cityPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigList returned %d, stderr: %s", code, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "test-city (HQ)") {
		t.Errorf("output missing HQ: %s", output)
	}
	// Should not contain any rig entries beyond HQ.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Path:") {
			t.Errorf("should have no rig paths when empty, got line: %s", line)
		}
	}
}

func TestCollectRigRoutes(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "my-city"},
		Rigs: []config.Rig{
			{Name: "frontend", Path: "/home/user/frontend", Prefix: "fe"},
			{Name: "backend", Path: "/home/user/backend"},
		},
	}

	routes := collectRigRoutes("/home/user/my-city", cfg)
	if len(routes) != 3 {
		t.Fatalf("len(routes) = %d, want 3", len(routes))
	}
	// HQ
	if routes[0].Prefix != "mc" {
		t.Errorf("HQ prefix = %q, want %q", routes[0].Prefix, "mc")
	}
	if routes[0].AbsDir != "/home/user/my-city" {
		t.Errorf("HQ dir = %q, want %q", routes[0].AbsDir, "/home/user/my-city")
	}
	// Frontend (explicit prefix).
	if routes[1].Prefix != "fe" {
		t.Errorf("frontend prefix = %q, want %q", routes[1].Prefix, "fe")
	}
	// Backend (derived prefix).
	if routes[2].Prefix != "ba" {
		t.Errorf("backend prefix = %q, want %q", routes[2].Prefix, "ba")
	}
}
