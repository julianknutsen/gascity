package agent

import (
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

func TestManagedName(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, StartupHints{}, sp)
	if got := a.Name(); got != "mayor" {
		t.Errorf("Name() = %q, want %q", got, "mayor")
	}
}

func TestManagedSessionName(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, StartupHints{}, sp)
	if got := a.SessionName(); got != "gc-city-mayor" {
		t.Errorf("SessionName() = %q, want %q", got, "gc-city-mayor")
	}
}

func TestManagedStart(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude --skip", "", nil, StartupHints{}, sp)

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

func TestManagedStartWithPrompt(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude --skip", "You are a mayor", nil, StartupHints{}, sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	c := sp.Calls[0]
	want := "claude --skip 'You are a mayor'"
	if c.Config.Command != want {
		t.Errorf("Config.Command = %q, want %q", c.Config.Command, want)
	}
}

func TestManagedStartWithEnv(t *testing.T) {
	sp := session.NewFake()
	env := map[string]string{"GC_AGENT": "mayor"}
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", env, StartupHints{}, sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	c := sp.Calls[0]
	if c.Config.Env["GC_AGENT"] != "mayor" {
		t.Errorf("Config.Env[GC_AGENT] = %q, want %q", c.Config.Env["GC_AGENT"], "mayor")
	}
}

func TestManagedStartWithHints(t *testing.T) {
	sp := session.NewFake()
	hints := StartupHints{
		ReadyPromptPrefix:      "> ",
		ReadyDelayMs:           5000,
		ProcessNames:           []string{"claude", "node"},
		EmitsPermissionWarning: true,
	}
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, hints, sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	c := sp.Calls[0]
	if c.Config.ReadyPromptPrefix != "> " {
		t.Errorf("Config.ReadyPromptPrefix = %q, want %q", c.Config.ReadyPromptPrefix, "> ")
	}
	if c.Config.ReadyDelayMs != 5000 {
		t.Errorf("Config.ReadyDelayMs = %d, want %d", c.Config.ReadyDelayMs, 5000)
	}
	if len(c.Config.ProcessNames) != 2 || c.Config.ProcessNames[0] != "claude" {
		t.Errorf("Config.ProcessNames = %v, want [claude node]", c.Config.ProcessNames)
	}
	if !c.Config.EmitsPermissionWarning {
		t.Error("Config.EmitsPermissionWarning = false, want true")
	}
}

func TestManagedStartWithZeroHints(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, StartupHints{}, sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	c := sp.Calls[0]
	if c.Config.ReadyPromptPrefix != "" {
		t.Errorf("Config.ReadyPromptPrefix = %q, want empty", c.Config.ReadyPromptPrefix)
	}
	if c.Config.ReadyDelayMs != 0 {
		t.Errorf("Config.ReadyDelayMs = %d, want 0", c.Config.ReadyDelayMs)
	}
	if len(c.Config.ProcessNames) != 0 {
		t.Errorf("Config.ProcessNames = %v, want nil", c.Config.ProcessNames)
	}
	if c.Config.EmitsPermissionWarning {
		t.Error("Config.EmitsPermissionWarning = true, want false")
	}
}

func TestManagedStartAllParamsCombined(t *testing.T) {
	sp := session.NewFake()
	env := map[string]string{"GC_AGENT": "mayor"}
	hints := StartupHints{
		ReadyPromptPrefix:      "❯ ",
		ReadyDelayMs:           10000,
		ProcessNames:           []string{"claude", "node"},
		EmitsPermissionWarning: true,
	}
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude --skip", "You are mayor", env, hints, sp)

	if err := a.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	c := sp.Calls[0]
	// Command includes shell-quoted prompt.
	want := "claude --skip 'You are mayor'"
	if c.Config.Command != want {
		t.Errorf("Config.Command = %q, want %q", c.Config.Command, want)
	}
	if c.Config.Env["GC_AGENT"] != "mayor" {
		t.Errorf("Config.Env[GC_AGENT] = %q, want %q", c.Config.Env["GC_AGENT"], "mayor")
	}
	if c.Config.ReadyPromptPrefix != "❯ " {
		t.Errorf("Config.ReadyPromptPrefix = %q, want %q", c.Config.ReadyPromptPrefix, "❯ ")
	}
	if c.Config.ReadyDelayMs != 10000 {
		t.Errorf("Config.ReadyDelayMs = %d, want %d", c.Config.ReadyDelayMs, 10000)
	}
	if len(c.Config.ProcessNames) != 2 || c.Config.ProcessNames[0] != "claude" {
		t.Errorf("Config.ProcessNames = %v, want [claude node]", c.Config.ProcessNames)
	}
	if !c.Config.EmitsPermissionWarning {
		t.Error("Config.EmitsPermissionWarning = false, want true")
	}
}

func TestManagedStartError(t *testing.T) {
	sp := session.NewFailFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, StartupHints{}, sp)

	err := a.Start()
	if err == nil {
		t.Fatal("Start() = nil, want error from broken provider")
	}
}

