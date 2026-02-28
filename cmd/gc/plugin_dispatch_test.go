package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/plugins"
)

func TestPluginDispatcherNil(t *testing.T) {
	pd := buildPluginDispatcher(t.TempDir(), &config.City{}, noopRunner, events.Discard, &bytes.Buffer{})
	if pd != nil {
		t.Error("expected nil dispatcher for empty plugins")
	}
}

func TestBuildPluginDispatcherNoPlugins(t *testing.T) {
	// City with formula layers that exist but contain no plugins.
	dir := t.TempDir()
	cfg := &config.City{}
	pd := buildPluginDispatcher(dir, cfg, noopRunner, events.Discard, &bytes.Buffer{})
	if pd != nil {
		t.Error("expected nil dispatcher when no plugins exist")
	}
}

func TestPluginDispatchManualFiltered(t *testing.T) {
	pd := buildPluginDispatcherFromPlugins(
		[]plugins.Plugin{{Name: "manual-only", Gate: "manual", Formula: "noop"}},
		beads.NewMemStore(), nil, noopRunner,
	)
	if pd != nil {
		t.Error("expected nil dispatcher — manual plugins should be filtered out")
	}
}

func TestPluginDispatchCooldownDue(t *testing.T) {
	store := beads.NewMemStore()
	var labelArgs []string
	runner := func(_, name string, args ...string) ([]byte, error) {
		if name == "bd" && len(args) > 0 && args[0] == "update" {
			labelArgs = args
		}
		return []byte("ok\n"), nil
	}

	pp := []plugins.Plugin{{
		Name:     "test-plugin",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "test-formula",
		Pool:     "worker",
	}}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, runner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}

	// Verify labels include plugin-run and pool routing.
	found := map[string]bool{}
	for _, a := range labelArgs {
		found[a] = true
	}
	if !found["--label=plugin-run:test-plugin"] {
		t.Errorf("missing plugin-run label, got %v", labelArgs)
	}
	if !found["--label=pool:worker"] {
		t.Errorf("missing pool label, got %v", labelArgs)
	}
}

func TestPluginDispatchCooldownNotDue(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent plugin-run bead.
	_, err := store.Create(beads.Bead{
		Title:  "plugin run",
		Labels: []string{"plugin-run:test-plugin"},
	})
	if err != nil {
		t.Fatal(err)
	}

	pp := []plugins.Plugin{{
		Name:     "test-plugin",
		Gate:     "cooldown",
		Interval: "1h", // 1 hour — far in the future
		Formula:  "test-formula",
	}}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, noopRunner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0 (cooldown not elapsed)", dispatched)
	}
}

func TestPluginDispatchMultiple(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent run for plugin-b so only plugin-a is due.
	_, err := store.Create(beads.Bead{
		Title:  "recent run",
		Labels: []string{"plugin-run:plugin-b"},
	})
	if err != nil {
		t.Fatal(err)
	}

	pp := []plugins.Plugin{
		{Name: "plugin-a", Gate: "cooldown", Interval: "1m", Formula: "formula-a"},
		{Name: "plugin-b", Gate: "cooldown", Interval: "1h", Formula: "formula-b"},
	}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, noopRunner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1 (only plugin-a due)", dispatched)
	}
}

func TestPluginDispatchMolCookError(t *testing.T) {
	// Store that fails on MolCook.
	store := &failMolCookStore{}

	pp := []plugins.Plugin{{
		Name:     "fail-plugin",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "bad-formula",
	}}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, noopRunner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	// Should not crash — best-effort skip.
	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch should not error: %v", err)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0 (MolCook failed)", dispatched)
	}
}

// --- helpers ---

// noopRunner is a CommandRunner that always succeeds.
var noopRunner beads.CommandRunner = func(_, _ string, _ ...string) ([]byte, error) {
	return []byte("ok\n"), nil
}

// buildPluginDispatcherFromPlugins builds a dispatcher from pre-scanned plugins,
// bypassing the filesystem scan. Returns nil if no auto-dispatchable plugins.
func buildPluginDispatcherFromPlugins(pp []plugins.Plugin, store beads.Store, ep events.Provider, runner beads.CommandRunner) pluginDispatcher { //nolint:unparam // ep is nil in current tests but needed for event-gate tests
	var auto []plugins.Plugin
	for _, p := range pp {
		if p.Gate != "manual" {
			auto = append(auto, p)
		}
	}
	if len(auto) == 0 {
		return nil
	}
	return &memoryPluginDispatcher{
		pp:     auto,
		store:  store,
		ep:     ep,
		runner: runner,
	}
}

// failMolCookStore wraps MemStore but fails on MolCook.
type failMolCookStore struct {
	beads.MemStore
}

func (f *failMolCookStore) MolCook(formula, _ string, _ []string) (string, error) {
	return "", fmt.Errorf("mol cook failed: %s", formula)
}

