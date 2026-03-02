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
	if !HasTopologyRigs([]Rig{{Name: "a", Path: "/a", RigTopologies: []string{"topo"}}}) {
		t.Error("rig with plural topologies should return true")
	}
}

// --- EffectiveCityTopologies tests ---

func TestEffectiveCityTopologies_SingularOnly(t *testing.T) {
	ws := Workspace{Topology: "topologies/gastown"}
	got := EffectiveCityTopologies(ws)
	if len(got) != 1 || got[0] != "topologies/gastown" {
		t.Errorf("got %v, want [topologies/gastown]", got)
	}
}

func TestEffectiveCityTopologies_PluralOnly(t *testing.T) {
	ws := Workspace{CityTopologies: []string{"topologies/a", "topologies/b"}}
	got := EffectiveCityTopologies(ws)
	if len(got) != 2 || got[0] != "topologies/a" || got[1] != "topologies/b" {
		t.Errorf("got %v, want [topologies/a topologies/b]", got)
	}
}

func TestEffectiveCityTopologies_Both(t *testing.T) {
	ws := Workspace{
		Topology:       "topologies/singular",
		CityTopologies: []string{"topologies/a", "topologies/b"},
	}
	got := EffectiveCityTopologies(ws)
	if len(got) != 3 || got[0] != "topologies/singular" || got[1] != "topologies/a" || got[2] != "topologies/b" {
		t.Errorf("got %v, want [topologies/singular topologies/a topologies/b]", got)
	}
}

func TestEffectiveCityTopologies_Neither(t *testing.T) {
	ws := Workspace{}
	got := EffectiveCityTopologies(ws)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- EffectiveRigTopologies tests ---

func TestEffectiveRigTopologies_SingularOnly(t *testing.T) {
	rig := Rig{Topology: "topologies/gastown"}
	got := EffectiveRigTopologies(rig)
	if len(got) != 1 || got[0] != "topologies/gastown" {
		t.Errorf("got %v, want [topologies/gastown]", got)
	}
}

func TestEffectiveRigTopologies_PluralOnly(t *testing.T) {
	rig := Rig{RigTopologies: []string{"topologies/a", "topologies/b"}}
	got := EffectiveRigTopologies(rig)
	if len(got) != 2 || got[0] != "topologies/a" || got[1] != "topologies/b" {
		t.Errorf("got %v, want [topologies/a topologies/b]", got)
	}
}

func TestEffectiveRigTopologies_Both(t *testing.T) {
	rig := Rig{
		Topology:      "topologies/singular",
		RigTopologies: []string{"topologies/a", "topologies/b"},
	}
	got := EffectiveRigTopologies(rig)
	if len(got) != 3 || got[0] != "topologies/singular" || got[1] != "topologies/a" || got[2] != "topologies/b" {
		t.Errorf("got %v, want [topologies/singular topologies/a topologies/b]", got)
	}
}

func TestEffectiveRigTopologies_Neither(t *testing.T) {
	rig := Rig{}
	got := EffectiveRigTopologies(rig)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- ExpandCityTopologies (plural) tests ---

func TestExpandCityTopologies_Multiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[[agents]]
name = "agent-a"
`)
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[[agents]]
name = "agent-b"
`)

	cfg := &City{
		Workspace: Workspace{CityTopologies: []string{
			"topologies/alpha", "topologies/beta",
		}},
		Agents: []Agent{{Name: "existing"}},
	}

	dirs, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopologies: %v", err)
	}

	// Should have 3 agents: agent-a, agent-b (from topologies), then existing.
	if len(cfg.Agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "agent-a" {
		t.Errorf("first agent = %q, want agent-a", cfg.Agents[0].Name)
	}
	if cfg.Agents[1].Name != "agent-b" {
		t.Errorf("second agent = %q, want agent-b", cfg.Agents[1].Name)
	}
	if cfg.Agents[2].Name != "existing" {
		t.Errorf("third agent = %q, want existing", cfg.Agents[2].Name)
	}

	// No formulas configured → empty list.
	if len(dirs) != 0 {
		t.Errorf("formula dirs = %v, want empty", dirs)
	}
}

