package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

// fakeDrainOps is a test double for drainOps.
type fakeDrainOps struct {
	draining        map[string]bool
	err             error // injected error for all ops
	setDrainCalls   []string
	clearDrainCalls []string
}

func newFakeDrainOps() *fakeDrainOps {
	return &fakeDrainOps{draining: make(map[string]bool)}
}

func (f *fakeDrainOps) setDrain(sessionName string) error {
	f.setDrainCalls = append(f.setDrainCalls, sessionName)
	if f.err != nil {
		return f.err
	}
	f.draining[sessionName] = true
	return nil
}

func (f *fakeDrainOps) clearDrain(sessionName string) error {
	f.clearDrainCalls = append(f.clearDrainCalls, sessionName)
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

// ---------------------------------------------------------------------------
// doAgentDrain tests
// ---------------------------------------------------------------------------

func TestDoAgentDrain(t *testing.T) {
	dops := newFakeDrainOps()
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := events.NewFake()
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
	if len(rec.Events) != 1 || rec.Events[0].Type != events.AgentDraining {
		t.Errorf("events = %v, want one AgentDraining event", rec.Events)
	}
	if rec.Events[0].Subject != "worker" {
		t.Errorf("event subject = %q, want %q", rec.Events[0].Subject, "worker")
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

// ---------------------------------------------------------------------------
// doAgentUndrain tests
// ---------------------------------------------------------------------------

func TestDoAgentUndrain(t *testing.T) {
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker"] = true
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}

	rec := events.NewFake()
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
	if len(rec.Events) != 1 || rec.Events[0].Type != events.AgentUndrained {
		t.Errorf("events = %v, want one AgentUndrained event", rec.Events)
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

// ---------------------------------------------------------------------------
// doAgentDrainCheck tests
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// newDrainOps factory tests
// ---------------------------------------------------------------------------

func TestNewDrainOpsTmuxProvider(t *testing.T) {
	tp := sessiontmux.NewProvider()
	dops := newDrainOps(tp)
	if dops == nil {
		t.Fatal("newDrainOps(tmux.Provider) = nil, want non-nil")
	}
	if _, ok := dops.(*tmuxDrainOps); !ok {
		t.Errorf("newDrainOps returned %T, want *tmuxDrainOps", dops)
	}
}

func TestNewDrainOpsFakeProvider(t *testing.T) {
	fp := session.NewFake()
	dops := newDrainOps(fp)
	if dops != nil {
		t.Errorf("newDrainOps(Fake) = %v, want nil", dops)
	}
}

// ---------------------------------------------------------------------------
// findAgentInConfig unit tests
// ---------------------------------------------------------------------------

func TestFindAgentInConfig(t *testing.T) {
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker", Pool: &config.PoolConfig{Min: 0, Max: 5, Check: "echo 3"}},
			{Name: "singleton", Pool: &config.PoolConfig{Min: 0, Max: 1, Check: "echo 1"}},
		},
	}

	tests := []struct {
		name      string
		lookup    string
		wantFound bool
		wantName  string
		wantPool  bool // whether returned agent should have Pool set
	}{
		// Exact match on non-pool agent.
		{"exact match", "mayor", true, "mayor", false},

		// Exact match on pool agent (base name).
		{"pool base name", "worker", true, "worker", true},

		// Pool instance matches.
		{"pool instance worker-1", "worker-1", true, "worker-1", false},
		{"pool instance worker-5", "worker-5", true, "worker-5", false},

		// Pool instance out of range (too high).
		{"pool instance worker-6", "worker-6", false, "", false},

		// Pool instance out of range (zero).
		{"pool instance worker-0", "worker-0", false, "", false},

		// Pool instance non-numeric suffix.
		{"pool instance worker-abc", "worker-abc", false, "", false},

		// Pool instance negative (parsed as non-numeric due to dash).
		{"pool instance worker--1", "worker--1", false, "", false},

		// Max=1 pool: the guard requires Max > 1, so {name}-1 does NOT match.
		{"singleton-1 no match", "singleton-1", false, "", false},

		// Nonexistent agent.
		{"nonexistent", "nobody", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := findAgentInConfig(cfg, tt.lookup)
			if found != tt.wantFound {
				t.Fatalf("findAgentInConfig(%q) found = %v, want %v", tt.lookup, found, tt.wantFound)
			}
			if !found {
				return
			}
			if got.Name != tt.wantName {
				t.Errorf("agent.Name = %q, want %q", got.Name, tt.wantName)
			}
			if (got.Pool != nil) != tt.wantPool {
				t.Errorf("agent.Pool != nil = %v, want %v", got.Pool != nil, tt.wantPool)
			}
		})
	}
}
