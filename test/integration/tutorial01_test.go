//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

func TestTutorial01_StartCreatesSession(t *testing.T) {
	if usingSubprocess() {
		t.Skip("tmux-specific test")
	}
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)
	_ = cityDir

	mayorSession := guard.SessionName("mayor")
	if !guard.HasSession(mayorSession) {
		t.Errorf("expected tmux session %q after gc start", mayorSession)
	}
}

func TestTutorial01_StopKillsSession(t *testing.T) {
	if usingSubprocess() {
		t.Skip("tmux-specific test")
	}
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
	if usingSubprocess() {
		t.Skip("tmux-specific test")
	}
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
	if usingSubprocess() {
		t.Skip("tmux-specific test")
	}
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
	if usingSubprocess() {
		t.Skip("tmux-specific test")
	}
	guard := tmuxtest.NewGuard(t)
	cityDir := setupRunningCity(t, guard)

	// Bead CRUD — run from inside the city directory.
	out, err := gc(cityDir, "bd", "create", "Build a Tower of Hanoi app")
	if err != nil {
		t.Fatalf("gc bd create failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "status: open") {
		t.Errorf("expected 'status: open' in bead create output, got: %s", out)
	}

	// Extract bead ID from output like "Created bead: gc-1  (status: open)"
	beadID := extractBeadID(t, out)

	// List beads.
	out, err = gc(cityDir, "bd", "list")
	if err != nil {
		t.Fatalf("gc bd list failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, beadID) {
		t.Errorf("expected bead %q in list output, got: %s", beadID, out)
	}

	// Show bead.
	out, err = gc(cityDir, "bd", "show", beadID)
	if err != nil {
		t.Fatalf("gc bd show failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Tower of Hanoi") {
		t.Errorf("expected 'Tower of Hanoi' in show output, got: %s", out)
	}

	// Ready beads (should include our open bead).
	out, err = gc(cityDir, "bd", "ready")
	if err != nil {
		t.Fatalf("gc bd ready failed: %v\noutput: %s", err, out)
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
	out, err = gc(cityDir, "bd", "close", beadID)
	if err != nil {
		t.Fatalf("gc bd close failed: %v\noutput: %s", err, out)
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

// TestTutorial01_BashAgent validates the one-shot prompt flow using a bash
// script as the agent. The bash script (test/agents/one-shot.sh) implements
// prompts/one-shot.md:
//
//  1. Agent polls its hook for assigned work
//  2. Finds hooked bead, closes it
//  3. Exits after processing one bead
//
// This is the Tutorial 01 experience: a single agent processes a single bead.
func TestTutorial01_BashAgent(t *testing.T) {
	var cityDir string
	if usingSubprocess() {
		cityDir = setupCityNoGuard(t, []agentConfig{
			{Name: "mayor", StartCommand: "bash " + agentScript("one-shot.sh")},
		})
	} else {
		guard := tmuxtest.NewGuard(t)
		cityDir = setupCity(t, guard, []agentConfig{
			{Name: "mayor", StartCommand: "bash " + agentScript("one-shot.sh")},
		})
		if !guard.HasSession(guard.SessionName("mayor")) {
			t.Fatal("expected mayor tmux session after gc start")
		}
	}

	// Create a bead and hook it to the agent.
	out, err := gc(cityDir, "bd", "create", "Build a Tower of Hanoi app")
	if err != nil {
		t.Fatalf("gc bd create failed: %v\noutput: %s", err, out)
	}
	beadID := extractBeadID(t, out)

	out, err = gc(cityDir, "agent", "hook", "mayor", beadID)
	if err != nil {
		t.Fatalf("gc agent hook failed: %v\noutput: %s", err, out)
	}

	// Poll until the bead is closed (agent processed it).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, _ = gc(cityDir, "bd", "show", beadID)
		if strings.Contains(out, "Status:   closed") {
			t.Logf("Bead closed: %s", out)

			out, err = gc("", "stop", cityDir)
			if err != nil {
				t.Fatalf("gc stop failed: %v\noutput: %s", err, out)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	beadShow, _ := gc(cityDir, "bd", "show", beadID)
	beadList, _ := gc(cityDir, "bd", "list")
	t.Fatalf("timed out waiting for bead close\nbead show:\n%s\nbead list:\n%s", beadShow, beadList)
}

// extractBeadID parses a bead ID from gc bd create output like
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