func TestExpandCityTopologies_FormulaDirsStacked(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "agent-a"
`)
	writeFile(t, dir, "topologies/alpha/formulas/mol-a.toml", "test")
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "agent-b"
`)
	writeFile(t, dir, "topologies/beta/formulas/mol-b.toml", "test")

	cfg := &City{
		Workspace: Workspace{CityTopologies: []string{
			"topologies/alpha", "topologies/beta",
		}},
	}

	dirs, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopologies: %v", err)
	}

	if len(dirs) != 2 {
		t.Fatalf("formula dirs = %d, want 2", len(dirs))
	}
	if dirs[0] != filepath.Join(dir, "topologies/alpha/formulas") {
		t.Errorf("dirs[0] = %q, want alpha formulas", dirs[0])
	}
	if dirs[1] != filepath.Join(dir, "topologies/beta/formulas") {
		t.Errorf("dirs[1] = %q, want beta formulas", dirs[1])
	}
}

func TestExpandCityTopologies_Empty(t *testing.T) {
	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
	}

	dirs, err := ExpandCityTopologies(cfg, fsys.OSFS{}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("formula dirs = %v, want empty", dirs)
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("got %d agents, want 1 (unchanged)", len(cfg.Agents))
	}
}

func TestExpandCityTopologies_BackwardCompat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
schema = 1

[[agents]]
name = "mayor"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gt"},
	}

	dirs, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopologies: %v", err)
	}

	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "mayor" {
		t.Errorf("agents = %v, want [mayor]", cfg.Agents)
	}
	if len(dirs) != 0 {
		t.Errorf("formula dirs = %v, want empty (no formulas)", dirs)
	}
}

func TestExpandCityTopologies_ProvidersMerged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[providers.codex]
command = "codex"

[[agents]]
name = "agent-a"
`)
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[providers.gemini]
command = "gemini"

[providers.codex]
command = "codex-from-beta"

[[agents]]
name = "agent-b"
`)

	cfg := &City{
		Workspace: Workspace{CityTopologies: []string{
			"topologies/alpha", "topologies/beta",
		}},
		Providers: map[string]ProviderSpec{
			"claude": {Command: "claude"},
		},
	}

	_, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopologies: %v", err)
	}

	// codex from alpha (first wins).
	if cfg.Providers["codex"].Command != "codex" {
		t.Errorf("codex command = %q, want codex (first wins)", cfg.Providers["codex"].Command)
	}
	// gemini from beta.
	if _, ok := cfg.Providers["gemini"]; !ok {
		t.Error("gemini provider should be merged from beta")
	}
	// claude unchanged.
	if cfg.Providers["claude"].Command != "claude" {
		t.Error("existing claude provider should not be overwritten")
	}
}

// --- ExpandTopologies plural rig tests ---

