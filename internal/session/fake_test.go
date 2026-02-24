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

func TestFailFake_AllOpsFail(t *testing.T) {
	f := NewFailFake()

	if err := f.Start("mayor", Config{WorkDir: "/tmp"}); err == nil {
		t.Fatal("expected Start to fail on broken fake")
	}
	if f.IsRunning("mayor") {
		t.Fatal("expected IsRunning to return false on broken fake")
	}
	if err := f.Attach("mayor"); err == nil {
		t.Fatal("expected Attach to fail on broken fake")
	}
	if err := f.Stop("mayor"); err == nil {
		t.Fatal("expected Stop to fail on broken fake")
	}
}

func TestFailFake_RecordsCalls(t *testing.T) {
	f := NewFailFake()

	_ = f.Start("a", Config{})
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

func TestFake_CapturesAllConfigFields(t *testing.T) {
	f := NewFake()

	cfg := Config{
		WorkDir:                "/proj",
		Command:                "claude --dangerously-skip-permissions",
		Env:                    map[string]string{"GC_AGENT": "mayor", "HOME": "/home/user"},
		ReadyPromptPrefix:      "❯ ",
		ReadyDelayMs:           10000,
		ProcessNames:           []string{"claude", "node"},
		EmitsPermissionWarning: true,
	}
	if err := f.Start("mayor", cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := f.Calls[0].Config
	if got.WorkDir != "/proj" {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, "/proj")
	}
	if got.Command != "claude --dangerously-skip-permissions" {
		t.Errorf("Command = %q, want %q", got.Command, "claude --dangerously-skip-permissions")
	}
	if got.Env["GC_AGENT"] != "mayor" {
		t.Errorf("Env[GC_AGENT] = %q, want %q", got.Env["GC_AGENT"], "mayor")
	}
	if got.Env["HOME"] != "/home/user" {
		t.Errorf("Env[HOME] = %q, want %q", got.Env["HOME"], "/home/user")
	}
	if got.ReadyPromptPrefix != "❯ " {
		t.Errorf("ReadyPromptPrefix = %q, want %q", got.ReadyPromptPrefix, "❯ ")
	}
	if got.ReadyDelayMs != 10000 {
		t.Errorf("ReadyDelayMs = %d, want %d", got.ReadyDelayMs, 10000)
	}
	if len(got.ProcessNames) != 2 || got.ProcessNames[0] != "claude" || got.ProcessNames[1] != "node" {
		t.Errorf("ProcessNames = %v, want [claude node]", got.ProcessNames)
	}
	if !got.EmitsPermissionWarning {
		t.Error("EmitsPermissionWarning = false, want true")
	}
}

func TestFakeProcessAliveDefault(t *testing.T) {
	f := NewFake()
	_ = f.Start("mayor", Config{})

	if !f.ProcessAlive("mayor", []string{"claude"}) {
		t.Error("ProcessAlive = false for healthy session, want true")
	}
}

func TestFakeProcessAliveZombie(t *testing.T) {
	f := NewFake()
	_ = f.Start("mayor", Config{})
	f.Zombies["mayor"] = true

	if f.ProcessAlive("mayor", []string{"claude"}) {
		t.Error("ProcessAlive = true for zombie, want false")
	}
}

func TestFakeProcessAliveEmptyNames(t *testing.T) {
	f := NewFake()
	_ = f.Start("mayor", Config{})
	f.Zombies["mayor"] = true // zombie, but no names to check

	if !f.ProcessAlive("mayor", nil) {
		t.Error("ProcessAlive = false with empty names, want true")
	}
}

func TestFakeProcessAliveBroken(t *testing.T) {
	f := NewFailFake()

	if f.ProcessAlive("mayor", []string{"claude"}) {
		t.Error("ProcessAlive = true on broken fake, want false")
	}
}

func TestFakeNudge(t *testing.T) {
	f := NewFake()
	_ = f.Start("mayor", Config{})

	if err := f.Nudge("mayor", "wake up"); err != nil {
		t.Fatalf("Nudge: %v", err)
	}

	// Find the Nudge call.
	var found bool
	for _, c := range f.Calls {
		if c.Method == "Nudge" {
			found = true
			if c.Name != "mayor" {
				t.Errorf("Nudge Name = %q, want %q", c.Name, "mayor")
			}
			if c.Message != "wake up" {
				t.Errorf("Nudge Message = %q, want %q", c.Message, "wake up")
			}
		}
	}
	if !found {
		t.Error("Nudge call not recorded")
	}
}

func TestFakeNudgeBroken(t *testing.T) {
	f := NewFailFake()

	err := f.Nudge("mayor", "wake up")
	if err == nil {
		t.Fatal("expected Nudge to fail on broken fake")
	}

	// Call should still be recorded.
	var found bool
	for _, c := range f.Calls {
		if c.Method == "Nudge" {
			found = true
		}
	}
	if !found {
		t.Error("Nudge call not recorded on broken fake")
	}
}
