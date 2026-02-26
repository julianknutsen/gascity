package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

// writeFile is a test helper that creates a file in dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandTopologies_Basic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
prompt_template = "prompts/witness.md"

[[agents]]
name = "refinery"
`)

	writeFile(t, dir, "topologies/gastown/prompts/witness.md", "you are a witness")

	cfg := &City{
		Rigs: []Rig{
			{Name: "hello-world", Path: "/home/user/hello-world", Topology: "topologies/gastown"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	// Agents should have dir stamped to rig name.
	for _, a := range cfg.Agents {
		if a.Dir != "hello-world" {
			t.Errorf("agent %q: dir = %q, want %q", a.Name, a.Dir, "hello-world")
		}
	}
	// witness should have adjusted prompt_template path.
	if !strings.Contains(cfg.Agents[0].PromptTemplate, "prompts/witness.md") {
		t.Errorf("witness prompt_template = %q, want to contain prompts/witness.md", cfg.Agents[0].PromptTemplate)
	}
}

func TestExpandTopologies_MultipleRigs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "polecat"
[agents.pool]
min = 0
max = 3
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "proj-a", Path: "/a", Topology: "topologies/gastown"},
			{Name: "proj-b", Path: "/b", Topology: "topologies/gastown"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	// Each rig gets its own stamped copy.
	if cfg.Agents[0].Dir != "proj-a" {
		t.Errorf("first polecat dir = %q, want proj-a", cfg.Agents[0].Dir)
	}
	if cfg.Agents[1].Dir != "proj-b" {
		t.Errorf("second polecat dir = %q, want proj-b", cfg.Agents[1].Dir)
	}
	// Pool config should be preserved.
	if cfg.Agents[0].Pool == nil || cfg.Agents[0].Pool.Max != 3 {
		t.Errorf("first polecat pool not preserved")
	}
}

func TestExpandTopologies_NoTopology(t *testing.T) {
	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
		Rigs:   []Rig{{Name: "simple", Path: "/simple"}},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, "/tmp", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("got %d agents, want 1 (unchanged)", len(cfg.Agents))
	}
}

func TestExpandTopologies_MixedRigs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/basic/topology.toml", `
[topology]
name = "basic"
version = "0.1.0"
schema = 1