func TestExpandTopologies_MultiplePerRig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[[agents]]
name = "worker-a"
`)
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[[agents]]
name = "worker-b"
`)

	cfg := &City{
		Agents: []Agent{{Name: "mayor"}},
		Rigs: []Rig{
			{
				Name: "hw",
				Path: "/hw",
				RigTopologies: []string{
					"topologies/alpha", "topologies/beta",
				},
			},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	if len(cfg.Agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("first agent = %q, want mayor", cfg.Agents[0].Name)
	}
	if cfg.Agents[1].Name != "worker-a" || cfg.Agents[1].Dir != "hw" {
		t.Errorf("second agent: name=%q dir=%q, want worker-a/hw", cfg.Agents[1].Name, cfg.Agents[1].Dir)
	}
	if cfg.Agents[2].Name != "worker-b" || cfg.Agents[2].Dir != "hw" {
		t.Errorf("third agent: name=%q dir=%q, want worker-b/hw", cfg.Agents[2].Name, cfg.Agents[2].Dir)
	}
}

func TestExpandTopologies_RigFormulaDirsMultiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "worker-a"
`)
	writeFile(t, dir, "topologies/alpha/formulas/mol-a.toml", "test")
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "worker-b"
`)
	writeFile(t, dir, "topologies/beta/formulas/mol-b.toml", "test")

	cfg := &City{
		Rigs: []Rig{
			{
				Name: "hw",
				Path: "/hw",
				RigTopologies: []string{
					"topologies/alpha", "topologies/beta",
				},
			},
		},
	}

	rigFormulaDirs := make(map[string][]string)
	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, rigFormulaDirs); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	got := rigFormulaDirs["hw"]
	if len(got) != 2 {
		t.Fatalf("rigFormulaDirs[hw] = %d entries, want 2", len(got))
	}
	if got[0] != filepath.Join(dir, "topologies/alpha/formulas") {
		t.Errorf("got[0] = %q, want alpha formulas", got[0])
	}
	if got[1] != filepath.Join(dir, "topologies/beta/formulas") {
		t.Errorf("got[1] = %q, want beta formulas", got[1])
	}
}

// --- FormulaLayers plural tests ---

func TestFormulaLayers_MultipleCityAndRigTopoFormulas(t *testing.T) {
	rigTopoFormulas := map[string][]string{
		"hw": {"/city/topologies/alpha/formulas", "/city/topologies/beta/formulas"},
	}
	rigs := []Rig{
		{Name: "hw", Path: "/home/user/hw", FormulasDir: "local-formulas"},
	}

	fl := ComputeFormulaLayers(
		[]string{"/city/topo-a/formulas", "/city/topo-b/formulas"},
		"/city/.gc/formulas",
		rigTopoFormulas, rigs, "/city")

	// City layers: 2 topo + 1 local = 3.
	if len(fl.City) != 3 {
		t.Fatalf("City layers = %d, want 3", len(fl.City))
	}
	if fl.City[0] != "/city/topo-a/formulas" {
		t.Errorf("City[0] = %q", fl.City[0])
	}
	if fl.City[1] != "/city/topo-b/formulas" {
		t.Errorf("City[1] = %q", fl.City[1])
	}
	if fl.City[2] != "/city/.gc/formulas" {
		t.Errorf("City[2] = %q", fl.City[2])
	}

	// Rig "hw": 3 city + 2 rig topo + 1 rig local = 6.
	hwLayers := fl.Rigs["hw"]
	if len(hwLayers) != 6 {
		t.Fatalf("hw layers = %d, want 6", len(hwLayers))
	}
	if hwLayers[3] != "/city/topologies/alpha/formulas" {
		t.Errorf("hw[3] = %q, want rig topo alpha", hwLayers[3])
	}
	if hwLayers[4] != "/city/topologies/beta/formulas" {
		t.Errorf("hw[4] = %q, want rig topo beta", hwLayers[4])
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
	fl := ComputeFormulaLayers([]string{"/city/topo/formulas"}, "/city/.gc/formulas", nil, nil, "/city")

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
	rigTopoFormulas := map[string][]string{
		"hw": {"/city/topologies/gt/formulas"},
	}
	rigs := []Rig{
		{Name: "hw", Path: "/home/user/hw", FormulasDir: "local-formulas"},
	}

	fl := ComputeFormulaLayers([]string{"/city/topo/formulas"}, "/city/.gc/formulas", rigTopoFormulas, rigs, "/city")

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

	fl := ComputeFormulaLayers(nil, "", nil, rigs, "/city")

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

	fl := ComputeFormulaLayers(nil, "", nil, rigs, "/city")

	if len(fl.City) != 0 {
		t.Errorf("City layers = %d, want 0", len(fl.City))
	}
	// Rig with no formula sources should not appear in map.
	if _, ok := fl.Rigs["hw"]; ok {
		t.Error("hw should not appear in Rigs (no formula layers)")
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

	rigFormulaDirs := make(map[string][]string)
	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, rigFormulaDirs); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	want := filepath.Join(dir, "topologies/gt/formulas")
	if got := rigFormulaDirs["hw"]; len(got) != 1 || got[0] != want {
		t.Errorf("rigFormulaDirs[hw] = %v, want [%q]", got, want)
	}
}

func TestExpandTopologies_SourceDirSet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	wantDir := filepath.Join(dir, "topologies/gt")
	if cfg.Agents[0].SourceDir != wantDir {
		t.Errorf("SourceDir = %q, want %q", cfg.Agents[0].SourceDir, wantDir)
	}
}

