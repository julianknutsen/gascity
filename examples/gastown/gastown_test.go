// Package gastown_test validates the Gas Town example configuration.
//
// This test ensures the example stays valid as the SDK evolves:
// city.toml parses and validates, all formulas parse, and all
// prompt template files referenced by agents exist on disk.
package gastown_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/formula"
	"github.com/steveyegge/gascity/internal/fsys"
)

func exampleDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// loadExpanded loads city.toml with full topology expansion.
func loadExpanded(t *testing.T) *config.City {
	t.Helper()
	dir := exampleDir()
	cfg, _, err := config.LoadWithIncludes(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.LoadWithIncludes: %v", err)
	}
	return cfg
}

func TestCityTomlParses(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Workspace.Name != "gastown" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "gastown")
	}
	if cfg.Workspace.Topology != "topologies/gastown" {
		t.Errorf("Workspace.Topology = %q, want %q", cfg.Workspace.Topology, "topologies/gastown")
	}
}

func TestCityTomlValidates(t *testing.T) {
	cfg := loadExpanded(t)
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		t.Errorf("ValidateAgents: %v", err)
	}
}

func TestPromptFilesExist(t *testing.T) {
	dir := exampleDir()
	cfg := loadExpanded(t)
	for _, a := range cfg.Agents {
		if a.PromptTemplate == "" {
			continue
		}
		path := filepath.Join(dir, a.PromptTemplate)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("agent %q: prompt_template %q: %v", a.Name, a.PromptTemplate, err)
		}
	}
}

func TestAllFormulasParseAndValidate(t *testing.T) {
	dir := exampleDir()
	formulaDir := filepath.Join(dir, "topologies", "gastown", "formulas")

	entries, err := os.ReadDir(formulaDir)
	if err != nil {
		t.Fatalf("reading formulas dir: %v", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".formula.toml") {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(formulaDir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}
			f, err := formula.Parse(data)
			if err != nil {
				t.Fatalf("Parse(%s): %v", e.Name(), err)
			}
			if err := formula.Validate(f); err != nil {
				t.Errorf("Validate(%s): %v", e.Name(), err)
			}
		})
	}

	if count != 10 {
		t.Errorf("found %d formula files, want 10", count)
	}
}

func TestAllPromptTemplatesExist(t *testing.T) {
	dir := exampleDir()
	promptDir := filepath.Join(dir, "topologies", "gastown", "prompts")

	entries, err := os.ReadDir(promptDir)
	if err != nil {
		t.Fatalf("reading prompts dir: %v", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md.tmpl") {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(promptDir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}
			if len(data) == 0 {
				t.Errorf("%s is empty", e.Name())
			}
		})
	}

	if count != 8 {
		t.Errorf("found %d prompt template files, want 8", count)
	}
}

func TestAgentNudgeField(t *testing.T) {
	cfg := loadExpanded(t)

	// Verify nudge is populated for agents that have it.
	nudgeCounts := 0
	for _, a := range cfg.Agents {
		if a.Nudge != "" {
			nudgeCounts++
		}
	}
	if nudgeCounts == 0 {
		t.Error("no agents have nudge configured")
	}
}