[[agents]]
name = "worker"
`)

	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
		Rigs: []Rig{
			{Name: "with-topo", Path: "/a", Topology: "topologies/basic"},
			{Name: "no-topo", Path: "/b"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("first agent should be mayor, got %q", cfg.Agents[0].Name)
	}
	if cfg.Agents[1].Name != "worker" || cfg.Agents[1].Dir != "with-topo" {
		t.Errorf("second agent: name=%q dir=%q, want worker/with-topo", cfg.Agents[1].Name, cfg.Agents[1].Dir)
	}
}

func TestExpandTopologies_OverrideDirStamp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
`)

	dirOverride := "services/api"
	cfg := &City{
		Rigs: []Rig{
			{
				Name:     "monorepo",
				Path:     "/home/user/mono",
				Topology: "topologies/gt",
				Overrides: []AgentOverride{
					{Agent: "witness", Dir: &dirOverride},
				},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if cfg.Agents[0].Dir != "services/api" {
		t.Errorf("dir = %q, want %q", cfg.Agents[0].Dir, "services/api")
	}
}

func TestExpandTopologies_OverridePool(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "polecat"
[agents.pool]
min = 0
max = 3
`)

	maxOverride := 10
	cfg := &City{
		Rigs: []Rig{
			{
				Name:     "big-project",
				Path:     "/big",
				Topology: "topologies/gt",
				Overrides: []AgentOverride{
					{Agent: "polecat", Pool: &PoolOverride{Max: &maxOverride}},
				},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if cfg.Agents[0].Pool == nil {
		t.Fatal("pool is nil")
	}
	if cfg.Agents[0].Pool.Max != 10 {
		t.Errorf("pool.max = %d, want 10", cfg.Agents[0].Pool.Max)
	}
	if cfg.Agents[0].Pool.Min != 0 {
		t.Errorf("pool.min = %d, want 0 (preserved from topology)", cfg.Agents[0].Pool.Min)
	}
}

func TestExpandTopologies_OverrideSuspend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
`)

	suspended := true
	cfg := &City{
		Rigs: []Rig{
			{
				Name:     "hw",
				Path:     "/hw",
				Topology: "topologies/gt",
				Overrides: []AgentOverride{
					{Agent: "witness", Suspended: &suspended},
				},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if !cfg.Agents[0].Suspended {
		t.Error("witness should be suspended")
	}
}

func TestExpandTopologies_OverrideNotFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
`)

	suspended := true
	cfg := &City{
		Rigs: []Rig{
			{
				Name:     "hw",
				Path:     "/hw",
				Topology: "topologies/gt",
				Overrides: []AgentOverride{
					{Agent: "nonexistent", Suspended: &suspended},
				},
			},
		},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent override target")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention nonexistent, got: %v", err)
	}
}

func TestExpandTopologies_MissingTopologyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/empty/.keep", "")

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/empty"},
		},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for missing topology.toml")
	}
}

func TestExpandTopologies_BadSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/future/topology.toml", `
[topology]
name = "future"
version = "9.0.0"
schema = 99
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/future"},
		},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for unsupported schema")
	}
	if !strings.Contains(err.Error(), "schema 99 not supported") {
		t.Errorf("error should mention schema, got: %v", err)
	}
}

func TestExpandTopologies_MissingName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/bad/topology.toml", `
[topology]
version = "1.0.0"
schema = 1
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/bad"},
		},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for missing topology name")
	}
}

func TestExpandTopologies_MissingSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/bad/topology.toml", `
[topology]
name = "bad"
version = "1.0.0"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/bad"},
		},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for missing schema")
	}
}

func TestExpandTopologies_PromptPathResolution(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
prompt_template = "prompts/witness.md"

[[agents]]
name = "refinery"
prompt_template = "//prompts/shared.md"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// Relative path: resolved relative to topology dir, then made city-root-relative.
	if cfg.Agents[0].PromptTemplate != "topologies/gt/prompts/witness.md" {
		t.Errorf("witness prompt = %q, want topologies/gt/prompts/witness.md", cfg.Agents[0].PromptTemplate)
	}
	// "//" path: resolved to city root.
	if cfg.Agents[1].PromptTemplate != "prompts/shared.md" {
		t.Errorf("refinery prompt = %q, want prompts/shared.md", cfg.Agents[1].PromptTemplate)
	}
}

func TestExpandTopologies_ProvidersMerged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[providers.codex]
command = "codex"
args = ["--full-auto"]

[[agents]]
name = "witness"
provider = "codex"
`)

	cfg := &City{
		Providers: map[string]ProviderSpec{
			"claude": {Command: "claude"},
		},
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// codex provider should be added.
	if _, ok := cfg.Providers["codex"]; !ok {
		t.Error("codex provider should be merged from topology")
	}
	// claude should still exist.
	if _, ok := cfg.Providers["claude"]; !ok {
		t.Error("claude provider should still exist")
	}
}

func TestExpandTopologies_ProvidersNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[providers.claude]
command = "claude-from-topo"

[[agents]]
name = "witness"
`)

	cfg := &City{
		Providers: map[string]ProviderSpec{
			"claude": {Command: "claude-original"},
		},
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// City's existing provider should NOT be overwritten by topology.
	if cfg.Providers["claude"].Command != "claude-original" {
		t.Errorf("claude command = %q, want claude-original (should not be overwritten)", cfg.Providers["claude"].Command)
	}
}

func TestTopologyContentHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topology.toml", `[topology]
name = "test"
schema = 1
`)
	writeFile(t, dir, "prompts/witness.md", "witness prompt")

	h1 := TopologyContentHash(fsys.OSFS{}, dir)
	h2 := TopologyContentHash(fsys.OSFS{}, dir)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestTopologyContentHash_ChangesOnModification(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topology.toml", `[topology]
name = "test"
schema = 1
`)

	h1 := TopologyContentHash(fsys.OSFS{}, dir)

	// Modify the file.
	writeFile(t, dir, "topology.toml", `[topology]
name = "test-modified"
schema = 1
`)

	h2 := TopologyContentHash(fsys.OSFS{}, dir)
	if h1 == h2 {
		t.Error("hash should change when file content changes")
	}
}

func TestTopologyContentHashRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topology.toml", "test")
	writeFile(t, dir, "prompts/a.md", "prompt a")
	writeFile(t, dir, "prompts/b.md", "prompt b")

	h1 := TopologyContentHashRecursive(fsys.OSFS{}, dir)
	if h1 == "" {
		t.Error("hash should not be empty")
	}

	// Should be deterministic.
	h2 := TopologyContentHashRecursive(fsys.OSFS{}, dir)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}

	// Change a subdirectory file.
	writeFile(t, dir, "prompts/a.md", "modified prompt a")
	h3 := TopologyContentHashRecursive(fsys.OSFS{}, dir)
	if h3 == h1 {
		t.Error("hash should change when subdirectory file changes")
	}
}

func TestExpandTopologies_ViaLoadWithIncludes(t *testing.T) {
	dir := t.TempDir()

	// Write topology.
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
prompt_template = "prompts/witness.md"
`)
	writeFile(t, dir, "topologies/gt/prompts/witness.md", "you are a witness")

	// Write city.toml with a rig that references the topology.
	writeFile(t, dir, "city.toml", `
[workspace]
name = "test-city"

[[agents]]
name = "mayor"

[[rigs]]
name = "hello-world"
path = "/home/user/hw"
topology = "topologies/gt"
`)

	cfg, prov, err := LoadWithIncludes(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}

	// Should have mayor + witness.
	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("first agent = %q, want mayor", cfg.Agents[0].Name)
	}
	if cfg.Agents[1].Name != "witness" {
		t.Errorf("second agent = %q, want witness", cfg.Agents[1].Name)
	}
	if cfg.Agents[1].Dir != "hello-world" {
		t.Errorf("witness dir = %q, want hello-world", cfg.Agents[1].Dir)
	}

	// Provenance should track topology agents.
	if src, ok := prov.Agents["hello-world/witness"]; !ok {
		t.Error("provenance should track hello-world/witness")
	} else if !strings.Contains(src, "topology.toml") {
		t.Errorf("witness provenance = %q, want to contain topology.toml", src)
	}
}