func TestExpandTopologies_SessionSetupScriptAdjusted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
session_setup_script = "scripts/setup.sh"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// session_setup_script should be adjusted relative to topology dir → city root.
	want := "topologies/gt/scripts/setup.sh"
	if cfg.Agents[0].SessionSetupScript != want {
		t.Errorf("SessionSetupScript = %q, want %q", cfg.Agents[0].SessionSetupScript, want)
	}
}

func TestExpandCityTopology_SourceDirSet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "mayor"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gastown"},
	}

	_, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	wantDir := filepath.Join(dir, "topologies/gastown")
	if cfg.Agents[0].SourceDir != wantDir {
		t.Errorf("SourceDir = %q, want %q", cfg.Agents[0].SourceDir, wantDir)
	}
}

func TestExpandTopologies_OverlayDirAdjusted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/gt/topology.toml", `
[topology]
name = "gastown"
version = "1.0.0"
schema = 1

[[agents]]
name = "witness"
overlay_dir = "overlays/worker"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/hw", Topology: "topologies/gt"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// overlay_dir should be adjusted relative to topology dir → city root.
	want := "topologies/gt/overlays/worker"
	if cfg.Agents[0].OverlayDir != want {
		t.Errorf("OverlayDir = %q, want %q", cfg.Agents[0].OverlayDir, want)
	}
}

// --- CityAgents tests ---

func TestLoadTopologyCityAgents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/combined/topology.toml", `
[topology]
name = "combined"
schema = 1
city_agents = ["mayor", "deacon"]

[[agents]]
name = "mayor"

[[agents]]
name = "deacon"

[[agents]]
name = "witness"
`)

	agents, _, _, _, cityAgents, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/combined/topology.toml"),
		filepath.Join(dir, "topologies/combined"),
		dir, "", nil)
	if err != nil {
		t.Fatalf("loadTopology: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(agents))
	}
	if len(cityAgents) != 2 {
		t.Fatalf("got %d cityAgents, want 2", len(cityAgents))
	}
	if cityAgents[0] != "mayor" || cityAgents[1] != "deacon" {
		t.Errorf("cityAgents = %v, want [mayor deacon]", cityAgents)
	}
}

func TestLoadTopologyCityAgentsInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/bad/topology.toml", `
[topology]
name = "bad"
schema = 1
city_agents = ["mayor", "nonexistent"]

[[agents]]
name = "mayor"

[[agents]]
name = "witness"
`)

	_, _, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/bad/topology.toml"),
		filepath.Join(dir, "topologies/bad"),
		dir, "", nil)
	if err == nil {
		t.Fatal("expected error for city_agents with unknown agent name")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error = %v, want to contain 'nonexistent'", err)
	}
}

func TestExpandCityTopologyFilters(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/combined/topology.toml", `
[topology]
name = "combined"
schema = 1
city_agents = ["mayor", "deacon"]

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"

[[agents]]
name = "deacon"

[[agents]]
name = "witness"
prompt_template = "prompts/witness.md"

