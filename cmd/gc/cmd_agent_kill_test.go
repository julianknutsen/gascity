package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

// ---------------------------------------------------------------------------
// doAgentKill tests
// ---------------------------------------------------------------------------

func TestDoAgentKill(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := events.NewFake()
	var stdout, stderr bytes.Buffer
	code := doAgentKill(sp, rec, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	// Session should be stopped.
	if sp.IsRunning("gc-city-worker") {
		t.Error("session still running after kill")
	}
	// Event recorded.
	if len(rec.Events) != 1 || rec.Events[0].Type != events.AgentStopped {
		t.Errorf("events = %v, want one AgentStopped event", rec.Events)
	}
	if rec.Events[0].Subject != "worker" {
		t.Errorf("event subject = %q, want %q", rec.Events[0].Subject, "worker")
	}
	// stdout message.
	if got := stdout.String(); got != "Killed agent 'worker'\n" {
		t.Errorf("stdout = %q, want %q", got, "Killed agent 'worker'\n")
	}
}

func TestDoAgentKillNotRunning(t *testing.T) {
	sp := session.NewFake() // no sessions started

	var stdout, stderr bytes.Buffer
	code := doAgentKill(sp, events.Discard, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got := stderr.String(); got != "gc agent kill: agent \"worker\" is not running\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestDoAgentKillStopError(t *testing.T) {
	sp := session.NewFailFake()
	// FailFake returns false for IsRunning, so we need a custom approach.
	// Use a regular fake and inject a stop error via a wrapper.
	sp2 := session.NewFake()
	if err := sp2.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	_ = sp // unused

	// Use a stopErrorProvider that wraps Fake but fails on Stop.
	wrapper := &stopErrorProvider{Fake: sp2, stopErr: errors.New("tmux borked")}

	var stdout, stderr bytes.Buffer
	code := doAgentKill(wrapper, events.Discard, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got := stderr.String(); got != "gc agent kill: tmux borked\n" {
		t.Errorf("stderr = %q", got)
	}
}

// stopErrorProvider wraps session.Fake but returns an error on Stop.
type stopErrorProvider struct {
	*session.Fake
	stopErr error
}

func (s *stopErrorProvider) Stop(_ string) error {
	return s.stopErr
}
