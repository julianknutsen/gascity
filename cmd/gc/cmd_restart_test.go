package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

func TestDoRigRestart(t *testing.T) {
	sp := session.NewFake()
	// Start 2 sessions for agents in the rig.
	// SessionNameFor replaces "/" with "--".
	if err := sp.Start("gc-city-frontend--polecat", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if err := sp.Start("gc-city-frontend--worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := events.NewFake()
	agents := []config.Agent{
		{Name: "polecat", Dir: "frontend"},
		{Name: "worker", Dir: "frontend"},
	}

	var stdout, stderr bytes.Buffer
	code := doRigRestart(sp, rec, agents, "frontend", "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Both sessions should be stopped.
	if sp.IsRunning("gc-city-frontend--polecat") {
		t.Error("polecat session still running")
	}
	if sp.IsRunning("gc-city-frontend--worker") {
		t.Error("worker session still running")
	}

	// 2 AgentStopped events recorded.
	if len(rec.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(rec.Events))
	}
	for _, e := range rec.Events {
		if e.Type != events.AgentStopped {
			t.Errorf("event type = %q, want %q", e.Type, events.AgentStopped)
		}
	}

	// stdout message.
	if got := stdout.String(); !strings.Contains(got, "Restarted 2 agent(s)") {
		t.Errorf("stdout = %q, want to contain 'Restarted 2 agent(s)'", got)
	}
}

func TestDoRigRestartNoneRunning(t *testing.T) {
	sp := session.NewFake() // no sessions started
	rec := events.NewFake()
	agents := []config.Agent{
		{Name: "polecat", Dir: "frontend"},
	}

	var stdout, stderr bytes.Buffer
	code := doRigRestart(sp, rec, agents, "frontend", "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if got := stdout.String(); !strings.Contains(got, "Restarted 0 agent(s)") {
		t.Errorf("stdout = %q, want to contain 'Restarted 0 agent(s)'", got)
	}
	if len(rec.Events) != 0 {
		t.Errorf("got %d events, want 0", len(rec.Events))
	}
}

func TestDoRigRestartWithPool(t *testing.T) {
	sp := session.NewFake()
	// Pool agent with Max=3, only 2 running.
	// SessionNameFor replaces "/" with "--".
	if err := sp.Start("gc-city-frontend--worker-1", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if err := sp.Start("gc-city-frontend--worker-2", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	// worker-3 is NOT running.

	rec := events.NewFake()
	agents := []config.Agent{
		{Name: "worker", Dir: "frontend", Pool: &config.PoolConfig{Min: 1, Max: 3, Check: "echo 2"}},
	}

	var stdout, stderr bytes.Buffer
	code := doRigRestart(sp, rec, agents, "frontend", "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Both running instances should be stopped.
	if sp.IsRunning("gc-city-frontend--worker-1") {
		t.Error("worker-1 still running")
	}
	if sp.IsRunning("gc-city-frontend--worker-2") {
		t.Error("worker-2 still running")
	}

	// 2 events.
	if len(rec.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(rec.Events))
	}

	// Correct count in output.
	if got := stdout.String(); !strings.Contains(got, "Restarted 2 agent(s)") {
		t.Errorf("stdout = %q, want to contain 'Restarted 2 agent(s)'", got)
	}
}

func TestDoRigRestartStopError(t *testing.T) {
	// When Stop fails, the agent is skipped but the command still succeeds.
	sp := session.NewFake()
	if err := sp.Start("gc-city-frontend--polecat", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	wrapper := &stopErrorProvider{Fake: sp, stopErr: fmt.Errorf("tmux borked")}

	rec := events.NewFake()
	agents := []config.Agent{
		{Name: "polecat", Dir: "frontend"},
	}

	var stdout, stderr bytes.Buffer
	code := doRigRestart(wrapper, rec, agents, "frontend", "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	// Error logged to stderr.
	if !strings.Contains(stderr.String(), "tmux borked") {
		t.Errorf("stderr = %q, want to contain 'tmux borked'", stderr.String())
	}
	// 0 killed (stop failed).
	if got := stdout.String(); !strings.Contains(got, "Restarted 0 agent(s)") {
		t.Errorf("stdout = %q, want to contain 'Restarted 0 agent(s)'", got)
	}
}