[[agents]]
name = "polecat"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/combined"},
	}

	_, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	// Should only have city agents (mayor, deacon).
	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	names := make(map[string]bool)
	for _, a := range cfg.Agents {
		names[a.Name] = true
		if a.Dir != "" {
			t.Errorf("city agent %q: dir = %q, want empty", a.Name, a.Dir)
		}
	}
	if !names["mayor"] || !names["deacon"] {
		t.Errorf("agents = %v, want mayor and deacon", names)
	}
	if names["witness"] || names["polecat"] {
		t.Error("rig agents should be filtered out of city topology expansion")
	}
}

func TestExpandTopologiesFilters(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "topologies/combined/topology.toml", `
[topology]
name = "combined"
schema = 1
city_agents = ["mayor", "deacon"]

[[agents]]
name = "mayor"

[[agents]]
name = "deacon"

[[agents]]
name = "witness"
prompt_template = "prompts/witness.md"

[[agents]]
name = "polecat"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "hw", Path: "/home/user/hw", Topology: "topologies/combined"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("ExpandTopologies: %v", err)
	}

	// Should only have rig agents (witness, polecat), not city agents.
	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	names := make(map[string]bool)
	for _, a := range cfg.Agents {
		names[a.Name] = true
		if a.Dir != "hw" {
			t.Errorf("rig agent %q: dir = %q, want %q", a.Name, a.Dir, "hw")
		}
	}
	if !names["witness"] || !names["polecat"] {
		t.Errorf("agents = %v, want witness and polecat", names)
	}
	if names["mayor"] || names["deacon"] {
		t.Error("city agents should be filtered out of rig topology expansion")
	}
}

func TestExpandCityTopologyNoCityAgents(t *testing.T) {
	// When city_agents is empty, all agents are city-scoped (backward compat).
	dir := t.TempDir()
	writeFile(t, dir, "topologies/simple/topology.toml", `
[topology]
name = "simple"
schema = 1

[[agents]]
name = "alpha"

[[agents]]
name = "beta"
`)

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/simple"},
	}

	_, err := ExpandCityTopology(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopology: %v", err)
	}

	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2 (all agents without city_agents filter)", len(cfg.Agents))
	}
}

func TestExpandTopologies_DuplicateAgentCollision(t *testing.T) {
	// Two rig topologies defining the same agent name should produce
	// a provenance-aware error naming both topology directories.
	dir := t.TempDir()
	writeFile(t, dir, "topologies/base/topology.toml", `
[topology]
name = "base"
schema = 1

[[agents]]
name = "worker"
`)
	writeFile(t, dir, "topologies/extras/topology.toml", `
[topology]
name = "extras"
schema = 1

[[agents]]
name = "worker"
`)

	cfg := &City{
		Rigs: []Rig{{
			Name:          "myrig",
			Path:          "/tmp/myrig",
			RigTopologies: []string{"topologies/base", "topologies/extras"},
		}},
	}

	err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil)
	if err == nil {
		t.Fatal("expected error for duplicate agent across rig topologies")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "duplicate agent") {
		t.Errorf("error should mention 'duplicate agent', got: %s", errStr)
	}
	if !strings.Contains(errStr, "myrig") {
		t.Errorf("error should mention rig name 'myrig', got: %s", errStr)
	}
	if !strings.Contains(errStr, "topologies/base") {
		t.Errorf("error should mention first topology dir, got: %s", errStr)
	}
	if !strings.Contains(errStr, "topologies/extras") {
		t.Errorf("error should mention second topology dir, got: %s", errStr)
	}
}

func TestExpandCityTopologies_DuplicateAgentCollision(t *testing.T) {
	// Two city topologies defining the same agent name should produce
	// a provenance-aware error.
	dir := t.TempDir()
	writeFile(t, dir, "topologies/alpha/topology.toml", `
[topology]
name = "alpha"
schema = 1

[[agents]]
name = "overseer"
`)
	writeFile(t, dir, "topologies/beta/topology.toml", `
[topology]
name = "beta"
schema = 1

[[agents]]
name = "overseer"
`)

	cfg := &City{
		Workspace: Workspace{
			CityTopologies: []string{"topologies/alpha", "topologies/beta"},
		},
	}

	_, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err == nil {
		t.Fatal("expected error for duplicate agent across city topologies")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "duplicate agent") {
		t.Errorf("error should mention 'duplicate agent', got: %s", errStr)
	}
	if !strings.Contains(errStr, "city") {
		t.Errorf("error should mention 'city' scope, got: %s", errStr)
	}
	if !strings.Contains(errStr, "topologies/alpha") {
		t.Errorf("error should mention first topology dir, got: %s", errStr)
	}
	if !strings.Contains(errStr, "topologies/beta") {
		t.Errorf("error should mention second topology dir, got: %s", errStr)
	}
}

func TestExpandTopologies_DifferentNamesNoCollision(t *testing.T) {
	// Two rig topologies with different agent names should compose without error.
	dir := t.TempDir()
	writeFile(t, dir, "topologies/base/topology.toml", `
[topology]
name = "base"
schema = 1

[[agents]]
name = "worker"
`)
	writeFile(t, dir, "topologies/extras/topology.toml", `
[topology]
name = "extras"
schema = 1

[[agents]]
name = "reviewer"
`)

	cfg := &City{
		Rigs: []Rig{{
			Name:          "myrig",
			Path:          "/tmp/myrig",
			RigTopologies: []string{"topologies/base", "topologies/extras"},
		}},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("unexpected error for different-named agents: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
}

func TestExpandTopologies_SameTopologyDifferentRigsNoCollision(t *testing.T) {
	// Same topology applied to two different rigs should not collide
	// (different dir scope).
	dir := t.TempDir()
	writeFile(t, dir, "topologies/shared/topology.toml", `
[topology]
name = "shared"
schema = 1

[[agents]]
name = "worker"
`)

	cfg := &City{
		Rigs: []Rig{
			{Name: "rig-a", Path: "/tmp/a", Topology: "topologies/shared"},
			{Name: "rig-b", Path: "/tmp/b", Topology: "topologies/shared"},
		},
	}

	if err := ExpandTopologies(cfg, fsys.OSFS{}, dir, nil); err != nil {
		t.Fatalf("unexpected error for same topology on different rigs: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Dir != "rig-a" || cfg.Agents[1].Dir != "rig-b" {
		t.Errorf("agents should have different dirs: %q, %q", cfg.Agents[0].Dir, cfg.Agents[1].Dir)
	}
}

// --- Topology includes tests ---

func TestTopologyIncludes(t *testing.T) {
	dir := t.TempDir()

	// maintenance topology: defines "dog" agent.
	writeFile(t, dir, "topologies/maintenance/topology.toml", `
[topology]
name = "maintenance"
schema = 1

[[agents]]
name = "dog"
`)

	// gastown topology: includes maintenance, defines "mayor".
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
schema = 1
includes = ["../maintenance"]

[[agents]]
name = "mayor"
`)

	agents, _, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/gastown/topology.toml"),
		filepath.Join(dir, "topologies/gastown"),
		dir, "", nil)
	if err != nil {
		t.Fatalf("loadTopology: %v", err)
	}

	// Should have 2 agents: dog (from include, first) then mayor (parent).
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(agents))
	}
	if agents[0].Name != "dog" {
		t.Errorf("agents[0].Name = %q, want dog (from include)", agents[0].Name)
	}
	if agents[1].Name != "mayor" {
		t.Errorf("agents[1].Name = %q, want mayor (from parent)", agents[1].Name)
	}
}

