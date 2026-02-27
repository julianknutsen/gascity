package config

import (
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestLoadWithIncludes_NoIncludes(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
[workspace]
name = "test"

[[agents]]
name = "mayor"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if prov.Root != "/city/city.toml" {
		t.Errorf("Root = %q, want %q", prov.Root, "/city/city.toml")
	}
	if len(prov.Sources) != 1 {
		t.Errorf("len(Sources) = %d, want 1", len(prov.Sources))
	}
	if len(prov.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", prov.Warnings)
	}
	// Include should be cleared from the result.
	if cfg.Include != nil {
		t.Errorf("Include should be nil, got %v", cfg.Include)
	}
}

func TestLoadWithIncludes_ConcatAgents(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["agents/workers.toml"]

[workspace]
name = "test"

[[agents]]
name = "mayor"
`)
	fs.Files["/city/agents/workers.toml"] = []byte(`
[[agents]]
name = "worker"
dir = "project"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg.Agents[1].Name, "worker")
	}
	if cfg.Agents[1].Dir != "project" {
		t.Errorf("Agents[1].Dir = %q, want %q", cfg.Agents[1].Dir, "project")
	}

	// Provenance.
	if prov.Agents["mayor"] != "/city/city.toml" {
		t.Errorf("mayor source = %q, want root", prov.Agents["mayor"])
	}
	if prov.Agents["project/worker"] != "/city/agents/workers.toml" {
		t.Errorf("worker source = %q, want fragment", prov.Agents["project/worker"])
	}
	if len(prov.Sources) != 2 {
		t.Errorf("len(Sources) = %d, want 2", len(prov.Sources))
	}
}

func TestLoadWithIncludes_ConcatRigs(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["rigs/hw.toml"]

[workspace]
name = "test"

[[rigs]]
name = "project-a"
path = "/tmp/a"
`)
	fs.Files["/city/rigs/hw.toml"] = []byte(`
[[rigs]]
name = "hello-world"
path = "/tmp/hw"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if len(cfg.Rigs) != 2 {
		t.Fatalf("len(Rigs) = %d, want 2", len(cfg.Rigs))
	}
	if cfg.Rigs[0].Name != "project-a" {
		t.Errorf("Rigs[0].Name = %q, want %q", cfg.Rigs[0].Name, "project-a")
	}
	if cfg.Rigs[1].Name != "hello-world" {
		t.Errorf("Rigs[1].Name = %q, want %q", cfg.Rigs[1].Name, "hello-world")
	}
	if prov.Rigs["project-a"] != "/city/city.toml" {
		t.Errorf("project-a source = %q, want root", prov.Rigs["project-a"])
	}
	if prov.Rigs["hello-world"] != "/city/rigs/hw.toml" {
		t.Errorf("hello-world source = %q, want fragment", prov.Rigs["hello-world"])
	}
}

func TestLoadWithIncludes_MultipleFragments(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["a.toml", "b.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/a.toml"] = []byte(`
[[agents]]
name = "alpha"
`)
	fs.Files["/city/b.toml"] = []byte(`
[[agents]]
name = "beta"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "alpha" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "alpha")
	}
	if cfg.Agents[1].Name != "beta" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg.Agents[1].Name, "beta")
	}
	if len(prov.Sources) != 3 {
		t.Errorf("len(Sources) = %d, want 3", len(prov.Sources))
	}
}

func TestLoadWithIncludes_RecursiveIncludeFails(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["fragment.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/fragment.toml"] = []byte(`
include = ["other.toml"]

[[agents]]
name = "worker"
`)
	_, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err == nil {
		t.Fatal("expected error for recursive includes")
	}
	if !strings.Contains(err.Error(), "not allowed in fragments") {
		t.Errorf("error = %q, want contains 'not allowed in fragments'", err)
	}
}

func TestLoadWithIncludes_FragmentNotFound(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["missing.toml"]

[workspace]
name = "test"
`)
	_, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err == nil {
		t.Fatal("expected error for missing fragment")
	}
	if !strings.Contains(err.Error(), "missing.toml") {
		t.Errorf("error = %q, want mention of missing.toml", err)
	}
}

func TestLoadWithIncludes_FragmentParseError(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["bad.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/bad.toml"] = []byte(`{{invalid toml`)
	_, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "bad.toml") {
		t.Errorf("error = %q, want mention of bad.toml", err)
	}
}