func TestFormulasDir(t *testing.T) {
	cfg := loadExpanded(t)
	// Formulas come from the topology, not from city.toml directly.
	// FormulaLayers.City should have the topology formulas dir.
	if len(cfg.FormulaLayers.City) == 0 {
		t.Fatal("FormulaLayers.City is empty, want topology formulas layer")
	}
	// The topology formulas dir should end with "topologies/gastown/formulas".
	found := false
	for _, d := range cfg.FormulaLayers.City {
		if strings.HasSuffix(d, filepath.Join("topologies", "gastown", "formulas")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FormulaLayers.City = %v, want entry ending with topologies/gastown/formulas", cfg.FormulaLayers.City)
	}
}

func TestDaemonConfig(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Daemon.PatrolInterval != "30s" {
		t.Errorf("Daemon.PatrolInterval = %q, want %q", cfg.Daemon.PatrolInterval, "30s")
	}
	if cfg.Daemon.MaxRestartsOrDefault() != 5 {
		t.Errorf("Daemon.MaxRestarts = %d, want 5", cfg.Daemon.MaxRestartsOrDefault())
	}
	if cfg.Daemon.RestartWindow != "1h" {
		t.Errorf("Daemon.RestartWindow = %q, want %q", cfg.Daemon.RestartWindow, "1h")
	}
	if cfg.Daemon.ShutdownTimeout != "5s" {
		t.Errorf("Daemon.ShutdownTimeout = %q, want %q", cfg.Daemon.ShutdownTimeout, "5s")
	}
}

// topologyFileConfig mirrors the topology.toml structure for test parsing.
type topologyFileConfig struct {
	Topology config.TopologyMeta `toml:"topology"`
	Agents   []config.Agent      `toml:"agents"`
}

func TestCombinedTopologyParses(t *testing.T) {
	dir := exampleDir()
	topoPath := filepath.Join(dir, "topologies", "gastown", "topology.toml")

	data, err := os.ReadFile(topoPath)
	if err != nil {
		t.Fatalf("reading topology.toml: %v", err)
	}

	var tc topologyFileConfig
	if _, err := toml.Decode(string(data), &tc); err != nil {
		t.Fatalf("parsing topology.toml: %v", err)
	}

	if tc.Topology.Name != "gastown" {
		t.Errorf("[topology] name = %q, want %q", tc.Topology.Name, "gastown")
	}
	if tc.Topology.Schema != 1 {
		t.Errorf("[topology] schema = %d, want 1", tc.Topology.Schema)
	}

	// Expect all 8 agents.
	want := map[string]bool{
		"mayor": false, "deacon": false, "boot": false, "dog": false,
		"witness": false, "refinery": false, "polecat": false,
	}
	for _, a := range tc.Agents {
		if _, ok := want[a.Name]; ok {
			want[a.Name] = true
		} else {
			t.Errorf("unexpected topology agent %q", a.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing topology agent %q", name)
		}
	}
	if len(tc.Agents) != 7 {
		t.Errorf("topology has %d agents, want 7", len(tc.Agents))
	}

	// Verify city_agents list.
	cityAgents := map[string]bool{
		"mayor": false, "deacon": false, "boot": false, "dog": false,
	}
	for _, ca := range tc.Topology.CityAgents {
		if _, ok := cityAgents[ca]; ok {
			cityAgents[ca] = true
		} else {
			t.Errorf("unexpected city_agent %q", ca)
		}
	}
	for name, found := range cityAgents {
		if !found {
			t.Errorf("missing city_agent %q", name)
		}
	}
}

func TestTopologyPromptFilesExist(t *testing.T) {
	dir := exampleDir()
	topoDir := filepath.Join(dir, "topologies", "gastown")
	topoPath := filepath.Join(topoDir, "topology.toml")

	data, err := os.ReadFile(topoPath)
	if err != nil {
		t.Fatalf("reading topology.toml: %v", err)
	}

	var tc topologyFileConfig
	if _, err := toml.Decode(string(data), &tc); err != nil {
		t.Fatalf("parsing topology.toml: %v", err)
	}

	for _, a := range tc.Agents {
		if a.PromptTemplate == "" {
			continue
		}
		// Paths in topology are relative to topology dir.
		path := filepath.Join(topoDir, a.PromptTemplate)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("agent %q: prompt_template %q resolves to %q: %v",
				a.Name, a.PromptTemplate, path, err)
		}
	}
}

func TestCityAgentsFilter(t *testing.T) {
	// Verify config.LoadWithIncludes with the combined topology produces
	// only city-scoped agents when no rigs are registered.
	cfg := loadExpanded(t)

	cityAgents := map[string]bool{"mayor": true, "deacon": true, "boot": true, "dog": true}
	for _, a := range cfg.Agents {
		if !cityAgents[a.Name] {
			t.Errorf("unexpected agent %q — should be filtered out without rigs", a.Name)
		}
		if a.Dir != "" {
			t.Errorf("city agent %q: dir = %q, want empty", a.Name, a.Dir)
		}
	}
	if len(cfg.Agents) != 4 {
		t.Errorf("got %d agents, want 4 city-scoped agents", len(cfg.Agents))
	}
}
