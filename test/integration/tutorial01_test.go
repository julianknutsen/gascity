//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

func TestTutorial01_StartCreatesSession(t *testing.T) {
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)
	_ = cityDir

	mayorSession := guard.SessionName("mayor")
	if !guard.HasSession(mayorSession) {
		t.Errorf("expected tmux session %q after gc start", mayorSession)
	}
}

func TestTutorial01_StopKillsSession(t *testing.T) {
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)

	out, err := gc("", "stop", cityDir)
	if err != nil {
		t.Fatalf("gc stop failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("expected 'City stopped.' in output, got: %s", out)
	}

	// Give tmux a moment to clean up.
	time.Sleep(200 * time.Millisecond)

	if guard.HasSession(guard.SessionName("mayor")) {
		t.Errorf("session %q should not exist after gc stop", guard.SessionName("mayor"))
	}
}

func TestTutorial01_StopIsIdempotent(t *testing.T) {
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)

	// Stop once.
	out, err := gc("", "stop", cityDir)
	if err != nil {
		t.Fatalf("first gc stop failed: %v\noutput: %s", err, out)
	}
	time.Sleep(200 * time.Millisecond)

	// Stop again — should still succeed, not error.
	out, err = gc("", "stop", cityDir)
	if err != nil {
		t.Fatalf("second gc stop failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("expected 'City stopped.' in output, got: %s", out)
	}
}

func TestTutorial01_StartIsIdempotent(t *testing.T) {
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)

	// Start again — should not error, session still exists.
	out, err := gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("second gc start failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("expected 'City started.' in output, got: %s", out)
	}

	mayorSession := guard.SessionName("mayor")
	if !guard.HasSession(mayorSession) {
		t.Errorf("session %q should still exist after second start", mayorSession)
	}
}

func TestTutorial01_FullFlow(t *testing.T) {
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)

	// Bead CRUD — run from inside the city directory.
	out, err := gc(cityDir, "bead", "create", "Build a Tower of Hanoi app")
	if err != nil {
		t.Fatalf("gc bead create failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "status: open") {
		t.Errorf("expected 'status: open' in bead create output, got: %s", out)
	}

	// Extract bead ID from output like "Created bead: gc-1  (status: open)"
	beadID := extractBeadID(t, out)

	// List beads.
	out, err = gc(cityDir, "bead", "list")
	if err != nil {
		t.Fatalf("gc bead list failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, beadID) {
		t.Errorf("expected bead %q in list output, got: %s", beadID, out)
	}

	// Show bead.
	out, err = gc(cityDir, "bead", "show", beadID)
	if err != nil {
		t.Fatalf("gc bead show failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Tower of Hanoi") {
		t.Errorf("expected 'Tower of Hanoi' in show output, got: %s", out)
	}

	// Ready beads (should include our open bead).
	out, err = gc(cityDir, "bead", "ready")
	if err != nil {
		t.Fatalf("gc bead ready failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, beadID) {
		t.Errorf("expected bead %q in ready output, got: %s", beadID, out)
	}

	// Verify session exists.
	mayorSession := guard.SessionName("mayor")
	if !guard.HasSession(mayorSession) {
		t.Errorf("session %q should exist during full flow", mayorSession)
	}

	// Close the bead.
	out, err = gc(cityDir, "bead", "close", beadID)
	if err != nil {
		t.Fatalf("gc bead close failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Closed bead") {
		t.Errorf("expected 'Closed bead' in close output, got: %s", out)
	}

	// Stop the city.
	out, err = gc("", "stop", cityDir)
	if err != nil {
		t.Fatalf("gc stop failed: %v\noutput: %s", err, out)
	}
	time.Sleep(200 * time.Millisecond)

	// Verify session is gone.
	if guard.HasSession(mayorSession) {
		t.Errorf("session %q should not exist after gc stop", mayorSession)
	}
}

// extractBeadID parses a bead ID from gc bead create output like
// "Created bead: gc-1  (status: open)".
func extractBeadID(t *testing.T, output string) string {
	t.Helper()
	// Look for "Created bead: <id>"
	prefix := "Created bead: "
	idx := strings.Index(output, prefix)
	if idx < 0 {
		t.Fatalf("could not find %q in output: %s", prefix, output)
	}
	rest := output[idx+len(prefix):]
	// ID ends at whitespace.
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		t.Fatalf("could not parse bead ID from: %s", rest)
	}
	return fields[0]
}
