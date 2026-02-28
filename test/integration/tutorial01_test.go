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

	// Bead CRUD — use bd directly.
	out, err := bd(cityDir, "create", "Build a Tower of Hanoi app")
	if err != nil {
		t.Fatalf("bd create failed: %v\noutput: %s", err, out)
	}

	// Extract bead ID from bd create output.
	beadID := extractBeadID(t, out)

	// List beads.
	out, err = bd(cityDir, "list")
	if err != nil {
		t.Fatalf("bd list failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, beadID) {
		t.Errorf("expected bead %q in list output, got: %s", beadID, out)
	}

	// Show bead.
	out, err = bd(cityDir, "show", beadID)
	if err != nil {
		t.Fatalf("bd show failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Tower of Hanoi") {
		t.Errorf("expected 'Tower of Hanoi' in show output, got: %s", out)
	}

	// Ready beads (should include our open bead).
	out, err = bd(cityDir, "ready")
	if err != nil {
		t.Fatalf("bd ready failed: %v\noutput: %s", err, out)
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
	out, err = bd(cityDir, "close", beadID)
	if err != nil {
		t.Fatalf("bd close failed: %v\noutput: %s", err, out)
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

	// Create a bead and claim it for the agent.
	out, err := bd(cityDir, "create", "Build a Tower of Hanoi app")
	if err != nil {
		t.Fatalf("bd create failed: %v\noutput: %s", err, out)
	}
	beadID := extractBeadID(t, out)

	out, err = gc(cityDir, "agent", "claim", "mayor", beadID)
	if err != nil {
		t.Fatalf("gc agent claim failed: %v\noutput: %s", err, out)
	}

	// Poll until the bead is closed (agent processed it).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, _ = bd(cityDir, "show", beadID)
		if strings.Contains(out, "closed") {
			t.Logf("Bead closed: %s", out)

			out, err = gc("", "stop", cityDir)
			if err != nil {
				t.Fatalf("gc stop failed: %v\noutput: %s", err, out)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	beadShow, _ := bd(cityDir, "show", beadID)
	beadList, _ := bd(cityDir, "list")
	t.Fatalf("timed out waiting for bead close\nbead show:\n%s\nbead list:\n%s", beadShow, beadList)
}

// extractBeadID parses a bead ID from bd create output.
func extractBeadID(t *testing.T, output string) string {
	t.Helper()
	// Look for "Created bead: <id>" (file store format).
	prefix := "Created bead: "
	if idx := strings.Index(output, prefix); idx >= 0 {
		rest := output[idx+len(prefix):]
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	// Look for "Created issue: <id>" (bd CLI format).
	issuePrefix := "Created issue: "
	if idx := strings.Index(output, issuePrefix); idx >= 0 {
		rest := output[idx+len(issuePrefix):]
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	// bd CLI may output differently — try to find an ID pattern.
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "bd-") || strings.HasPrefix(line, "gc-") {
			fields := strings.Fields(line)
			return fields[0]
		}
	}
	t.Fatalf("could not parse bead ID from output: %s", output)
	return ""
}
