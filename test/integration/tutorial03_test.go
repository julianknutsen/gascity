//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// TestTutorial03_BashAgent validates the Tutorial 03 (Ralph Loop) flow:
// a loop agent that drains the backlog by self-claiming beads from the
// ready queue. The bash script (test/agents/loop.sh) implements
// prompts/loop.md:
//
//  1. Check claim for already-assigned work
//  2. If nothing claimed, check ready queue
//  3. Claim first available bead
//  4. Close it
//  5. Repeat
//
// Three beads are created; the agent should drain them all without any
// external nudging.
func TestTutorial03_BashAgent(t *testing.T) {
	agents := []agentConfig{
		{Name: "mayor", StartCommand: "bash " + agentScript("loop.sh")},
	}

	var cityDir string
	if usingSubprocess() {
		cityDir = setupCityNoGuard(t, agents)
	} else {
		guard := tmuxtest.NewGuard(t)
		cityDir = setupCity(t, guard, agents)
		if !guard.HasSession(guard.SessionName("mayor")) {
			t.Fatal("expected mayor tmux session after gc start")
		}
	}

	// Create three beads — the agent should drain them all.
	var beadIDs []string
	for _, title := range []string{
		"Implement 3-disk solver",
		"Add animation to disc moves",
		"Write unit tests for the solver",
	} {
		out, err := bd(cityDir, "create", title)
		if err != nil {
			t.Fatalf("bd create failed: %v\noutput: %s", err, out)
		}
		beadIDs = append(beadIDs, extractBeadID(t, out))
	}

	// Poll until all three beads are closed.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		allClosed := true
		for _, id := range beadIDs {
			out, _ := bd(cityDir, "show", id)
			if !strings.Contains(out, "Status:   closed") {
				allClosed = false
				break
			}
		}
		if allClosed {
			t.Logf("All %d beads closed", len(beadIDs))
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	beadList, _ := bd(cityDir, "list")
	t.Fatalf("timed out waiting for all beads to close\nbead list:\n%s", beadList)
}
