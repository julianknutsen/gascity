//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
	"github.com/steveyegge/gascity/internal/dolt"
)

// TestBdStoreConformance runs the beads conformance suite against BdStore
// backed by a real dolt server. This proves the full stack works:
// dolt server → bd CLI → BdStore → beads.Store interface.
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

	cityPath := t.TempDir()
	cityName := "conformance-test"

	// Init dolt for this city (identity, database, server, metadata, bd init).
	if err := dolt.InitCity(cityPath, cityName, os.Stderr); err != nil {
		t.Fatalf("InitCity: %v", err)
	}
	t.Cleanup(func() {
		_ = dolt.StopCity(cityPath)
	})

	// Verify .beads/metadata.json was written.
	metadataPath := filepath.Join(cityPath, ".beads", "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("metadata.json not found: %v", err)
	}

	// Run conformance suite. Each subtest gets a fresh BdStore pointing at
	// the same server. BdStore doesn't have its own state — each bd command
	// is independent — so the factory just creates a new wrapper.
	//
	// NOTE: We don't run RunSequentialIDTests because BdStore uses bd's
	// ID format (bd-XXXX), not the gc-N sequential format.
	beadstest.RunStoreTests(t, func() beads.Store {
		return beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	})
}
