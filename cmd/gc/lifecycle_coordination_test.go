package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSpyScript creates a shell script that logs operations to a file and
// recreates .beads/ on init (simulating bd init wiping hooks). Returns the
// script path.
func writeSpyScript(t *testing.T, logFile string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "spy-beads.sh")

	// The spy logs "op arg1 arg2 ..." to logFile, one line per call.
	// For "init" operations, it also creates .beads/ in the target dir
	// (simulating bd init creating the directory, which wipes hooks).
	content := `#!/bin/sh
echo "$@" >> "` + logFile + `"
case "$1" in
  init)
    # Simulate bd init: create .beads/ (may wipe existing hooks)
    mkdir -p "$2/.beads"
    ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// readOpLog reads the spy script's operation log and returns the lines.
func readOpLog(t *testing.T, logFile string) []string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// TestLifecycleCoordination_InitRigAddStart exercises the real lifecycle code
// paths using GC_BEADS=exec:<spy> to verify ordering and hook survival.
func TestLifecycleCoordination_InitRigAddStart(t *testing.T) {
	// Set up city directory with city.toml.
	cityPath := t.TempDir()
	cityName := "testcity"
	rigPath := filepath.Join(cityPath, "rigs", "myrig")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write minimal city.toml so beadsProvider reads from env.
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"),
		[]byte("[workspace]\nname = \""+cityName+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.Join(t.TempDir(), "ops.log")
	script := writeSpyScript(t, logFile)
	t.Setenv("GC_BEADS", "exec:"+script)

	// Phase 1: gc init — initBeadsForDir for city root.
	prefix := "tc"
	if err := initBeadsForDir(cityPath, cityPath, prefix); err != nil {
		t.Fatalf("initBeadsForDir (city): %v", err)
	}
	if err := installBeadHooks(cityPath); err != nil {
		t.Fatalf("installBeadHooks (city): %v", err)
	}

	ops := readOpLog(t, logFile)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op after init, got %d: %v", len(ops), ops)
	}
	if !strings.HasPrefix(ops[0], "init "+cityPath) {
		t.Fatalf("expected init op for city, got: %s", ops[0])
	}

	// Verify hooks exist at city root.
	for _, hook := range []string{"on_create", "on_close", "on_update"} {
		path := filepath.Join(cityPath, ".beads", "hooks", hook)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("hook %s missing after city init: %v", hook, err)
		}
	}

	// Phase 2: gc rig add — initBeadsForDir for rig.
	rigPrefix := "mr"
	if err := initBeadsForDir(cityPath, rigPath, rigPrefix); err != nil {
		t.Fatalf("initBeadsForDir (rig): %v", err)
	}
	if err := installBeadHooks(rigPath); err != nil {
		t.Fatalf("installBeadHooks (rig): %v", err)
	}

	ops = readOpLog(t, logFile)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops after rig add, got %d: %v", len(ops), ops)
	}
	if !strings.HasPrefix(ops[1], "init "+rigPath) {
		t.Fatalf("expected init op for rig, got: %s", ops[1])
	}

	// Verify hooks exist at rig path.
	for _, hook := range []string{"on_create", "on_close", "on_update"} {
		path := filepath.Join(rigPath, ".beads", "hooks", hook)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("hook %s missing after rig add: %v", hook, err)
		}
	}

	// Phase 3: Simulate hook wipe (bd init recreates .beads/).
	if err := os.RemoveAll(filepath.Join(cityPath, ".beads", "hooks")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(rigPath, ".beads", "hooks")); err != nil {
		t.Fatal(err)
	}

	// Verify hooks are gone.
	if _, err := os.Stat(filepath.Join(cityPath, ".beads", "hooks", "on_create")); !os.IsNotExist(err) {
		t.Fatal("expected city hooks to be gone after wipe")
	}
	if _, err := os.Stat(filepath.Join(rigPath, ".beads", "hooks", "on_create")); !os.IsNotExist(err) {
		t.Fatal("expected rig hooks to be gone after wipe")
	}

	// Phase 4: gc start sequence — ensure-ready, then init + hooks for each.
	if err := ensureBeadsProvider(cityPath); err != nil {
		t.Fatalf("ensureBeadsProvider: %v", err)
	}
	if err := initBeadsForDir(cityPath, cityPath, prefix); err != nil {
		t.Fatalf("initBeadsForDir (city, start): %v", err)
	}
	if err := installBeadHooks(cityPath); err != nil {
		t.Fatalf("installBeadHooks (city, start): %v", err)
	}
	if err := initBeadsForDir(cityPath, rigPath, rigPrefix); err != nil {
		t.Fatalf("initBeadsForDir (rig, start): %v", err)
	}
	if err := installBeadHooks(rigPath); err != nil {
		t.Fatalf("installBeadHooks (rig, start): %v", err)
	}

	ops = readOpLog(t, logFile)
	// Should have: init(city), init(rig), ensure-ready, init(city), init(rig)
	if len(ops) != 5 {
		t.Fatalf("expected 5 ops total, got %d: %v", len(ops), ops)
	}

	// Verify hooks reinstalled at both paths after start.
	for _, dir := range []string{cityPath, rigPath} {
		for _, hook := range []string{"on_create", "on_close", "on_update"} {
			path := filepath.Join(dir, ".beads", "hooks", hook)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("hook %s missing at %s after start: %v", hook, dir, err)
			}
		}
	}
}

// TestLifecycleCoordination_StartOrder verifies that ensure-ready always
// precedes any init call during gc start. This catches bugs where init
// runs before the backing service is ready.
func TestLifecycleCoordination_StartOrder(t *testing.T) {
	cityPath := t.TempDir()
	rigPath := filepath.Join(cityPath, "rigs", "myrig")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"),
		[]byte("[workspace]\nname = \"ordertest\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.Join(t.TempDir(), "ops.log")
	script := writeSpyScript(t, logFile)
	t.Setenv("GC_BEADS", "exec:"+script)

	// Simulate the gc start sequence exactly.
	if err := ensureBeadsProvider(cityPath); err != nil {
		t.Fatalf("ensureBeadsProvider: %v", err)
	}
	if err := initBeadsForDir(cityPath, cityPath, "ot"); err != nil {
		t.Fatalf("initBeadsForDir (city): %v", err)
	}
	if err := initBeadsForDir(cityPath, rigPath, "mr"); err != nil {
		t.Fatalf("initBeadsForDir (rig): %v", err)
	}

	ops := readOpLog(t, logFile)
	if len(ops) < 2 {
		t.Fatalf("expected at least 2 ops, got %d: %v", len(ops), ops)
	}

	// First op must be ensure-ready.
	if !strings.HasPrefix(ops[0], "ensure-ready") {
		t.Fatalf("first op should be ensure-ready, got: %s", ops[0])
	}

	// All subsequent ops must be init.
	for i := 1; i < len(ops); i++ {
		if !strings.HasPrefix(ops[i], "init ") {
			t.Fatalf("op[%d] should be init, got: %s", i, ops[i])
		}
	}
}

// TestLifecycleCoordination_StopOrder verifies that shutdown is called
// during gc stop via shutdownBeadsProvider.
func TestLifecycleCoordination_StopOrder(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"),
		[]byte("[workspace]\nname = \"stoptest\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.Join(t.TempDir(), "ops.log")
	script := writeSpyScript(t, logFile)
	t.Setenv("GC_BEADS", "exec:"+script)

	// Simulate stop: shutdown is called after agents are terminated.
	if err := shutdownBeadsProvider(cityPath); err != nil {
		t.Fatalf("shutdownBeadsProvider: %v", err)
	}

	ops := readOpLog(t, logFile)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %v", len(ops), ops)
	}
	if !strings.HasPrefix(ops[0], "shutdown") {
		t.Fatalf("expected shutdown op, got: %s", ops[0])
	}
}
