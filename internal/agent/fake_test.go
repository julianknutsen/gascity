package agent

import (
	"fmt"
	"testing"
)

func TestFakeStart(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	if err := f.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if !f.Running {
		t.Error("Running = false after Start, want true")
	}
	if len(f.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(f.Calls))
	}
	if f.Calls[0].Method != "Start" {
		t.Errorf("Method = %q, want %q", f.Calls[0].Method, "Start")
	}
	if f.Calls[0].Name != "mayor" {
		t.Errorf("Name = %q, want %q", f.Calls[0].Name, "mayor")
	}
}

func TestFakeStartError(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	f.StartErr = fmt.Errorf("boom")

	err := f.Start()
	if err == nil {
		t.Fatal("Start() = nil, want error")
	}
	if err.Error() != "boom" {
		t.Errorf("Start() = %q, want %q", err, "boom")
	}
	if f.Running {
		t.Error("Running = true after failed Start, want false")
	}
}

func TestFakeStop(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	f.Running = true

	if err := f.Stop(); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
	if f.Running {
		t.Error("Running = true after Stop, want false")
	}
	if len(f.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(f.Calls))
	}
	if f.Calls[0].Method != "Stop" {
		t.Errorf("Method = %q, want %q", f.Calls[0].Method, "Stop")
	}
}

func TestFakeStopError(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.StopErr = fmt.Errorf("stop boom")

	err := f.Stop()
	if err == nil {
		t.Fatal("Stop() = nil, want error")
	}
	if err.Error() != "stop boom" {
		t.Errorf("Stop() = %q, want %q", err, "stop boom")
	}
	// Running stays true on error — stop didn't succeed.
	if !f.Running {
		t.Error("Running = false after failed Stop, want true")
	}
}

func TestFakeIsRunning(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")

	if f.IsRunning() {
		t.Error("IsRunning() = true, want false")
	}

	f.Running = true
	if !f.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Both calls recorded.
	if len(f.Calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(f.Calls))
	}
	for _, c := range f.Calls {
		if c.Method != "IsRunning" {
			t.Errorf("Method = %q, want %q", c.Method, "IsRunning")
		}
	}
}

func TestFakeAttach(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")

	if err := f.Attach(); err != nil {
		t.Fatalf("Attach() = %v, want nil", err)
	}
	if len(f.Calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(f.Calls))
	}
	if f.Calls[0].Method != "Attach" {
		t.Errorf("Method = %q, want %q", f.Calls[0].Method, "Attach")
	}
}

func TestFakeAttachError(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	f.AttachErr = fmt.Errorf("attach boom")

	err := f.Attach()
	if err == nil {
		t.Fatal("Attach() = nil, want error")
	}
	if err.Error() != "attach boom" {
		t.Errorf("Attach() = %q, want %q", err, "attach boom")
	}
}

func TestFakeName(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	if got := f.Name(); got != "mayor" {
		t.Errorf("Name() = %q, want %q", got, "mayor")
	}
}

func TestFakeSessionName(t *testing.T) {
	f := NewFake("mayor", "gc-city-mayor")
	if got := f.SessionName(); got != "gc-city-mayor" {
		t.Errorf("SessionName() = %q, want %q", got, "gc-city-mayor")
	}
}
