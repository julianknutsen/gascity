//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// TestTutorial02_BashAgent validates the Tutorial 02 (Named Crew) flow:
// multiple named agents, each with their own hooked beads. Two one-shot
// agents run concurrently; each gets a different bead assigned via hook.
//
// This tests the key Tutorial 02 concepts:
//   - Multiple [[agents]] in city.toml
//   - Hook-based assignment to specific named agents
//   - Each agent processes only its own hooked work
func TestTutorial02_BashAgent(t *testing.T) {
	agents := []agentConfig{
		{Name: "alice", StartCommand: "bash " + agentScript("one-shot.sh")},
		{Name: "bob", StartCommand: "bash " + agentScript("one-shot.sh")},
	}

	var cityDir string
	if usingSubprocess() {
		cityDir = setupCityNoGuard(t, agents)
	} else {
		guard := tmuxtest.NewGuard(t)
		cityDir = setupCity(t, guard, agents)
		for _, name := range []string{"alice", "bob"} {
			if !guard.HasSession(guard.SessionName(name)) {
				t.Fatalf("expected %s tmux session after gc start", name)
			}
		}
	}

	// Create two beads and hook each to a different agent.
	out, err := gc(cityDir, "bd", "create", "Task for Alice")
	if err != nil {
		t.Fatalf("gc bd create failed: %v\noutput: %s", err, out)
	}
	aliceBead := extractBeadID(t, out)

	out, err = gc(cityDir, "bd", "create", "Task for Bob")
	if err != nil {
		t.Fatalf("gc bd create failed: %v\noutput: %s", err, out)
	}
	bobBead := extractBeadID(t, out)

	out, err = gc(cityDir, "agent", "claim", "alice", aliceBead)
	if err != nil {
		t.Fatalf("gc agent claim alice failed: %v\noutput: %s", err, out)
	}

	out, err = gc(cityDir, "agent", "claim", "bob", bobBead)
	if err != nil {
		t.Fatalf("gc agent claim bob failed: %v\noutput: %s", err, out)
	}

	// Poll until both beads are closed.
	deadline := time.Now().Add(10 * time.Second)
	aliceDone, bobDone := false, false
	for time.Now().Before(deadline) {
		if !aliceDone {
			out, _ = gc(cityDir, "bd", "show", aliceBead)
			if strings.Contains(out, "Status:   closed") {
				aliceDone = true
				t.Logf("Alice's bead closed")
			}
		}
		if !bobDone {
			out, _ = gc(cityDir, "bd", "show", bobBead)
			if strings.Contains(out, "Status:   closed") {
				bobDone = true
				t.Logf("Bob's bead closed")
			}
		}
		if aliceDone && bobDone {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	beadList, _ := gc(cityDir, "bd", "list")
	t.Fatalf("timed out waiting for both beads to close (alice=%v, bob=%v)\nbead list:\n%s",
		aliceDone, bobDone, beadList)
}