// --- rig-scoped dispatch tests ---

func TestBuildPluginDispatcherWithRigs(t *testing.T) {
	// Build a config with rig formula layers that include plugins.
	rigDir := t.TempDir()
	// Create a plugin in the rig-exclusive layer.
	pluginDir := rigDir + "/plugins/rig-health"
	if err := mkdirAll(pluginDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, pluginDir+"/plugin.toml", `[plugin]
formula = "mol-rig-health"
gate = "cooldown"
interval = "5m"
pool = "polecat"
`)

	cfg := &config.City{
		FormulaLayers: config.FormulaLayers{
			City: []string{"/nonexistent/city-layer"}, // no city plugins
			Rigs: map[string][]string{
				"demo": {"/nonexistent/city-layer", rigDir},
			},
		},
	}

	var stderr bytes.Buffer
	pd := buildPluginDispatcher(t.TempDir(), cfg, noopRunner, events.Discard, &stderr)
	if pd == nil {
		t.Fatalf("expected non-nil dispatcher; stderr: %s", stderr.String())
	}

	mpd := pd.(*memoryPluginDispatcher)
	if len(mpd.pp) != 1 {
		t.Fatalf("got %d plugins, want 1", len(mpd.pp))
	}
	if mpd.pp[0].Rig != "demo" {
		t.Errorf("plugin Rig = %q, want %q", mpd.pp[0].Rig, "demo")
	}
	if mpd.pp[0].Name != "rig-health" {
		t.Errorf("plugin Name = %q, want %q", mpd.pp[0].Name, "rig-health")
	}
}

func TestPluginDispatchRigScoped(t *testing.T) {
	store := beads.NewMemStore()
	var labelArgs []string
	runner := func(_, name string, args ...string) ([]byte, error) {
		if name == "bd" && len(args) > 0 && args[0] == "update" {
			labelArgs = args
		}
		return []byte("ok\n"), nil
	}

	pp := []plugins.Plugin{{
		Name:     "db-health",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "mol-db-health",
		Pool:     "polecat",
		Rig:      "demo-repo",
	}}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, runner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}

	found := map[string]bool{}
	for _, a := range labelArgs {
		found[a] = true
	}
	// Scoped label.
	if !found["--label=plugin-run:db-health:rig:demo-repo"] {
		t.Errorf("missing scoped plugin-run label, got %v", labelArgs)
	}
	// Auto-qualified pool.
	if !found["--label=pool:demo-repo/polecat"] {
		t.Errorf("missing qualified pool label, got %v", labelArgs)
	}
}

func TestPluginDispatchRigCooldownIndependent(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent run for rig-A's plugin (scoped name).
	_, err := store.Create(beads.Bead{
		Title:  "plugin run",
		Labels: []string{"plugin-run:db-health:rig:rig-a"},
	})
	if err != nil {
		t.Fatal(err)
	}

	pp := []plugins.Plugin{
		{Name: "db-health", Gate: "cooldown", Interval: "1h", Formula: "mol-db-health", Rig: "rig-a"},
		{Name: "db-health", Gate: "cooldown", Interval: "1h", Formula: "mol-db-health", Rig: "rig-b"},
	}
	pd := buildPluginDispatcherFromPlugins(pp, store, nil, noopRunner)
	if pd == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := pd.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// rig-a should NOT be due (recent run). rig-b should be due (never run).
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1 (only rig-b due)", dispatched)
	}
}

func TestRigExclusiveLayers(t *testing.T) {
	city := []string{"/city/topo", "/city/local"}
	rig := []string{"/city/topo", "/city/local", "/rig/topo", "/rig/local"}

	got := rigExclusiveLayers(rig, city)
	if len(got) != 2 {
		t.Fatalf("got %d layers, want 2", len(got))
	}
	if got[0] != "/rig/topo" || got[1] != "/rig/local" {
		t.Errorf("got %v, want [/rig/topo /rig/local]", got)
	}
}

func TestRigExclusiveLayersNoCityPrefix(t *testing.T) {
	// Rig shorter than city → no exclusive layers.
	got := rigExclusiveLayers([]string{"/x"}, []string{"/a", "/b"})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestQualifyPool(t *testing.T) {
	tests := []struct {
		pool, rig, want string
	}{
		{"polecat", "demo-repo", "demo-repo/polecat"},
		{"demo-repo/polecat", "demo-repo", "demo-repo/polecat"}, // already qualified
		{"dog", "", "dog"}, // city plugin
	}
	for _, tt := range tests {
		got := qualifyPool(tt.pool, tt.rig)
		if got != tt.want {
			t.Errorf("qualifyPool(%q, %q) = %q, want %q", tt.pool, tt.rig, got, tt.want)
		}
	}
}

// --- helpers ---

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