func TestTopologyIncludesCityAgents(t *testing.T) {
	dir := t.TempDir()

	// maintenance topology: defines "dog" with city_agents.
	writeFile(t, dir, "topologies/maintenance/topology.toml", `
[topology]
name = "maintenance"
schema = 1
city_agents = ["dog"]

[[agents]]
name = "dog"
`)

	// gastown topology: includes maintenance, own city_agents.
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
schema = 1
includes = ["../maintenance"]
city_agents = ["mayor"]

[[agents]]
name = "mayor"
`)

	_, _, _, _, cityAgents, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/gastown/topology.toml"),
		filepath.Join(dir, "topologies/gastown"),
		dir, "", nil)
	if err != nil {
		t.Fatalf("loadTopology: %v", err)
	}

	// Union of city_agents: dog (from include, sorted first) + mayor (parent).
	if len(cityAgents) != 2 {
		t.Fatalf("got %d cityAgents, want 2: %v", len(cityAgents), cityAgents)
	}
	// Included-only entries come first (sorted), then parent in declaration order.
	caSet := setFromSlice(cityAgents)
	if !caSet["dog"] || !caSet["mayor"] {
		t.Errorf("cityAgents = %v, want [dog, mayor]", cityAgents)
	}
}

func TestTopologyIncludesFormulas(t *testing.T) {
	dir := t.TempDir()

	// maintenance topology with formulas.
	writeFile(t, dir, "topologies/maintenance/topology.toml", `
[topology]
name = "maintenance"
schema = 1

[formulas]
dir = "formulas"

[[agents]]
name = "dog"
`)
	writeFile(t, dir, "topologies/maintenance/formulas/.keep", "")

	// gastown topology with formulas, includes maintenance.
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
schema = 1
includes = ["../maintenance"]

[formulas]
dir = "formulas"

[[agents]]
name = "mayor"
`)
	writeFile(t, dir, "topologies/gastown/formulas/.keep", "")

	_, _, formulaDirs, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/gastown/topology.toml"),
		filepath.Join(dir, "topologies/gastown"),
		dir, "", nil)
	if err != nil {
		t.Fatalf("loadTopology: %v", err)
	}

	// Should have 2 formula dirs: maintenance first, then gastown.
	if len(formulaDirs) != 2 {
		t.Fatalf("got %d formulaDirs, want 2: %v", len(formulaDirs), formulaDirs)
	}
	if !strings.Contains(formulaDirs[0], "maintenance") {
		t.Errorf("formulaDirs[0] = %q, want maintenance formulas", formulaDirs[0])
	}
	if !strings.Contains(formulaDirs[1], "gastown") {
		t.Errorf("formulaDirs[1] = %q, want gastown formulas", formulaDirs[1])
	}
}