func TestLoadWithIncludes_ProviderDeepMerge(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["override.toml"]

[workspace]
name = "test"

[providers.custom]
command = "my-agent"
prompt_mode = "arg"
ready_delay_ms = 5000
`)
	fs.Files["/city/override.toml"] = []byte(`
[providers.custom]
ready_delay_ms = 10000
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	p := cfg.Providers["custom"]
	// Unchanged fields preserved.
	if p.Command != "my-agent" {
		t.Errorf("Command = %q, want %q", p.Command, "my-agent")
	}
	if p.PromptMode != "arg" {
		t.Errorf("PromptMode = %q, want %q", p.PromptMode, "arg")
	}
	// Overridden field.
	if p.ReadyDelayMs != 10000 {
		t.Errorf("ReadyDelayMs = %d, want 10000", p.ReadyDelayMs)
	}
	// Collision warning for ready_delay_ms.
	if len(prov.Warnings) != 1 {
		t.Fatalf("len(Warnings) = %d, want 1: %v", len(prov.Warnings), prov.Warnings)
	}
	if !strings.Contains(prov.Warnings[0], "ready_delay_ms") {
		t.Errorf("warning = %q, want mention of ready_delay_ms", prov.Warnings[0])
	}
}

func TestLoadWithIncludes_ProviderAddsNew(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["providers.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/providers.toml"] = []byte(`
[providers.custom]
command = "my-agent"
prompt_mode = "flag"
prompt_flag = "--prompt"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	p, ok := cfg.Providers["custom"]
	if !ok {
		t.Fatal("provider 'custom' not found")
	}
	if p.Command != "my-agent" {
		t.Errorf("Command = %q, want %q", p.Command, "my-agent")
	}
	if p.PromptFlag != "--prompt" {
		t.Errorf("PromptFlag = %q, want %q", p.PromptFlag, "--prompt")
	}
	// No collision warnings for new provider.
	if len(prov.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_ProviderEnvMerge(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["env.toml"]

[workspace]
name = "test"

[providers.custom]
command = "agent"

[providers.custom.env]
KEY_A = "1"
KEY_B = "2"
`)
	fs.Files["/city/env.toml"] = []byte(`
[providers.custom.env]
KEY_B = "override"
KEY_C = "3"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	env := cfg.Providers["custom"].Env
	if env["KEY_A"] != "1" {
		t.Errorf("KEY_A = %q, want %q", env["KEY_A"], "1")
	}
	if env["KEY_B"] != "override" {
		t.Errorf("KEY_B = %q, want %q", env["KEY_B"], "override")
	}
	if env["KEY_C"] != "3" {
		t.Errorf("KEY_C = %q, want %q", env["KEY_C"], "3")
	}
	// KEY_B collision warning.
	found := false
	for _, w := range prov.Warnings {
		if strings.Contains(w, "KEY_B") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about KEY_B collision, got: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_WorkspaceMerge(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["ws.toml"]

[workspace]
name = "bright-lights"
provider = "claude"
`)
	fs.Files["/city/ws.toml"] = []byte(`
[workspace]
provider = "gemini"
session_template = "custom-{{.Agent}}"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// Name unchanged (fragment didn't define it).
	if cfg.Workspace.Name != "bright-lights" {
		t.Errorf("Name = %q, want %q", cfg.Workspace.Name, "bright-lights")
	}
	// Provider overridden.
	if cfg.Workspace.Provider != "gemini" {
		t.Errorf("Provider = %q, want %q", cfg.Workspace.Provider, "gemini")
	}
	// SessionTemplate added from fragment.
	if cfg.Workspace.SessionTemplate != "custom-{{.Agent}}" {
		t.Errorf("SessionTemplate = %q, want %q", cfg.Workspace.SessionTemplate, "custom-{{.Agent}}")
	}
	// Provenance tracking.
	if prov.Workspace["name"] != "/city/city.toml" {
		t.Errorf("name source = %q, want root", prov.Workspace["name"])
	}
	if prov.Workspace["provider"] != "/city/ws.toml" {
		t.Errorf("provider source = %q, want fragment", prov.Workspace["provider"])
	}
	// Collision warning for provider.
	found := false
	for _, w := range prov.Warnings {
		if strings.Contains(w, "workspace.provider") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about workspace.provider collision, got: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_PromptTemplatePathAdjustment(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["agents/team.toml"]

[workspace]
name = "test"

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"
`)
	fs.Files["/city/agents/team.toml"] = []byte(`
[[agents]]
name = "worker"
dir = "project"
prompt_template = "prompts/worker.md"
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// Root agent's path unchanged (already city-root-relative).
	if cfg.Agents[0].PromptTemplate != "prompts/mayor.md" {
		t.Errorf("mayor prompt_template = %q, want %q",
			cfg.Agents[0].PromptTemplate, "prompts/mayor.md")
	}
	// Fragment agent's path adjusted to city-root-relative.
	// "prompts/worker.md" relative to /city/agents/ → "agents/prompts/worker.md"
	want := "agents/prompts/worker.md"
	if cfg.Agents[1].PromptTemplate != want {
		t.Errorf("worker prompt_template = %q, want %q",
			cfg.Agents[1].PromptTemplate, want)
	}
}

func TestLoadWithIncludes_CityRootPath(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["agents/team.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/agents/team.toml"] = []byte(`
[[agents]]
name = "worker"
prompt_template = "//prompts/worker.md"
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// "//" prefix resolves to city root.
	if cfg.Agents[0].PromptTemplate != "prompts/worker.md" {
		t.Errorf("prompt_template = %q, want %q",
			cfg.Agents[0].PromptTemplate, "prompts/worker.md")
	}
}

func TestLoadWithIncludes_IncludeCleared(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["a.toml"]

[workspace]
name = "test"
`)
	fs.Files["/city/a.toml"] = []byte(`
[[agents]]
name = "worker"
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if cfg.Include != nil {
		t.Errorf("Include should be nil after merge, got %v", cfg.Include)
	}
}

func TestLoadWithIncludes_SimpleSectionOverride(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["infra.toml"]

[workspace]
name = "test"

[beads]
provider = "bd"
`)
	fs.Files["/city/infra.toml"] = []byte(`
[beads]
provider = "file"
`)
	cfg, _, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if cfg.Beads.Provider != "file" {
		t.Errorf("Beads.Provider = %q, want %q", cfg.Beads.Provider, "file")
	}
}

func TestResolveConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		p        string
		declDir  string
		cityRoot string
		want     string
	}{
		{"relative", "agents/mayor.toml", "/city", "/city", "/city/agents/mayor.toml"},
		{"absolute", "/etc/config.toml", "/city", "/city", "/etc/config.toml"},
		{"city-root", "//prompts/mayor.md", "/city/agents", "/city", "/city/prompts/mayor.md"},
		{"nested-relative", "sub/file.toml", "/city/agents", "/city", "/city/agents/sub/file.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveConfigPath(tt.p, tt.declDir, tt.cityRoot)
			if got != tt.want {
				t.Errorf("resolveConfigPath(%q, %q, %q) = %q, want %q",
					tt.p, tt.declDir, tt.cityRoot, got, tt.want)
			}
		})
	}
}

func TestAdjustFragmentPath(t *testing.T) {
	tests := []struct {
		name     string
		p        string
		fragDir  string
		cityRoot string
		want     string
	}{
		{"empty", "", "/city/agents", "/city", ""},
		{"absolute", "/abs/path.md", "/city/agents", "/city", "/abs/path.md"},
		{"city-root", "//prompts/foo.md", "/city/agents", "/city", "prompts/foo.md"},
		{"relative", "prompts/foo.md", "/city/agents", "/city", "agents/prompts/foo.md"},
		{"same-dir", "foo.md", "/city", "/city", "foo.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustFragmentPath(tt.p, tt.fragDir, tt.cityRoot)
			if got != tt.want {
				t.Errorf("adjustFragmentPath(%q, %q, %q) = %q, want %q",
					tt.p, tt.fragDir, tt.cityRoot, got, tt.want)
			}
		})
	}
}

func TestLoadWithIncludes_WorkspaceProvenanceTracking(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
[workspace]
name = "test"
provider = "claude"
`)
	_, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if prov.Workspace["name"] != "/city/city.toml" {
		t.Errorf("name source = %q, want root", prov.Workspace["name"])
	}
	if prov.Workspace["provider"] != "/city/city.toml" {
		t.Errorf("provider source = %q, want root", prov.Workspace["provider"])
	}
	// session_template not defined → not in provenance.
	if _, ok := prov.Workspace["session_template"]; ok {
		t.Error("session_template should not be in provenance (not defined)")
	}
}