func TestExpandTopologies_OverrideEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
[agents.env]
ROLE = "witness"
DEBUG = "false"
`)

	cfg := &City{
		Rigs: []Rig{
			{
				Name:     "hw",
				Path:     "/hw",
				Topology: "topologies/gt",
				Overrides: []AgentOverride{
					{
						Agent:     "witness",
						Env:       map[string]string{"DEBUG": "true", "EXTRA": "val"},
						EnvRemove: []string{"ROLE"},
					},
				},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	env := cfg.Agents[0].Env
	if env["DEBUG"] != "true" {
		t.Errorf("DEBUG = %q, want true", env["DEBUG"])
	}
	if env["EXTRA"] != "val" {
		t.Errorf("EXTRA = %q, want val", env["EXTRA"])
	}
	if _, ok := env["ROLE"]; ok {
		t.Error("ROLE should have been removed")
	}
}

func TestTopologySummary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "2.1.0"
schema = 1

[[agents]]
name = "witness"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
			{Name: "simple", Path: "/simple"},
		},
	}

	summary := TopologySummary(cfg, fsys.OSFS{}, dir)

	if _, ok := summary["simple"]; ok {
		t.Error("simple rig (no topology) should not appear in summary")
	}
	s, ok := summary["hw"]
	if !ok {
		t.Fatal("hw should appear in summary")
	}
	if !strings.Contains(s, "gastown") {
		t.Errorf("summary should contain topology name, got: %q", s)
	}
	if !strings.Contains(s, "2.1.0") {
		t.Errorf("summary should contain version, got: %q", s)
	}
}

func TestResolveNamedTopologies_Basic(t *testing.T) {
	cfg := &City{
		Topologies: map[string]TopologySource{
			"gastown": {Source: "https://example.com/gastown.git"},
		},
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "gastown"},
		},
	}

	resolveNamedTopologies(cfg, "/city")

	want := "/city/.gc/topologies/gastown"
	if cfg.Rigs[0].Topology != want {
		t.Errorf("Topology = %q, want %q", cfg.Rigs[0].Topology, want)
	}
}

func TestResolveNamedTopologies_WithPath(t *testing.T) {
	cfg := &City{
		Topologies: map[string]TopologySource{
			"mono": {Source: "https://example.com/mono.git", Path: "packages/topo"},
		},
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "mono"},
		},
	}

	resolveNamedTopologies(cfg, "/city")

	want := "/city/.gc/topologies/mono/packages/topo"
	if cfg.Rigs[0].Topology != want {
		t.Errorf("Topology = %q, want %q", cfg.Rigs[0].Topology, want)
	}
}

func TestResolveNamedTopologies_LocalPathUnchanged(t *testing.T) {
	cfg := &City{
		Topologies: map[string]TopologySource{
			"gastown": {Source: "https://example.com/gastown.git"},
		},
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/mine"},
		},
	}

	resolveNamedTopologies(cfg, "/city")

	// "topologies/mine" doesn't match any key in Topologies, so it stays as-is.
	if cfg.Rigs[0].Topology != "topologies/mine" {
		t.Errorf("Topology = %q, want %q", cfg.Rigs[0].Topology, "topologies/mine")
	}
}

func TestResolveNamedTopologies_EmptyTopologiesMap(t *testing.T) {
	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/local"},
		},
	}

	resolveNamedTopologies(cfg, "/city")

	// No topologies map — should be a no-op.
	if cfg.Rigs[0].Topology != "topologies/local" {
		t.Errorf("Topology = %q, want %q", cfg.Rigs[0].Topology, "topologies/local")
	}
}

func TestHasTopologyRigs(t *testing.T) {
	if HasTopologyRigs(nil) {
		t.Error("nil rigs should return false")
	}
	if HasTopologyRigs([]Rig{{Name: "a", Path: "/a"}}) {
		t.Error("rig without topology should return false")
	}
	if !HasTopologyRigs([]Rig{{Name: "a", Path: "/a", Topology: "topo"}}) {
		t.Error("rig with topology should return true")
	}
}

func TestExpandTopologies_OverrideInstallAgentHooks(t *testing.T) {
	fs := fsys.NewFake()
	topoTOML := `[topology]
name = "test"
schema = 1

