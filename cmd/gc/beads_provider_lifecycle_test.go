package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureBeadsProvider_file verifies that file provider is a no-op.
func TestEnsureBeadsProvider_file(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	if err := ensureBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestEnsureBeadsProvider_exec calls script with ensure-ready, exit 2 = no-op.
func TestEnsureBeadsProvider_exec(t *testing.T) {
	script := writeTestScript(t, "ensure-ready", 2, "")
	t.Setenv("GC_BEADS", "exec:"+script)
	if err := ensureBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil for exit 2, got %v", err)
	}
}

// TestEnsureBeadsProvider_bd_skip verifies bd provider is no-op when GC_DOLT=skip.
func TestEnsureBeadsProvider_bd_skip(t *testing.T) {
	t.Setenv("GC_BEADS", "bd")
	t.Setenv("GC_DOLT", "skip")
	if err := ensureBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestShutdownBeadsProvider_file verifies that file provider is a no-op.
func TestShutdownBeadsProvider_file(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	if err := shutdownBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestShutdownBeadsProvider_exec calls script with shutdown, exit 2 = no-op.
func TestShutdownBeadsProvider_exec(t *testing.T) {
	script := writeTestScript(t, "shutdown", 2, "")
	t.Setenv("GC_BEADS", "exec:"+script)
	if err := shutdownBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil for exit 2, got %v", err)
	}
}

// TestShutdownBeadsProvider_bd_skip verifies bd provider is no-op when GC_DOLT=skip.
func TestShutdownBeadsProvider_bd_skip(t *testing.T) {
	t.Setenv("GC_BEADS", "bd")
	t.Setenv("GC_DOLT", "skip")
	if err := shutdownBeadsProvider(t.TempDir()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestInitBeadsForDir_file verifies that file provider is a no-op.
func TestInitBeadsForDir_file(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	if err := initBeadsForDir(t.TempDir(), t.TempDir(), "test"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestInitBeadsForDir_exec calls script with init <dir> <prefix>.
func TestInitBeadsForDir_exec(t *testing.T) {
	script := writeTestScript(t, "init", 2, "")
	t.Setenv("GC_BEADS", "exec:"+script)
	if err := initBeadsForDir(t.TempDir(), "/some/dir", "prefix"); err != nil {
		t.Fatalf("expected nil for exit 2, got %v", err)
	}
}

// TestInitBeadsForDir_bd_skip verifies bd provider is no-op when GC_DOLT=skip.
func TestInitBeadsForDir_bd_skip(t *testing.T) {
	t.Setenv("GC_BEADS", "bd")
	t.Setenv("GC_DOLT", "skip")
	if err := initBeadsForDir(t.TempDir(), t.TempDir(), "test"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestRunProviderOp_exit2 verifies exit 2 is treated as success (not needed).
func TestRunProviderOp_exit2(t *testing.T) {
	script := writeTestScript(t, "", 2, "")
	if err := runProviderOp(script, "ensure-ready"); err != nil {
		t.Fatalf("expected nil for exit 2, got %v", err)
	}
}

// TestRunProviderOp_exit0 verifies exit 0 is success.
func TestRunProviderOp_exit0(t *testing.T) {
	script := writeTestScript(t, "", 0, "")
	if err := runProviderOp(script, "ensure-ready"); err != nil {
		t.Fatalf("expected nil for exit 0, got %v", err)
	}
}

// TestRunProviderOp_error verifies exit 1 propagates the error with stderr.
func TestRunProviderOp_error(t *testing.T) {
	script := writeTestScript(t, "", 1, "server crashed")
	err := runProviderOp(script, "ensure-ready")
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if got := err.Error(); got != "exec beads ensure-ready: server crashed" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

// TestRunProviderOp_errorNoStderr verifies exit 1 with no stderr uses exec error.
func TestRunProviderOp_errorNoStderr(t *testing.T) {
	script := writeTestScript(t, "", 1, "")
	err := runProviderOp(script, "shutdown")
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error")
	}
}

// writeTestScript creates a shell script that exits with the given code.
// If stderrMsg is non-empty, the script writes it to stderr before exiting.
func writeTestScript(t *testing.T, _ string, exitCode int, stderrMsg string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "test-beads.sh")

	content := "#!/bin/sh\n"
	if stderrMsg != "" {
		content += "echo '" + stderrMsg + "' >&2\n"
	}
	content += "exit " + itoa(exitCode) + "\n"

	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// itoa is a simple int to string converter for test scripts.
func itoa(n int) string {
	return []string{"0", "1", "2"}[n]
}
