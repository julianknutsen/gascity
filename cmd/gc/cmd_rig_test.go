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
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	rigPath := filepath.Join(t.TempDir(), "my-frontend")
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
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"/some/path\"\n"
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
	cityToml := "[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

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
	cityToml := "[workspace]\nname = \"my-city\"\n\n[[agents]]\nname = \"mayor\"\n"
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

// Regression: Bug 1 — city.toml must not be modified if rig infrastructure
// creation fails. This prevents phantom rigs in config.
func TestDoRigAdd_ConfigUnchangedOnInfraFailure(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	originalToml := "[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := os.WriteFile(tomlPath, []byte(originalToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a fake FS that fails on MkdirAll for the rigs/ directory.
	f := fsys.NewFake()
	f.Dirs["/fake-rig"] = true
	f.Files[tomlPath] = []byte(originalToml)
	rigDir := filepath.Join(cityPath, "rigs", "fake-rig")
	f.Errors[rigDir] = os.ErrPermission

	var stdout, stderr bytes.Buffer
	code := doRigAdd(f, cityPath, "/fake-rig", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected failure, got code %d", code)
	}

	// Verify city.toml was NOT modified.
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fake-rig") {
		t.Errorf("city.toml should be unchanged after infrastructure failure:\n%s", data)
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

	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"my-frontend\"\npath = \"" + rigPath + "\"\nprefix = \"fe\"\n"
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
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n"
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
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Path:") {
			t.Errorf("should have no rig paths when empty, got line: %s", line)
		}
	}
}

// Regression: Bug 6 — resolveRigForAgent should match agents to rigs.
func TestResolveRigForAgent(t *testing.T) {
	rigs := []config.Rig{
		{Name: "frontend", Path: "/home/user/frontend"},
		{Name: "backend", Path: "/home/user/backend"},
	}

	if got := resolveRigForAgent("/home/user/frontend", rigs); got != "frontend" {
		t.Errorf("resolveRigForAgent(frontend path) = %q, want %q", got, "frontend")
	}
	if got := resolveRigForAgent("/home/user/backend", rigs); got != "backend" {
		t.Errorf("resolveRigForAgent(backend path) = %q, want %q", got, "backend")
	}
	if got := resolveRigForAgent("/home/user/other", rigs); got != "" {
		t.Errorf("resolveRigForAgent(unmatched path) = %q, want empty", got)
	}
	if got := resolveRigForAgent("/home/user/frontend", nil); got != "" {
		t.Errorf("resolveRigForAgent(nil rigs) = %q, want empty", got)
	}
}

// Regression: trailing slash in rig path must still match.
func TestResolveRigForAgent_TrailingSlash(t *testing.T) {
	rigs := []config.Rig{
		{Name: "frontend", Path: "/home/user/frontend/"},
	}
	if got := resolveRigForAgent("/home/user/frontend", rigs); got != "frontend" {
		t.Errorf("resolveRigForAgent(no trailing slash) = %q, want %q", got, "frontend")
	}

	// Also test workDir with trailing slash, rig path without.
	rigs2 := []config.Rig{
		{Name: "backend", Path: "/home/user/backend"},
	}
	if got := resolveRigForAgent("/home/user/backend/", rigs2); got != "backend" {
		t.Errorf("resolveRigForAgent(trailing slash workDir) = %q, want %q", got, "backend")
	}
}

// ---------------------------------------------------------------------------
// gc rig suspend / resume tests
// ---------------------------------------------------------------------------

func TestDoRigSuspend(t *testing.T) {
	cityPath := t.TempDir()
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"/some/path\"\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigSuspend(fsys.OSFS{}, cityPath, "frontend", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigSuspend returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Suspended rig 'frontend'") {
		t.Errorf("output = %q, want suspend message", stdout.String())
	}

	// Verify config written with suspended=true.
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rigs) != 1 || !cfg.Rigs[0].Suspended {
		t.Errorf("rig should be suspended, got %+v", cfg.Rigs)
	}
}

