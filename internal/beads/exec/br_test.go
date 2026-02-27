//go:build integration

package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
)

func TestBrProviderConformance(t *testing.T) {
	brPath, err := exec.LookPath("br")
	if err != nil {
		t.Skip("br not available")
	}

	scriptPath := findGcBeadsBr(t)

	beadstest.RunStoreTests(t, func() beads.Store {
		dir := t.TempDir()
		initBr(t, brPath, dir)
		s := NewStore(scriptPath)
		s.SetEnv(map[string]string{"BR_DIR": dir})
		return s
	})
}

// findGcBeadsBr resolves the path to contrib/beads-scripts/gc-beads-br
// by walking up from the working directory to find the project root (go.mod).
func findGcBeadsBr(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			scriptPath := filepath.Join(dir, "contrib", "beads-scripts", "gc-beads-br")
			if _, err := os.Stat(scriptPath); err != nil {
				t.Fatalf("gc-beads-br not found at %s: %v", scriptPath, err)
			}
			return scriptPath
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod)")
		}
		dir = parent
	}
}

// initBr runs `br init` in the given directory to set up a beads_rust store.
func initBr(t *testing.T, brPath, dir string) {
	t.Helper()
	cmd := exec.Command(brPath, "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("br init failed: %v\n%s", err, out)
	}
}
