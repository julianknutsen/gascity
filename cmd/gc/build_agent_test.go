package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestBuildOneAgentMinimal(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
	}

	a, err := buildOneAgent(p, cfgAgent, "worker", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}
	if a.Name() != "worker" {
		t.Errorf("Name() = %q, want %q", a.Name(), "worker")
	}
	if a.SessionName() != "worker" {
		t.Errorf("SessionName() = %q, want %q", a.SessionName(), "worker")
	}
}

func TestBuildOneAgentStartCommandBypassesProvider(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "custom",
		StartCommand: "/usr/local/bin/my-agent",
		Provider:     "nonexistent-provider",
	}

	a, err := buildOneAgent(p, cfgAgent, "custom", nil)
	if err != nil {
		t.Fatalf("buildOneAgent should succeed with start_command: %v", err)
	}

	cfg := a.SessionConfig()
	if !strings.Contains(cfg.Command, "/usr/local/bin/my-agent") {
		t.Errorf("command should contain start_command, got %q", cfg.Command)
	}
}

func TestBuildOneAgentUnknownProviderError(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	// Override lookPath to fail for the unknown provider.
	p.lookPath = func(name string) (string, error) {
		return "", &lookPathErr{name}
	}

	cfgAgent := &config.Agent{
		Name:     "bad",
		Provider: "missing-agent",
	}

	_, err := buildOneAgent(p, cfgAgent, "bad", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention agent name: %v", err)
	}
}

type lookPathErr struct{ name string }

func (e *lookPathErr) Error() string { return e.name + ": not found" }

func TestBuildOneAgentSetsEnvironment(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "envtest",
		StartCommand: "echo",
	}

	a, err := buildOneAgent(p, cfgAgent, "envtest", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	cfg := a.SessionConfig()
	if cfg.Env["GC_AGENT"] != "envtest" {
		t.Errorf("GC_AGENT = %q, want %q", cfg.Env["GC_AGENT"], "envtest")
	}
	if cfg.Env["GC_CITY"] != p.cityPath {
		t.Errorf("GC_CITY = %q, want %q", cfg.Env["GC_CITY"], p.cityPath)
	}
	// New GC_SESSION_* vars (Phase 1 unified sessions).
	if cfg.Env["GC_TEMPLATE"] != "envtest" {
		t.Errorf("GC_TEMPLATE = %q, want %q", cfg.Env["GC_TEMPLATE"], "envtest")
	}
	if cfg.Env["GC_SESSION_NAME"] == "" {
		t.Error("GC_SESSION_NAME is empty, want non-empty")
	}
}

func TestBuildOneAgentPromptModeNoneSkipsPrompt(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	// Use a built-in provider with prompt_mode overridden to "none"
	// via city-level providers.
	p.providers = map[string]config.ProviderSpec{
		"silent-provider": {
			Command:    "echo",
			PromptMode: "none",
		},
	}
	cfgAgent := &config.Agent{
		Name:     "silent",
		Provider: "silent-provider",
	}

	a, err := buildOneAgent(p, cfgAgent, "silent", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	// With prompt_mode=none from provider, the command should NOT have
	// a quoted prompt appended.
	cfg := a.SessionConfig()
	if strings.Contains(cfg.Command, "silent •") {
		t.Errorf("command should not contain beacon with prompt_mode=none, got %q", cfg.Command)
	}
}

func TestBuildOneAgentFingerprintExtra(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "pooled",
		StartCommand: "echo",
	}
	fpExtra := map[string]string{"pool_min": "2", "pool_max": "5"}

	a, err := buildOneAgent(p, cfgAgent, "pooled", fpExtra)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	cfg := a.SessionConfig()
	if cfg.FingerprintExtra["pool_min"] != "2" {
		t.Errorf("FingerprintExtra[pool_min] = %q, want %q", cfg.FingerprintExtra["pool_min"], "2")
	}
}

func TestBuildOneAgentQualifiedName(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "polecat",
		Dir:          "myrig",
		StartCommand: "echo",
	}

	a, err := buildOneAgent(p, cfgAgent, "myrig/polecat", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}
	if a.Name() != "myrig/polecat" {
		t.Errorf("Name() = %q, want %q", a.Name(), "myrig/polecat")
	}
	if a.SessionName() != "myrig--polecat" {
		t.Errorf("SessionName() = %q, want %q", a.SessionName(), "myrig--polecat")
	}
}

func TestBuildOneAgentStartsAndRuns(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:         "runner",
		StartCommand: "echo hello",
	}

	a, err := buildOneAgent(p, cfgAgent, "runner", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !a.IsRunning() {
		t.Error("agent should be running after Start")
	}
}

func TestBuildOneAgentCustomSessionTemplate(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	p.sessionTemplate = "custom-{{.City}}-{{.Agent}}"
	cfgAgent := &config.Agent{
		Name:         "mayor",
		StartCommand: "echo",
	}

	a, err := buildOneAgent(p, cfgAgent, "mayor", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}
	if a.SessionName() != "custom-city-mayor" {
		t.Errorf("SessionName() = %q, want %q", a.SessionName(), "custom-city-mayor")
	}
}