func TestLoadWithIncludes_MergeTopologies(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["remote.toml"]

[workspace]
name = "test"

[topologies.gastown]
source = "https://github.com/example/gastown"
ref = "v1.0.0"
`)
	fs.Files["/city/remote.toml"] = []byte(`
[topologies.ralph]
source = "https://github.com/example/ralph"
ref = "main"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if len(cfg.Topologies) != 2 {
		t.Fatalf("len(Topologies) = %d, want 2", len(cfg.Topologies))
	}
	if cfg.Topologies["gastown"].Source != "https://github.com/example/gastown" {
		t.Errorf("gastown source = %q", cfg.Topologies["gastown"].Source)
	}
	if cfg.Topologies["ralph"].Source != "https://github.com/example/ralph" {
		t.Errorf("ralph source = %q", cfg.Topologies["ralph"].Source)
	}
	if len(prov.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_MergeTopologies_Collision(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["override.toml"]

[workspace]
name = "test"

[topologies.gastown]
source = "https://github.com/example/gastown"
ref = "v1.0.0"
`)
	fs.Files["/city/override.toml"] = []byte(`
[topologies.gastown]
source = "https://github.com/other/gastown"
ref = "v2.0.0"
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// Last writer wins.
	if cfg.Topologies["gastown"].Ref != "v2.0.0" {
		t.Errorf("gastown ref = %q, want v2.0.0", cfg.Topologies["gastown"].Ref)
	}
	// Collision warning.
	found := false
	for _, w := range prov.Warnings {
		if strings.Contains(w, "gastown") && strings.Contains(w, "redefined") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected collision warning for gastown, got: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_WorkspaceInstallAgentHooksMerge(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
include = ["frag.toml"]

[workspace]
name = "test"
install_agent_hooks = ["claude"]
`)
	fs.Files["/city/frag.toml"] = []byte(`
[workspace]
install_agent_hooks = ["gemini", "copilot"]
`)
	cfg, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	// Fragment replaces root.
	got := cfg.Workspace.InstallAgentHooks
	if len(got) != 2 || got[0] != "gemini" || got[1] != "copilot" {
		t.Errorf("InstallAgentHooks = %v, want [gemini copilot]", got)
	}
	// Provenance tracks the override.
	if prov.Workspace["install_agent_hooks"] != "/city/frag.toml" {
		t.Errorf("provenance = %q, want frag.toml", prov.Workspace["install_agent_hooks"])
	}
	// Should produce a collision warning.
	foundWarning := false
	for _, w := range prov.Warnings {
		if w == `workspace.install_agent_hooks redefined by "/city/frag.toml"` {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected collision warning, got: %v", prov.Warnings)
	}
}

func TestLoadWithIncludes_WorkspaceInstallAgentHooksProvenance(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`
[workspace]
name = "test"
install_agent_hooks = ["claude"]
`)
	_, prov, err := LoadWithIncludes(fs, "/city/city.toml")
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}
	if prov.Workspace["install_agent_hooks"] != "/city/city.toml" {
		t.Errorf("provenance = %q, want root", prov.Workspace["install_agent_hooks"])
	}
}

func TestAdjustAgentPaths_SourceDirSet(t *testing.T) {
	agents := []Agent{
		{Name: "worker", PromptTemplate: "prompts/worker.md"},
		{Name: "boss"},
	}
	adjustAgentPaths(agents, "/city/fragments", "/city")

	// Both agents should get SourceDir set to fragment dir.
	for _, a := range agents {
		if a.SourceDir != "/city/fragments" {
			t.Errorf("agent %q: SourceDir = %q, want /city/fragments", a.Name, a.SourceDir)
		}
	}
}

func TestAdjustAgentPaths_SessionSetupScriptAdjusted(t *testing.T) {
	agents := []Agent{
		{Name: "worker", SessionSetupScript: "scripts/setup.sh"},
		{Name: "boss", SessionSetupScript: "//scripts/global.sh"},
		{Name: "plain"},
	}
	adjustAgentPaths(agents, "/city/fragments", "/city")

	// Relative path: resolved fragment-relative → city-root-relative.
	if agents[0].SessionSetupScript != "fragments/scripts/setup.sh" {
		t.Errorf("worker script = %q, want fragments/scripts/setup.sh", agents[0].SessionSetupScript)
	}
	// "//" path: resolved to city root.
	if agents[1].SessionSetupScript != "scripts/global.sh" {
		t.Errorf("boss script = %q, want scripts/global.sh", agents[1].SessionSetupScript)
	}
	// Empty: unchanged.
	if agents[2].SessionSetupScript != "" {
		t.Errorf("plain script = %q, want empty", agents[2].SessionSetupScript)
	}
}

func TestAdjustAgentPaths_OverlayDirAdjusted(t *testing.T) {
	agents := []Agent{
		{Name: "worker", OverlayDir: "overlays/worker"},
		{Name: "boss", OverlayDir: "//overlays/global"},
		{Name: "plain"},
	}
	adjustAgentPaths(agents, "/city/fragments", "/city")

	// Relative path: resolved fragment-relative → city-root-relative.
	if agents[0].OverlayDir != "fragments/overlays/worker" {
		t.Errorf("worker overlay = %q, want fragments/overlays/worker", agents[0].OverlayDir)
	}
	// "//" path: resolved to city root.
	if agents[1].OverlayDir != "overlays/global" {
		t.Errorf("boss overlay = %q, want overlays/global", agents[1].OverlayDir)
	}
	// Empty: unchanged.
	if agents[2].OverlayDir != "" {
		t.Errorf("plain overlay = %q, want empty", agents[2].OverlayDir)
	}
}
