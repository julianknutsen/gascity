//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
	"github.com/steveyegge/gascity/internal/dolt"
)

// TestBdStoreConformance runs the beads conformance suite against BdStore
// backed by a real dolt server. This proves the full stack works:
// dolt server → bd CLI → BdStore → beads.Store interface.
//
// Each subtest gets a fresh database on the shared dolt server by creating
// a new workspace directory with its own bd init + unique prefix.
//
// Requires: dolt and bd installed, no other dolt server on port 3307.
func TestBdStoreConformance(t *testing.T) {
	// Skip if dolt or bd not installed.
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed")
	}
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed")
	}

	// Set up a shared dolt server. We use a single city for the server
	// but create separate workspaces (each with their own bd init) so
	// each newStore() call gets an isolated database.
	serverDir := t.TempDir()
	dataDir := filepath.Join(serverDir, ".gc", "dolt-data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := dolt.EnsureDoltIdentity(); err != nil {
		t.Fatalf("dolt identity: %v", err)
	}

	config := dolt.GasCityConfig(serverDir)
	// Start dolt server by writing state files and launching process.
	// We use the low-level approach so we control the data dir.
	logFile := filepath.Join(serverDir, ".gc", "dolt.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("dolt", "sql-server",
		"--port", fmt.Sprintf("%d", config.Port),
		"--data-dir", dataDir,
		"--max-connections", "50",
	)
	log, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatalf("starting dolt server: %v", err)
	}
	log.Close()

	// Write PID file so IsRunning/Stop work.
	pidFile := filepath.Join(serverDir, ".gc", "dolt.pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644)

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Wait for server to accept connections.
	for i := 0; i < 20; i++ {
		if err := dolt.CheckServerReachable(serverDir); err == nil {
			break
		}
		if i == 19 {
			t.Fatal("dolt server not reachable after 10s")
		}
		// Small sleep between retries.
		exec.Command("sleep", "0.5").Run()
	}

	var dbCounter atomic.Int64

	// Factory: each call creates a fresh workspace with its own bd init
	// and unique database on the shared server.
	newStore := func() beads.Store {
		n := dbCounter.Add(1)
		prefix := fmt.Sprintf("ct%d", n)

		// Create isolated workspace directory.
		wsDir := filepath.Join(serverDir, fmt.Sprintf("ws-%d", n))
		if err := os.MkdirAll(wsDir, 0o755); err != nil {
			t.Fatalf("creating workspace: %v", err)
		}

		// Initialize git repo (bd init requires it).
		gitCmd := exec.Command("git", "init", "--quiet")
		gitCmd.Dir = wsDir
		if out, err := gitCmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}

		// Run bd init --server with unique prefix (creates unique database).
		bdInit := exec.Command("bd", "init", "--server", "-p", prefix, "--skip-hooks")
		bdInit.Dir = wsDir
		if out, err := bdInit.CombinedOutput(); err != nil {
			t.Fatalf("bd init: %v: %s", err, out)
		}

		// Explicitly set issue_prefix (required for bd create).
		bdConfig := exec.Command("bd", "config", "set", "issue_prefix", prefix)
		bdConfig.Dir = wsDir
		if out, err := bdConfig.CombinedOutput(); err != nil {
			t.Fatalf("bd config set: %v: %s", err, out)
		}

		return beads.NewBdStore(wsDir, beads.ExecCommandRunner())
	}

	// Run conformance suite. We skip RunSequentialIDTests because BdStore
	// uses bd's ID format (prefix-XXXX), not gc-N sequential format.
	beadstest.RunStoreTests(t, newStore)
}
