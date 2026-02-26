package config

import (
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func ptrStr(s string) *string { return &s }
func ptrBool(b bool) *bool    { return &b }
func ptrInt(n int) *int       { return &n }

func TestApplyPatches_AgentSuspend(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "mayor"},
			{Name: "polecat", Dir: "hw"},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{Dir: "hw", Name: "polecat", Suspended: ptrBool(true)},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if !cfg.Agents[1].Suspended {
		t.Error("polecat should be suspended")
	}
	if cfg.Agents[0].Suspended {
		t.Error("mayor should not be suspended")
	}
}

func TestApplyPatches_AgentPool(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "polecat", Dir: "hw", Pool: &PoolConfig{Min: 0, Max: 3, Check: "echo 1"}},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{Dir: "hw", Name: "polecat", Pool: &PoolOverride{Max: ptrInt(10)}},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if cfg.Agents[0].Pool.Max != 10 {
		t.Errorf("Pool.Max = %d, want 10", cfg.Agents[0].Pool.Max)
	}
	// Unchanged fields preserved.
	if cfg.Agents[0].Pool.Min != 0 {
		t.Errorf("Pool.Min = %d, want 0", cfg.Agents[0].Pool.Min)
	}
	if cfg.Agents[0].Pool.Check != "echo 1" {
		t.Errorf("Pool.Check = %q, want %q", cfg.Agents[0].Pool.Check, "echo 1")
	}
}

func TestApplyPatches_AgentPoolCreate(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "worker", Dir: "hw"},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{Dir: "hw", Name: "worker", Pool: &PoolOverride{Min: ptrInt(1), Max: ptrInt(5)}},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if cfg.Agents[0].Pool == nil {
		t.Fatal("Pool should be created")
	}
	if cfg.Agents[0].Pool.Min != 1 || cfg.Agents[0].Pool.Max != 5 {
		t.Errorf("Pool = {Min:%d, Max:%d}, want {1, 5}",
			cfg.Agents[0].Pool.Min, cfg.Agents[0].Pool.Max)
	}
}

func TestApplyPatches_AgentEnv(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "worker", Env: map[string]string{"A": "1", "B": "2"}},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{
				Name: "worker",
				Env:  map[string]string{"B": "override", "C": "3"},
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	env := cfg.Agents[0].Env
	if env["A"] != "1" {
		t.Errorf("A = %q, want %q", env["A"], "1")
	}
	if env["B"] != "override" {
		t.Errorf("B = %q, want %q", env["B"], "override")
	}
	if env["C"] != "3" {
		t.Errorf("C = %q, want %q", env["C"], "3")
	}
}

func TestApplyPatches_AgentEnvRemove(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "worker", Env: map[string]string{"A": "1", "B": "2", "C": "3"}},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{Name: "worker", EnvRemove: []string{"B", "C"}},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	env := cfg.Agents[0].Env
	if _, ok := env["B"]; ok {
		t.Error("B should be removed")
	}
	if _, ok := env["C"]; ok {
		t.Error("C should be removed")
	}
	if env["A"] != "1" {
		t.Errorf("A = %q, want %q", env["A"], "1")
	}
}

