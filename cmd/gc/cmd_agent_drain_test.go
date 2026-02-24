package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

// fakeDrainOps is a test double for drainOps.
type fakeDrainOps struct {
	draining map[string]bool
	err      error // injected error for all ops
}

func newFakeDrainOps() *fakeDrainOps {
	return &fakeDrainOps{draining: make(map[string]bool)}
}

func (f *fakeDrainOps) setDrain(sessionName string) error {
	if f.err != nil {
		return f.err
	}
	f.draining[sessionName] = true
	return nil
}

func (f *fakeDrainOps) clearDrain(sessionName string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.draining, sessionName)
	return nil
}

func (f *fakeDrainOps) isDraining(sessionName string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.draining[sessionName], nil
}

func TestDoAgentDrain(t *testing.T) {
	dops := newFakeDrainOps()
	sp := session.NewFake()
	// Start a session so IsRunning returns true.
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingRecorder{}
	var stdout, stderr bytes.Buffer
	code := doAgentDrain(dops, sp, rec, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !dops.draining["gc-city-worker"] {
		t.Error("drain flag not set")
	}
	if got := stdout.String(); got != "Draining agent 'worker'\n" {
		t.Errorf("stdout = %q, want %q", got, "Draining agent 'worker'\n")
	}
	if len(rec.events) != 1 || rec.events[0].Type != events.AgentDraining {
		t.Errorf("events = %v, want one AgentDraining event", rec.events)
	}
	if rec.events[0].Subject != "worker" {
		t.Errorf("event subject = %q, want %q", rec.events[0].Subject, "worker")
	}
}

func TestDoAgentDrainNotRunning(t *testing.T) {
	dops := newFakeDrainOps()
	sp := session.NewFake() // no sessions started

	var stdout, stderr bytes.Buffer
	code := doAgentDrain(dops, sp, events.Discard, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got := stderr.String(); got != "gc agent drain: agent \"worker\" is not running\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestDoAgentDrainSetError(t *testing.T) {
	dops := newFakeDrainOps()
	dops.err = errors.New("tmux borked")
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doAgentDrain(dops, sp, events.Discard, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got := stderr.String(); got != "gc agent drain: tmux borked\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestDoAgentUndrain(t *testing.T) {
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker"] = true
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingRecorder{}
	var stdout, stderr bytes.Buffer
	code := doAgentUndrain(dops, sp, rec, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if dops.draining["gc-city-worker"] {
		t.Error("drain flag still set after undrain")
	}
	if got := stdout.String(); got != "Undrained agent 'worker'\n" {
		t.Errorf("stdout = %q, want %q", got, "Undrained agent 'worker'\n")
	}
	if len(rec.events) != 1 || rec.events[0].Type != events.AgentUndrained {
		t.Errorf("events = %v, want one AgentUndrained event", rec.events)
	}
}

func TestDoAgentUndrainNotRunning(t *testing.T) {
	dops := newFakeDrainOps()
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doAgentUndrain(dops, sp, events.Discard, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if got := stderr.String(); got != "gc agent undrain: agent \"worker\" is not running\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestDoAgentDrainCheck(t *testing.T) {
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker"] = true

	code := doAgentDrainCheck(dops, "gc-city-worker")
	if code != 0 {
		t.Errorf("code = %d, want 0 (draining)", code)
	}
}

func TestDoAgentDrainCheckNotDraining(t *testing.T) {
	dops := newFakeDrainOps()

	code := doAgentDrainCheck(dops, "gc-city-worker")
	if code != 1 {
		t.Errorf("code = %d, want 1 (not draining)", code)
	}
}

func TestDoAgentDrainCheckError(t *testing.T) {
	dops := newFakeDrainOps()
	dops.err = errors.New("tmux gone")

	code := doAgentDrainCheck(dops, "gc-city-worker")
	if code != 1 {
		t.Errorf("code = %d, want 1 (error → not draining)", code)
	}
}

// recordingRecorder captures events for test assertions.
type recordingRecorder struct {
	events []events.Event
}

func (r *recordingRecorder) Record(e events.Event) {
	r.events = append(r.events, e)
}
