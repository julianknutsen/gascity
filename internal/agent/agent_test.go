package agent

import (
	"testing"

	"github.com/steveyegge/gascity/internal/session"
)

func TestManagedName(t *testing.T) {
	sp := session.NewFake()
	a := New("mayor", "gc-city-mayor", "claude", sp)
	if got := a.Name(); got != "mayor" {
		t.Errorf("Name() = %q, want %q", got, "mayor")
	}
}

func TestManagedSessionName(t *testing.T) {
	sp := session.NewFake()
	a := New("mayor", "gc-city-mayor", "claude", sp)
	if got := a.SessionName(); got != "gc-city-mayor" {
		t.Errorf("SessionName() = %q, want %q", got, "gc-city-mayor")
	}
}

func TestManagedStart(t *testing.T) {
	sp := session.NewFake()
	a := New("mayor", "gc-city-mayor", "claude --skip", sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	// Verify delegation: sp.Start was called with session name + command.
	if len(sp.Calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(sp.Calls), sp.Calls)
	}
	c := sp.Calls[0]
	if c.Method != "Start" {
		t.Errorf("Method = %q, want %q", c.Method, "Start")
	}
	if c.Name != "gc-city-mayor" {
		t.Errorf("Name = %q, want %q", c.Name, "gc-city-mayor")
	}
	if c.Config.Command != "claude --skip" {
		t.Errorf("Config.Command = %q, want %q", c.Config.Command, "claude --skip")
	}
}

func TestManagedStop(t *testing.T) {
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil

	a := New("mayor", "gc-city-mayor", "", sp)
	if err := a.Stop(); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}

	if len(sp.Calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(sp.Calls), sp.Calls)
	}
	if sp.Calls[0].Method != "Stop" {
		t.Errorf("Method = %q, want %q", sp.Calls[0].Method, "Stop")
	}
	if sp.Calls[0].Name != "gc-city-mayor" {
		t.Errorf("Name = %q, want %q", sp.Calls[0].Name, "gc-city-mayor")
	}
}

func TestManagedIsRunning(t *testing.T) {
	sp := session.NewFake()
	a := New("mayor", "gc-city-mayor", "", sp)

	if a.IsRunning() {
		t.Error("IsRunning() = true before Start, want false")
	}

	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil

	if !a.IsRunning() {
		t.Error("IsRunning() = false after Start, want true")
	}

	if len(sp.Calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(sp.Calls), sp.Calls)
	}
	if sp.Calls[0].Method != "IsRunning" {
		t.Errorf("Method = %q, want %q", sp.Calls[0].Method, "IsRunning")
	}
}

func TestManagedAttach(t *testing.T) {
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil

	a := New("mayor", "gc-city-mayor", "", sp)
	if err := a.Attach(); err != nil {
		t.Fatalf("Attach() = %v, want nil", err)
	}

	if len(sp.Calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(sp.Calls), sp.Calls)
	}
	if sp.Calls[0].Method != "Attach" {
		t.Errorf("Method = %q, want %q", sp.Calls[0].Method, "Attach")
	}
	if sp.Calls[0].Name != "gc-city-mayor" {
		t.Errorf("Name = %q, want %q", sp.Calls[0].Name, "gc-city-mayor")
	}
}