func TestTopologyIncludesCycle(t *testing.T) {
	dir := t.TempDir()

	// A includes B, B includes A → cycle.
	writeFile(t, dir, "topologies/a/topology.toml", `
[topology]
name = "a"
schema = 1
includes = ["../b"]

[[agents]]
name = "alpha"
`)
	writeFile(t, dir, "topologies/b/topology.toml", `
[topology]
name = "b"
schema = 1
includes = ["../a"]

[[agents]]
name = "beta"
`)

	_, _, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/a/topology.toml"),
		filepath.Join(dir, "topologies/a"),
		dir, "", nil)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %v, want to contain 'cycle'", err)
	}
}

func TestTopologyIncludesNotFound(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "topologies/main/topology.toml", `
[topology]
name = "main"
schema = 1
includes = ["../nonexistent"]

[[agents]]
name = "alpha"
`)

	_, _, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/main/topology.toml"),
		filepath.Join(dir, "topologies/main"),
		dir, "", nil)
	if err == nil {
		t.Fatal("expected error for missing include")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error = %v, want to contain 'nonexistent'", err)
	}
}

func TestTopologyIncludesProviderMerge(t *testing.T) {
	dir := t.TempDir()

	// Included topology defines provider "claude".
	writeFile(t, dir, "topologies/base/topology.toml", `
[topology]
name = "base"
schema = 1

[providers.claude]
command = "base-claude"

[[agents]]
name = "worker"
`)

	// Parent topology also defines "claude" — parent wins.
	writeFile(t, dir, "topologies/main/topology.toml", `
[topology]
name = "main"
schema = 1
includes = ["../base"]

[providers.claude]
command = "main-claude"

[[agents]]
name = "boss"
`)

	_, providers, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(dir, "topologies/main/topology.toml"),
		filepath.Join(dir, "topologies/main"),
		dir, "", nil)
	if err != nil {
		t.Fatalf("loadTopology: %v", err)
	}

	if providers["claude"].Command != "main-claude" {
		t.Errorf("provider claude = %q, want main-claude (parent wins)", providers["claude"].Command)
	}
}