func TestApplyPatches_AgentScalars(t *testing.T) {
	cfg := &City{
		Agents: []Agent{
			{Name: "worker", PromptTemplate: "old.md", Provider: "claude"},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{
				Name:           "worker",
				PromptTemplate: ptrStr("new.md"),
				Provider:       ptrStr("gemini"),
				Isolation:      ptrStr("worktree"),
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	a := cfg.Agents[0]
	if a.PromptTemplate != "new.md" {
		t.Errorf("PromptTemplate = %q, want %q", a.PromptTemplate, "new.md")
	}
	if a.Provider != "gemini" {
		t.Errorf("Provider = %q, want %q", a.Provider, "gemini")
	}
	if a.Isolation != "worktree" {
		t.Errorf("Isolation = %q, want %q", a.Isolation, "worktree")
	}
}

func TestApplyPatches_AgentNotFound(t *testing.T) {
	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
	}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{
			{Dir: "hw", Name: "polecat", Suspended: ptrBool(true)},
		},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "hw/polecat") {
		t.Errorf("error = %q, want mention of hw/polecat", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want mention of 'not found'", err)
	}
}

func TestApplyPatches_AgentNameRequired(t *testing.T) {
	cfg := &City{}
	err := ApplyPatches(cfg, Patches{
		Agents: []AgentPatch{{Suspended: ptrBool(true)}},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want 'name is required'", err)
	}
}

func TestApplyPatches_RigPath(t *testing.T) {
	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/old/path"},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Rigs: []RigPatch{
			{Name: "hw", Path: ptrStr("/new/path")},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if cfg.Rigs[0].Path != "/new/path" {
		t.Errorf("Path = %q, want %q", cfg.Rigs[0].Path, "/new/path")
	}
}

func TestApplyPatches_RigSuspend(t *testing.T) {
	cfg := &City{
		Rigs: []Rig{{Name: "hw", Path: "/path"}},
	}
	err := ApplyPatches(cfg, Patches{
		Rigs: []RigPatch{
			{Name: "hw", Suspended: ptrBool(true)},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	if !cfg.Rigs[0].Suspended {
		t.Error("rig should be suspended")
	}
}

func TestApplyPatches_RigNotFound(t *testing.T) {
	cfg := &City{
		Rigs: []Rig{{Name: "hw", Path: "/path"}},
	}
	err := ApplyPatches(cfg, Patches{
		Rigs: []RigPatch{
			{Name: "missing", Path: ptrStr("/new")},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want mention of 'missing'", err)
	}
}

func TestApplyPatches_ProviderDeepMerge(t *testing.T) {
	cfg := &City{
		Providers: map[string]ProviderSpec{
			"custom": {
				Command:    "agent",
				PromptMode: "arg",
				Env:        map[string]string{"KEY": "val"},
			},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Providers: []ProviderPatch{
			{
				Name:      "custom",
				Command:   ptrStr("new-agent"),
				Env:       map[string]string{"KEY2": "val2"},
				EnvRemove: []string{"KEY"},
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	p := cfg.Providers["custom"]
	if p.Command != "new-agent" {
		t.Errorf("Command = %q, want %q", p.Command, "new-agent")
	}
	if p.PromptMode != "arg" {
		t.Errorf("PromptMode = %q, want %q (unchanged)", p.PromptMode, "arg")
	}
	if p.Env["KEY2"] != "val2" {
		t.Errorf("KEY2 = %q, want %q", p.Env["KEY2"], "val2")
	}
	if _, ok := p.Env["KEY"]; ok {
		t.Error("KEY should be removed")
	}
}

func TestApplyPatches_ProviderReplace(t *testing.T) {
	cfg := &City{
		Providers: map[string]ProviderSpec{
			"custom": {
				Command:    "old-agent",
				PromptMode: "arg",
				Env:        map[string]string{"SECRET": "hidden"},
			},
		},
	}
	err := ApplyPatches(cfg, Patches{
		Providers: []ProviderPatch{
			{
				Name:    "custom",
				Replace: true,
				Command: ptrStr("new-agent"),
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	p := cfg.Providers["custom"]
	if p.Command != "new-agent" {
		t.Errorf("Command = %q, want %q", p.Command, "new-agent")
	}
	// Replace clears fields not in patch.
	if p.PromptMode != "" {
		t.Errorf("PromptMode = %q, want empty (replaced)", p.PromptMode)
	}
	if len(p.Env) != 0 {
		t.Errorf("Env = %v, want empty (replaced)", p.Env)
	}
}

func TestApplyPatches_ProviderNotFound(t *testing.T) {
	cfg := &City{
		Providers: map[string]ProviderSpec{},
	}
	err := ApplyPatches(cfg, Patches{
		Providers: []ProviderPatch{
			{Name: "missing", Command: ptrStr("cmd")},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want mention of 'missing'", err)
	}
}

func TestApplyPatches_Empty(t *testing.T) {
	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
	}
	err := ApplyPatches(cfg, Patches{})
	if err != nil {
		t.Fatalf("ApplyPatches on empty: %v", err)
	}
}

func TestLoadWithIncludes_PatchesFromFragment(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["prod.toml"]

[workspace]
name = "test"

[[agents]]
name = "polecat"
dir = "hw"

[[agents]]
name = "mayor"
`)
	fs.Files["/city/prod.toml"] = []byte(`
[[patches.agents]]
dir = "hw"
name = "polecat"
suspended = true
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// Patches should be applied: polecat suspended.
	for _, a := range cfg.Agents {
		if a.Dir == "hw" && a.Name == "polecat" {
			if !a.Suspended {
				t.Error("polecat should be suspended after patch")
			}
			return
		}
	}
	t.Error("polecat not found in agents")
}

func TestLoadWithIncludes_PatchesFromRoot(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["agents.toml"]

[workspace]
name = "test"

[[patches.agents]]
name = "worker"
suspended = true
`)
	fs.Files["/city/agents.toml"] = []byte(`
[[agents]]
name = "worker"
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if !cfg.Agents[0].Suspended {
		t.Error("worker should be suspended after root patch")
	}
	// Patches should be cleared after application.
	if !cfg.Patches.IsEmpty() {
		t.Error("Patches should be cleared after application")
	}
}

func TestLoadWithIncludes_PatchTargetMissing(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
[workspace]
name = "test"

[[agents]]
name = "mayor"

[[patches.agents]]
dir = "hw"
name = "ghost"
suspended = true
`)
	_, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err == nil {
		t.Fatal("expected error for patch targeting nonexistent agent")
	}
	if !strings.Contains(err.Error(), "hw/ghost") {
		t.Errorf("error = %q, want mention of hw/ghost", err)
	}
}

func TestPatchesIsEmpty(t *testing.T) {
	if !(&Patches{}).IsEmpty() {
		t.Error("empty Patches should be empty")
	}
	if (&Patches{Agents: []AgentPatch{{Name: "x"}}}).IsEmpty() {
		t.Error("Patches with agents should not be empty")
	}
}

func TestApplyPatches_AgentInstallAgentHooks(t *testing.T) {
	cfg := &City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "polecat", InstallAgentHooks: []string{"claude"}}},
	}
	patches := Patches{
		Agents: []AgentPatch{{
			Name:              "polecat",
			InstallAgentHooks: []string{"gemini", "copilot"},
		}},
	}
	if err := ApplyPatches(cfg, patches); err != nil {
		t.Fatalf("ApplyPatches: %v", err)
	}
	got := cfg.Agents[0].InstallAgentHooks
	if len(got) != 2 || got[0] != "gemini" || got[1] != "copilot" {
		t.Errorf("InstallAgentHooks = %v, want [gemini copilot]", got)
	}
}
