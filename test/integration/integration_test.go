//go:build integration

// Package integration provides end-to-end tests that exercise the real gc
// binary with real tmux sessions. These tests validate the tutorial 01
// experience: gc init, gc start, gc stop, bead CRUD, etc.
//
// Session safety: all test cities use the "gctest-<8hex>" naming prefix.
// Three layers of cleanup (pre-sweep, per-test t.Cleanup, post-sweep)
// prevent orphan tmux sessions on developer boxes.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// gcBinary is the path to the built gc binary, set by TestMain.
var gcBinary string

// TestMain builds the gc binary and runs pre/post sweeps of orphan sessions.
func TestMain(m *testing.M) {
	// Check tmux availability — if not installed, skip all tests.
	if _, err := exec.LookPath("tmux"); err != nil {
		// Cannot call t.Skip from TestMain, so just exit cleanly.
		os.Exit(0)
	}

	// Pre-sweep: kill any orphaned gc-gctest-* sessions from prior crashes.
	// Use a minimal TB shim since we don't have a *testing.T yet.
	tmuxtest.KillAllTestSessions(&mainTB{})

	// Build gc binary to a temp directory.
	tmpDir, err := os.MkdirTemp("", "gc-integration-*")
	if err != nil {
		panic("integration: creating temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	gcBinary = filepath.Join(tmpDir, "gc")
	buildCmd := exec.Command("go", "build", "-o", gcBinary, "./cmd/gc")
	buildCmd.Dir = findModuleRoot()
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		panic("integration: building gc binary: " + err.Error() + "\n" + string(out))
	}

	// Run tests.
	code := m.Run()

	// Post-sweep: clean up any sessions that survived individual test cleanup.
	tmuxtest.KillAllTestSessions(&mainTB{})

	os.Exit(code)
}

// gc runs the gc binary with the given args. If dir is non-empty, it sets
// the working directory. Returns combined stdout+stderr and any error.
func gc(dir string, args ...string) (string, error) {
	cmd := exec.Command(gcBinary, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Ensure we use real tmux, not a fake.
	cmd.Env = filterEnv(os.Environ(), "GC_SESSION")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// findModuleRoot walks up from the current directory to find go.mod.
func findModuleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic("integration: getting cwd: " + err.Error())
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("integration: go.mod not found")
		}
		dir = parent
	}
}

// filterEnv returns env with the named variable removed.
func filterEnv(env []string, name string) []string {
	prefix := name + "="
	result := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		result = append(result, e)
	}
	return result
}

// mainTB is a minimal testing.TB implementation for use in TestMain where
// no *testing.T is available. Only Helper() and Logf() are called by
// KillAllTestSessions.
type mainTB struct{ testing.TB }

func (mainTB) Helper()                         {}
func (mainTB) Logf(format string, args ...any) {}