func TestDoRigSuspendNotFound(t *testing.T) {
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n"
	f := fsys.NewFake()
	f.Files["/city/city.toml"] = []byte(cityToml)

	var stdout, stderr bytes.Buffer
	code := doRigSuspend(f, "/city", "nonexistent", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doRigSuspend should fail for unknown rig, got code %d", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want not found message", stderr.String())
	}
}

func TestDoRigSuspendAlreadySuspended(t *testing.T) {
	cityPath := t.TempDir()
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"/some/path\"\nsuspended = true\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigSuspend(fsys.OSFS{}, cityPath, "frontend", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigSuspend should be idempotent, got code %d, stderr: %s", code, stderr.String())
	}
}

func TestDoRigResume(t *testing.T) {
	cityPath := t.TempDir()
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"/some/path\"\nsuspended = true\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigResume(fsys.OSFS{}, cityPath, "frontend", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigResume returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Resumed rig 'frontend'") {
		t.Errorf("output = %q, want resume message", stdout.String())
	}

	// Verify config written with suspended=false.
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rigs) != 1 || cfg.Rigs[0].Suspended {
		t.Errorf("rig should not be suspended, got %+v", cfg.Rigs)
	}
}

func TestDoRigResumeNotFound(t *testing.T) {
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n"
	f := fsys.NewFake()
	f.Files["/city/city.toml"] = []byte(cityToml)

	var stdout, stderr bytes.Buffer
	code := doRigResume(f, "/city", "nonexistent", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doRigResume should fail for unknown rig, got code %d", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want not found message", stderr.String())
	}
}

func TestDoRigResumeNotSuspended(t *testing.T) {
	cityPath := t.TempDir()
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"/some/path\"\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigResume(fsys.OSFS{}, cityPath, "frontend", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigResume should be idempotent, got code %d, stderr: %s", code, stderr.String())
	}
}

func TestDoRigListShowsSuspended(t *testing.T) {
	cityPath := t.TempDir()
	rigPath := filepath.Join(t.TempDir(), "my-frontend")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"my-frontend\"\npath = \"" + rigPath + "\"\nsuspended = true\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigList(fsys.OSFS{}, cityPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigList returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "my-frontend (suspended)") {
		t.Errorf("output = %q, want suspended annotation", stdout.String())
	}
}

func TestDoAgentListShowsRigSuspended(t *testing.T) {
	rigPath := t.TempDir()
	cityPath := t.TempDir()
	cityToml := "[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[agents]]\nname = \"polecat\"\ndir = \"" + rigPath + "\"\n\n[[rigs]]\nname = \"frontend\"\npath = \"" + rigPath + "\"\nsuspended = true\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doAgentList(fsys.OSFS{}, cityPath, "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentList returned %d, stderr: %s", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "rig suspended") {
		t.Errorf("output = %q, want 'rig suspended' annotation", output)
	}
	// Mayor should NOT show rig suspended.
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "mayor") && strings.Contains(line, "rig suspended") {
			t.Errorf("mayor should not show rig suspended: %s", line)
		}
	}
}

// Regression: doRigAdd must reject rigs with colliding prefixes.
func TestDoRigAdd_PrefixCollision(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// City "my-city" (prefix "mc") already has rig "my-frontend" (prefix "mf").
	cityToml := "[workspace]\nname = \"my-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"my-frontend\"\npath = \"/some/path\"\n"
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(cityToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to add "my-foo" — derives prefix "mf", collides with "my-frontend".
	rigPath := filepath.Join(t.TempDir(), "my-foo")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GC_DOLT", "skip")
	t.Setenv("GC_BEADS", "file")

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doRigAdd should fail for prefix collision, got code %d", code)
	}
	if !strings.Contains(stderr.String(), "collides") {
		t.Errorf("stderr should mention collision: %s", stderr.String())
	}
}