func TestExpandCityTopologiesWithIncludes(t *testing.T) {
	dir := t.TempDir()

	// maintenance topology.
	writeFile(t, dir, "topologies/maintenance/topology.toml", `
[topology]
name = "maintenance"
schema = 1
city_agents = ["dog"]

[formulas]
dir = "formulas"

[[agents]]
name = "dog"
`)
	writeFile(t, dir, "topologies/maintenance/formulas/.keep", "")

	// gastown topology includes maintenance.
	writeFile(t, dir, "topologies/gastown/topology.toml", `
[topology]
name = "gastown"
schema = 1
includes = ["../maintenance"]
city_agents = ["mayor"]

[formulas]
dir = "formulas"

[[agents]]
name = "mayor"

[[agents]]
name = "witness"
`)
	writeFile(t, dir, "topologies/gastown/formulas/.keep", "")

	cfg := &City{
		Workspace: Workspace{Topology: "topologies/gastown"},
	}
	formulaDirs, err := ExpandCityTopologies(cfg, fsys.OSFS{}, dir)
	if err != nil {
		t.Fatalf("ExpandCityTopologies: %v", err)
	}

	// city_agents union = [dog, mayor], so witness is filtered out.
	agentNames := make(map[string]bool)
	for _, a := range cfg.Agents {
		agentNames[a.Name] = true
	}
	if !agentNames["dog"] {
		t.Error("expected dog agent (from included maintenance)")
	}
	if !agentNames["mayor"] {
		t.Error("expected mayor agent (from gastown)")
	}
	if agentNames["witness"] {
		t.Error("witness should be filtered out (not in city_agents)")
	}

	// Formula dirs: maintenance then gastown.
	if len(formulaDirs) != 2 {
		t.Fatalf("got %d formulaDirs, want 2: %v", len(formulaDirs), formulaDirs)
	}
}

func TestTopologySharedDirsCollected(t *testing.T) {
	tmp := t.TempDir()

	// Create a topology with a prompts/shared/ directory.
	topoDir := filepath.Join(tmp, "topologies", "alpha")
	writeFile(t, topoDir, "topology.toml", `
[topology]
name = "alpha"
schema = 1

[[agents]]
name = "worker"
prompt_template = "prompts/worker.md.tmpl"
`)
	writeFile(t, filepath.Join(topoDir, "prompts"), "worker.md.tmpl", "Worker prompt")
	writeFile(t, filepath.Join(topoDir, "prompts", "shared"), "common.md.tmpl",
		`{{ define "common" }}shared content{{ end }}`)

	writeFile(t, tmp, "city.toml", `
[workspace]
name = "test"
topology = "topologies/alpha"
`)

	cfg, _, err := LoadWithIncludes(fsys.OSFS{}, filepath.Join(tmp, "city.toml"))
	if err != nil {
		t.Fatalf("LoadWithIncludes: %v", err)
	}

	if len(cfg.TopologySharedDirs) == 0 {
		t.Fatal("TopologySharedDirs is empty, want at least one entry")
	}

	found := false
	for _, d := range cfg.TopologySharedDirs {
		if strings.HasSuffix(d, filepath.Join("topologies", "alpha", "prompts", "shared")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TopologySharedDirs = %v, want entry ending with topologies/alpha/prompts/shared", cfg.TopologySharedDirs)
	}
}