[[agents]]
name = "polecat"
install_agent_hooks = ["claude"]
`
	fs.Files["/city/topologies/test/topology.toml"] = []byte(topoTOML)

	cfg := &City{
		Workspace: Workspace{Name: "test"},
		Rigs: []Rig{{
			Name:     "myrig",
			Path:     "/repo",
			Topology: "topologies/test",
			Overrides: []AgentOverride{{
				Agent:             "polecat",
				InstallAgentHooks: []string{"gemini", "copilot"},
			}},
		}},
	}

	if err := ExpandTopologies(cfg, fs, "/city", nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// Find the expanded agent.
	var found *Agent
	for i := range cfg.Agents {
		if cfg.Agents[i].Name == "polecat" {
			found = &cfg.Agents[i]
			break
		}
	}
	if found == nil {
		t.Fatal("polecat agent not found after expansion")
	}
	if len(found.InstallAgentHooks) != 2 || found.InstallAgentHooks[0] != "gemini" {
		t.Errorf("InstallAgentHooks = %v, want [gemini copilot]", found.InstallAgentHooks)
	}
}

// --- City topology tests ---

func TestExpandCityTopology_Basic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"

[[agents]]
name = "deacon"
`)
	writeFile(t, dir, "topologies/gastown/prompts/mayor.md", "you are the mayor")

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gastown"},
		Agents:    []Agent{{Name: "existing"}},
	}

	formulaDir, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	// Should have 3 agents: mayor, deacon (from topology), then existing.
	if len(cfg.Agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("first agent = %q, want mayor", cfg.Agents[0].Name)
	}
	if cfg.Agents[1].Name != "deacon" {
		t.Errorf("second agent = %q, want deacon", cfg.Agents[1].Name)
	}
	if cfg.Agents[2].Name != "existing" {
		t.Errorf("third agent = %q, want existing", cfg.Agents[2].Name)
	}

	// City topology agents should have dir="" (city-scoped).
	for _, a := range cfg.Agents[:2] {
		if a.Dir != "" {
			t.Errorf("city topology agent %q: dir = %q, want empty", a.Name, a.Dir)
		}
	}

	// No formulas configured → empty string.
	if formulaDir != "" {
		t.Errorf("formulaDir = %q, want empty", formulaDir)
	}
}