func TestManagedStopError(t *testing.T) {
	sp := session.NewFailFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "", "", nil, StartupHints{}, sp)

	err := a.Stop()
	if err == nil {
		t.Fatal("Stop() = nil, want error from broken provider")
	}
}

func TestManagedAttachError(t *testing.T) {
	sp := session.NewFailFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "", "", nil, StartupHints{}, sp)

	err := a.Attach()
	if err == nil {
		t.Fatal("Attach() = nil, want error from broken provider")
	}
}

func TestManagedSessionConfig(t *testing.T) {
	sp := session.NewFake()
	env := map[string]string{"GC_AGENT": "mayor"}
	hints := StartupHints{
		ReadyPromptPrefix:      "> ",
		ReadyDelayMs:           5000,
		ProcessNames:           []string{"claude"},
		EmitsPermissionWarning: true,
	}
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude --skip", "You are mayor", env, hints, sp)

	cfg := a.SessionConfig()

	// Command includes shell-quoted prompt.
	wantCmd := "claude --skip 'You are mayor'"
	if cfg.Command != wantCmd {
		t.Errorf("Command = %q, want %q", cfg.Command, wantCmd)
	}
	if cfg.Env["GC_AGENT"] != "mayor" {
		t.Errorf("Env[GC_AGENT] = %q, want %q", cfg.Env["GC_AGENT"], "mayor")
	}
	if cfg.ReadyPromptPrefix != "> " {
		t.Errorf("ReadyPromptPrefix = %q, want %q", cfg.ReadyPromptPrefix, "> ")
	}
	if cfg.ReadyDelayMs != 5000 {
		t.Errorf("ReadyDelayMs = %d, want %d", cfg.ReadyDelayMs, 5000)
	}
	if len(cfg.ProcessNames) != 1 || cfg.ProcessNames[0] != "claude" {
		t.Errorf("ProcessNames = %v, want [claude]", cfg.ProcessNames)
	}
	if !cfg.EmitsPermissionWarning {
		t.Error("EmitsPermissionWarning = false, want true")
	}

	// SessionConfig should not call the provider.
	if len(sp.Calls) != 0 {
		t.Errorf("provider received %d calls, want 0", len(sp.Calls))
	}
}

func TestManagedSessionConfigNoPrompt(t *testing.T) {
	sp := session.NewFake()
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "claude", "", nil, StartupHints{}, sp)

	cfg := a.SessionConfig()
	if cfg.Command != "claude" {
		t.Errorf("Command = %q, want %q", cfg.Command, "claude")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "'hello'"},
		{"it's here", `'it'\''s here'`},
		{"", "''"},
		{"line1\nline2", "'line1\nline2'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.in)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestManagedStop(t *testing.T) {
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil

	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "", "", nil, StartupHints{}, sp)
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
	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "", "", nil, StartupHints{}, sp)

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

	a := New(config.Agent{Name: "mayor"}, "gc-city-mayor", "", "", nil, StartupHints{}, sp)
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
