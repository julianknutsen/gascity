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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, "/tmp"); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir)
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

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir)
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

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir)
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

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir)
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

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir)
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir); err != nil {
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