func TestBuildOneAgentClaudeProviderCommand(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	cfgAgent := &config.Agent{
		Name:     "polecat",
		Provider: "claude",
	}

	a, err := buildOneAgent(p, cfgAgent, "wasteland/polecat", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	cfg := a.SessionConfig()
	// The command should contain the sh -c wrapper with bd preamble.
	if !strings.Contains(cfg.Command, "sh -c") {
		t.Errorf("command should contain sh -c wrapper, got %q", cfg.Command)
	}
	if !strings.Contains(cfg.Command, "bd list") {
		t.Errorf("command should contain bd list preamble, got %q", cfg.Command)
	}
	if !strings.Contains(cfg.Command, "${GC_CLI:-claude} --dangerously-skip-permissions") {
		t.Errorf("command should contain ${GC_CLI:-claude} --dangerously-skip-permissions, got %q", cfg.Command)
	}
	// GC_AGENT should be set for the preamble to use.
	if cfg.Env["GC_AGENT"] != "wasteland/polecat" {
		t.Errorf("GC_AGENT = %q, want %q", cfg.Env["GC_AGENT"], "wasteland/polecat")
	}
}

func TestNewAgentBuildParams(t *testing.T) {
	sp := runtime.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{
			Name:            "my-city",
			SessionTemplate: "gc-{{.City}}-{{.Agent}}",
		},
		Rigs: []config.Rig{
			{Name: "rig1", Path: "/tmp/rig1"},
		},
	}
	var stderr bytes.Buffer

	bp := newAgentBuildParams("my-city", "/tmp/city", cfg, sp, time.Time{}, &stderr)

	if bp.cityName != "my-city" {
		t.Errorf("cityName = %q, want %q", bp.cityName, "my-city")
	}
	if bp.cityPath != "/tmp/city" {
		t.Errorf("cityPath = %q, want %q", bp.cityPath, "/tmp/city")
	}
	if len(bp.rigs) != 1 {
		t.Errorf("rigs = %d, want 1", len(bp.rigs))
	}
	if bp.sessionTemplate != "gc-{{.City}}-{{.Agent}}" {
		t.Errorf("sessionTemplate = %q, want config value", bp.sessionTemplate)
	}
}

func TestEffectiveOverlayDirs(t *testing.T) {
	tests := []struct {
		name    string
		city    []string
		rig     map[string][]string
		rigName string
		want    []string
	}{
		{
			name: "city only",
			city: []string{"/a/overlay", "/b/overlay"},
			want: []string{"/a/overlay", "/b/overlay"},
		},
		{
			name:    "rig only",
			rig:     map[string][]string{"myrig": {"/r/overlay"}},
			rigName: "myrig",
			want:    []string{"/r/overlay"},
		},
		{
			name:    "city plus rig",
			city:    []string{"/a/overlay"},
			rig:     map[string][]string{"myrig": {"/r/overlay"}},
			rigName: "myrig",
			want:    []string{"/a/overlay", "/r/overlay"},
		},
		{
			name:    "rig not found",
			city:    []string{"/a/overlay"},
			rig:     map[string][]string{"other": {"/r/overlay"}},
			rigName: "myrig",
			want:    []string{"/a/overlay"},
		},
		{
			name: "both empty",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveOverlayDirs(tc.city, tc.rig, tc.rigName)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestBuildOneAgentPackOverlayDirs(t *testing.T) {
	sp := runtime.NewFake()
	p := testBuildParams(sp)
	p.packOverlayDirs = []string{"/pack1/overlay", "/pack2/overlay"}

	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo",
	}

	a, err := buildOneAgent(p, cfgAgent, "worker", nil)
	if err != nil {
		t.Fatalf("buildOneAgent: %v", err)
	}

	cfg := a.SessionConfig()
	if len(cfg.PackOverlayDirs) != 2 {
		t.Fatalf("PackOverlayDirs = %v, want 2 entries", cfg.PackOverlayDirs)
	}
	if cfg.PackOverlayDirs[0] != "/pack1/overlay" {
		t.Errorf("PackOverlayDirs[0] = %q, want %q", cfg.PackOverlayDirs[0], "/pack1/overlay")
	}
}

func TestNewAgentBuildParamsPackOverlayDirs(t *testing.T) {
	sp := runtime.NewFake()
	cfg := &config.City{
		Workspace:       config.Workspace{Name: "test"},
		PackOverlayDirs: []string{"/x/overlay"},
		RigOverlayDirs:  map[string][]string{"r": {"/y/overlay"}},
	}
	var stderr bytes.Buffer

	bp := newAgentBuildParams("test", "/tmp/city", cfg, sp, time.Time{}, &stderr)

	if len(bp.packOverlayDirs) != 1 || bp.packOverlayDirs[0] != "/x/overlay" {
		t.Errorf("packOverlayDirs = %v, want [/x/overlay]", bp.packOverlayDirs)
	}
	if len(bp.rigOverlayDirs["r"]) != 1 || bp.rigOverlayDirs["r"][0] != "/y/overlay" {
		t.Errorf("rigOverlayDirs = %v, want map[r:[/y/overlay]]", bp.rigOverlayDirs)
	}
}