func TestExpandCityTopology_FormulasDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "mayor"
`)
	writeFile(t, dir, "topologies/gastown/formulas/mol-a.formula.toml", "test formula")

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gastown"},
	}

	formulaDir, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	want := filepath.Join(dir, "topologies/gastown/formulas")
	if formulaDir != want {
		t.Errorf("formulaDir = %q, want %q", formulaDir, want)
	}
}

func TestExpandCityTopology_NoTopology(t *testing.T) {
	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
	}

	formulaDir, err := ExpandCityTopology(cfg, fsys.OSFS{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if formulaDir != "" {
		t.Errorf("formulaDir = %q, want empty", formulaDir)
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("got %d agents, want 1 (unchanged)", len(cfg.Agents))
	}
}

func TestExpandCityTopology_ProvidersMerged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[providers.codex]
command = "codex"

[[agents]]
name = "mayor"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gt"},
		Providers: map[string]ProviderSpec{
			"claude": {Command: "claude"},
		},
	}

	_, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	if _, ok := cfg.Providers["codex"]; !ok {
		t.Error("codex provider should be merged from city topology")
	}
	if cfg.Providers["claude"].Command != "claude" {
		t.Error("existing claude provider should not be overwritten")
	}
}

// --- FormulaLayers tests ---

func TestFormulaLayers_CityOnly(t *testing.T) {
	fl := ComputeFormulaLayers("/city/topo/formulas", "/city/.gc/formulas", nil, nil, "/city")

	if len(fl.City) != 2 {
		t.Fatalf("City layers = %d, want 2", len(fl.City))
	}
	if fl.City[0] != "/city/topo/formulas" {
		t.Errorf("City[0] = %q, want city topo formulas", fl.City[0])
	}
	if fl.City[1] != "/city/.gc/formulas" {
		t.Errorf("City[1] = %q, want city local formulas", fl.City[1])
	}
	if len(fl.Rigs) != 0 {
		t.Errorf("Rigs = %d entries, want 0", len(fl.Rigs))
	}
}

func TestFormulaLayers_WithRigs(t *testing.T) {
	rigTopoFormulas := map[string]string{
		"hw": "/city/topologies/gt/formulas",
	}
	rigs := []Rig{
		{Name: "hw", Path: "/home/user/hw", FormulasDir: "local-formulas"},
	}

	fl := ComputeFormulaLayers("/city/topo/formulas", "/city/.gc/formulas", rigTopoFormulas, rigs, "/city")

	// City layers should be [city-topo, city-local].
	if len(fl.City) != 2 {
		t.Fatalf("City layers = %d, want 2", len(fl.City))
	}

	// Rig "hw" should have 4 layers.
	hwLayers := fl.Rigs["hw"]
	if len(hwLayers) != 4 {
		t.Fatalf("hw layers = %d, want 4", len(hwLayers))
	}
	if hwLayers[0] != "/city/topo/formulas" {
		t.Errorf("hw[0] = %q, want city topo", hwLayers[0])
	}
	if hwLayers[1] != "/city/.gc/formulas" {
		t.Errorf("hw[1] = %q, want city local", hwLayers[1])
	}
	if hwLayers[2] != "/city/topologies/gt/formulas" {
		t.Errorf("hw[2] = %q, want rig topo", hwLayers[2])
	}
	// Layer 4: rig local formulas_dir resolved relative to city root.
	if hwLayers[3] != filepath.Join("/city", "local-formulas") {
		t.Errorf("hw[3] = %q, want rig local formulas", hwLayers[3])
	}
}

func TestFormulaLayers_RigLocalFormulasOnly(t *testing.T) {
	rigs := []Rig{
		{Name: "hw", Path: "/home/user/hw", FormulasDir: "formulas"},
	}

	fl := ComputeFormulaLayers("", "", nil, rigs, "/city")

	// City should have no layers (no topology, no local).
	if len(fl.City) != 0 {
		t.Errorf("City layers = %d, want 0", len(fl.City))
	}

	// Rig should have just the local layer.
	hwLayers := fl.Rigs["hw"]
	if len(hwLayers) != 1 {
		t.Fatalf("hw layers = %d, want 1", len(hwLayers))
	}
	if hwLayers[0] != filepath.Join("/city", "formulas") {
		t.Errorf("hw[0] = %q, want rig local formulas", hwLayers[0])
	}
}

func TestFormulaLayers_NoFormulas(t *testing.T) {
	rigs := []Rig{
		{Name: "hw", Path: "/home/user/hw"},
	}

	fl := ComputeFormulaLayers("", "", nil, rigs, "/city")

	if len(fl.City) != 0 {
		t.Errorf("City layers = %d, want 0", len(fl.City))
	}
	// Rig with no formula sources should not appear in map.
	if _, ok := fl.Rigs["hw"]; ok {
		t.Error("hw should not appear in Rigs (no formula layers)")
	}
}

// --- Periodic formula merge tests ---

func TestMergePeriodicFormulas_CityWins(t *testing.T) {
	// City defines mol-digest with 8h; topology ships same formula with 24h.
	// City-level should win.
	existing := []PeriodicFormula{
		{Formula: "mol-digest-generate", Gate: "cooldown", Interval: "8h", Pool: "dog"},
	}
	incoming := []PeriodicFormula{
		{Formula: "mol-digest-generate", Gate: "cooldown", Interval: "24h", Pool: "dog"},
		{Formula: "mol-orphan-scan", Gate: "cooldown", Interval: "12h"},
	}

	result := mergePeriodicFormulas(existing, incoming)

	if len(result) != 2 {
		t.Fatalf("got %d periodic formulas, want 2", len(result))
	}
	// First should be the city version (8h), not topology (24h).
	if result[0].Interval != "8h" {
		t.Errorf("digest interval = %q, want 8h (city wins)", result[0].Interval)
	}
	// Second should be the new one from topology.
	if result[1].Formula != "mol-orphan-scan" {
		t.Errorf("second formula = %q, want mol-orphan-scan", result[1].Formula)
	}
}

func TestMergePeriodicFormulas_EmptyIncoming(t *testing.T) {
	existing := []PeriodicFormula{
		{Formula: "mol-digest", Gate: "cooldown", Interval: "24h"},
	}
	result := mergePeriodicFormulas(existing, nil)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1 (unchanged)", len(result))
	}
}

func TestMergePeriodicFormulas_EmptyExisting(t *testing.T) {
	incoming := []PeriodicFormula{
		{Formula: "mol-digest", Gate: "cooldown", Interval: "24h"},
	}
	result := mergePeriodicFormulas(nil, incoming)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	if result[0].Formula != "mol-digest" {
		t.Errorf("formula = %q, want mol-digest", result[0].Formula)
	}
}

func TestExpandTopologies_PeriodicFormulasMerged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[formulas.periodic]]
formula = "mol-topo-task"
gate = "cooldown"
interval = "24h"
pool = "dog"

[[agents]]
name = "witness"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/home/user/hw", Topology: "topologies/gt"},
		},
		Formulas: FormulasConfig{
			Periodic: []PeriodicFormula{
				{Formula: "mol-city-task", Gate: "cooldown", Interval: "6h"},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if len(cfg.Formulas.Periodic) != 2 {
		t.Fatalf("got %d periodic formulas, want 2", len(cfg.Formulas.Periodic))
	}
	// City formula first, topology formula appended.
	if cfg.Formulas.Periodic[0].Formula != "mol-city-task" {
		t.Errorf("periodic[0] = %q, want mol-city-task", cfg.Formulas.Periodic[0].Formula)
	}
	if cfg.Formulas.Periodic[1].Formula != "mol-topo-task" {
		t.Errorf("periodic[1] = %q, want mol-topo-task", cfg.Formulas.Periodic[1].Formula)
	}
}

func TestExpandTopologies_PeriodicOverrideByCityConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[formulas.periodic]]
formula = "mol-digest-generate"
gate = "cooldown"
interval = "24h"
pool = "dog"

[[agents]]
name = "witness"
`)

	// City config overrides the same formula with 8h.
	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/home/user/hw", Topology: "topologies/gt"},
		},
		Formulas: FormulasConfig{
			Periodic: []PeriodicFormula{
				{Formula: "mol-digest-generate", Gate: "cooldown", Interval: "8h", Pool: "dog"},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// Should still be 1 — topology duplicate skipped.
	if len(cfg.Formulas.Periodic) != 1 {
		t.Fatalf("got %d periodic formulas, want 1 (city override wins)", len(cfg.Formulas.Periodic))
	}
	if cfg.Formulas.Periodic[0].Interval != "8h" {
		t.Errorf("interval = %q, want 8h (city wins over topology 24h)", cfg.Formulas.Periodic[0].Interval)
	}
}

func TestExpandCityTopology_PeriodicFormulasMerged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[formulas.periodic]]
formula = "mol-topo-digest"
gate = "cooldown"
interval = "12h"

[[agents]]
name = "mayor"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gt"},
		Formulas: FormulasConfig{
			Periodic: []PeriodicFormula{
				{Formula: "mol-topo-digest", Gate: "cooldown", Interval: "4h"},
			},
		},
	}

	_, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	// City config has mol-topo-digest with 4h, topology has same with 12h.
	// City should win — no duplicate.
	if len(cfg.Formulas.Periodic) != 1 {
		t.Fatalf("got %d periodic formulas, want 1", len(cfg.Formulas.Periodic))
	}
	if cfg.Formulas.Periodic[0].Interval != "4h" {
		t.Errorf("interval = %q, want 4h (city override)", cfg.Formulas.Periodic[0].Interval)
	}
}

func TestExpandTopologies_FormulaDirsRecorded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "witness"
`)
	writeFile(t, dir, "topologies/gt/formulas/mol-a.formula.toml", "test")

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/home/user/hw", Topology: "topologies/gt"},
		},
	}

	rigFormulaDirs := make(map[string]string)
	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, rigFormulaDirs); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	want := filepath.Join(dir, "topologies/gt/formulas")
	if got := rigFormulaDirs["hw"]; got != want {
		t.Errorf("rigFormulaDirs[hw] = %q, want %q", got, want)
	}
}
