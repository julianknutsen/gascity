package session

import "testing"

// Compile-time check: Fake implements Provider.
var _ Provider = (*Fake)(nil)

func TestFake_StartStop(t *testing.T) {
	f := NewFake()

	if err := f.Start("mayor", Config{WorkDir: "/tmp"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !f.IsRunning("mayor") {
		t.Fatal("expected mayor to be running after Start")
	}

	// Duplicate start should fail.
	if err := f.Start("mayor", Config{}); err == nil {
		t.Fatal("expected error on duplicate Start")
	}

	if err := f.Stop("mayor"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if f.IsRunning("mayor") {
		t.Fatal("expected mayor to not be running after Stop")
	}

	// Idempotent stop.
	if err := f.Stop("mayor"); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestFake_Attach(t *testing.T) {
	f := NewFake()

	// Attach to nonexistent session.
	if err := f.Attach("ghost"); err == nil {
		t.Fatal("expected error attaching to nonexistent session")
	}

	_ = f.Start("mayor", Config{})
	if err := f.Attach("mayor"); err != nil {
		t.Fatalf("Attach: %v", err)
	}
}

func TestFake_SpyRecordsCalls(t *testing.T) {
	f := NewFake()

	_ = f.Start("a", Config{WorkDir: "/w"})
	f.IsRunning("a")
	_ = f.Attach("a")
	_ = f.Stop("a")

	want := []string{"Start", "IsRunning", "Attach", "Stop"}
	if len(f.Calls) != len(want) {
		t.Fatalf("got %d calls, want %d", len(f.Calls), len(want))
	}
	for i, c := range f.Calls {
		if c.Method != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c.Method, want[i])
		}
		if c.Name != "a" {
			t.Errorf("call %d: got name %q, want %q", i, c.Name, "a")
		}
	}

	// Verify config was captured on Start.
	if f.Calls[0].Config.WorkDir != "/w" {
		t.Errorf("Start config WorkDir: got %q, want %q", f.Calls[0].Config.WorkDir, "/w")
	}
}
