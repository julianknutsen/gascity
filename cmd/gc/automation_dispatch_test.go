package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/automations"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
)

func TestAutomationDispatcherNil(t *testing.T) {
	ad := buildAutomationDispatcher(t.TempDir(), &config.City{}, noopRunner, events.Discard, &bytes.Buffer{})
	if ad != nil {
		t.Error("expected nil dispatcher for empty automations")
	}
}

func TestBuildAutomationDispatcherNoAutomations(t *testing.T) {
	// City with formula layers that exist but contain no automations.
	dir := t.TempDir()
	cfg := &config.City{}
	ad := buildAutomationDispatcher(dir, cfg, noopRunner, events.Discard, &bytes.Buffer{})
	if ad != nil {
		t.Error("expected nil dispatcher when no automations exist")
	}
}

func TestAutomationDispatchManualFiltered(t *testing.T) {
	ad := buildAutomationDispatcherFromList(
		[]automations.Automation{{Name: "manual-only", Gate: "manual", Formula: "noop"}},
		beads.NewMemStore(), nil, noopRunner,
	)
	if ad != nil {
		t.Error("expected nil dispatcher — manual automations should be filtered out")
	}
}

func TestAutomationDispatchCooldownDue(t *testing.T) {
	store := beads.NewMemStore()
	var labelArgs []string
	runner := func(_, name string, args ...string) ([]byte, error) {
		if name == "bd" && len(args) > 0 && args[0] == "update" {
			labelArgs = args
		}
		return []byte("ok\n"), nil
	}

	aa := []automations.Automation{{
		Name:     "test-automation",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "test-formula",
		Pool:     "worker",
	}}
	ad := buildAutomationDispatcherFromList(aa, store, nil, runner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}

	// Verify labels include automation-run and pool routing.
	found := map[string]bool{}
	for _, a := range labelArgs {
		found[a] = true
	}
	if !found["--label=automation-run:test-automation"] {
		t.Errorf("missing automation-run label, got %v", labelArgs)
	}
	if !found["--label=pool:worker"] {
		t.Errorf("missing pool label, got %v", labelArgs)
	}
}

func TestAutomationDispatchCooldownNotDue(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent automation-run bead.
	_, err := store.Create(beads.Bead{
		Title:  "automation run",
		Labels: []string{"automation-run:test-automation"},
	})
	if err != nil {
		t.Fatal(err)
	}

	aa := []automations.Automation{{
		Name:     "test-automation",
		Gate:     "cooldown",
		Interval: "1h", // 1 hour — far in the future
		Formula:  "test-formula",
	}}
	ad := buildAutomationDispatcherFromList(aa, store, nil, noopRunner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0 (cooldown not elapsed)", dispatched)
	}
}

func TestAutomationDispatchMultiple(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent run for automation-b so only automation-a is due.
	_, err := store.Create(beads.Bead{
		Title:  "recent run",
		Labels: []string{"automation-run:automation-b"},
	})
	if err != nil {
		t.Fatal(err)
	}

	aa := []automations.Automation{
		{Name: "automation-a", Gate: "cooldown", Interval: "1m", Formula: "formula-a"},
		{Name: "automation-b", Gate: "cooldown", Interval: "1h", Formula: "formula-b"},
	}
	ad := buildAutomationDispatcherFromList(aa, store, nil, noopRunner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1 (only automation-a due)", dispatched)
	}
}

func TestAutomationDispatchMolCookError(t *testing.T) {
	// Store that fails on MolCook.
	store := &failMolCookStore{}

	aa := []automations.Automation{{
		Name:     "fail-automation",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "bad-formula",
	}}
	ad := buildAutomationDispatcherFromList(aa, store, nil, noopRunner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	// Should not crash — best-effort skip.
	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
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

// buildAutomationDispatcherFromList builds a dispatcher from pre-scanned automations,
// bypassing the filesystem scan. Returns nil if no auto-dispatchable automations.
func buildAutomationDispatcherFromList(aa []automations.Automation, store beads.Store, ep events.Provider, runner beads.CommandRunner) automationDispatcher { //nolint:unparam // ep is nil in current tests but needed for event-gate tests
	var auto []automations.Automation
	for _, a := range aa {
		if a.Gate != "manual" {
			auto = append(auto, a)
		}
	}
	if len(auto) == 0 {
		return nil
	}
	return &memoryAutomationDispatcher{
		aa:     auto,
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

func TestBuildAutomationDispatcherWithRigs(t *testing.T) {
	// Build a config with rig formula layers that include automations.
	rigDir := t.TempDir()
	// Create an automation in the rig-exclusive layer.
	automationDir := rigDir + "/automations/rig-health"
	if err := mkdirAll(automationDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, automationDir+"/automation.toml", `[automation]
formula = "mol-rig-health"
gate = "cooldown"
interval = "5m"
pool = "polecat"
`)

	cfg := &config.City{
		FormulaLayers: config.FormulaLayers{
			City: []string{"/nonexistent/city-layer"}, // no city automations
			Rigs: map[string][]string{
				"demo": {"/nonexistent/city-layer", rigDir},
			},
		},
	}

	var stderr bytes.Buffer
	ad := buildAutomationDispatcher(t.TempDir(), cfg, noopRunner, events.Discard, &stderr)
	if ad == nil {
		t.Fatalf("expected non-nil dispatcher; stderr: %s", stderr.String())
	}

	mad := ad.(*memoryAutomationDispatcher)
	if len(mad.aa) != 1 {
		t.Fatalf("got %d automations, want 1", len(mad.aa))
	}
	if mad.aa[0].Rig != "demo" {
		t.Errorf("automation Rig = %q, want %q", mad.aa[0].Rig, "demo")
	}
	if mad.aa[0].Name != "rig-health" {
		t.Errorf("automation Name = %q, want %q", mad.aa[0].Name, "rig-health")
	}
}

func TestAutomationDispatchRigScoped(t *testing.T) {
	store := beads.NewMemStore()
	var labelArgs []string
	runner := func(_, name string, args ...string) ([]byte, error) {
		if name == "bd" && len(args) > 0 && args[0] == "update" {
			labelArgs = args
		}
		return []byte("ok\n"), nil
	}

	aa := []automations.Automation{{
		Name:     "db-health",
		Gate:     "cooldown",
		Interval: "1m",
		Formula:  "mol-db-health",
		Pool:     "polecat",
		Rig:      "demo-repo",
	}}
	ad := buildAutomationDispatcherFromList(aa, store, nil, runner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
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
	if !found["--label=automation-run:db-health:rig:demo-repo"] {
		t.Errorf("missing scoped automation-run label, got %v", labelArgs)
	}
	// Auto-qualified pool.
	if !found["--label=pool:demo-repo/polecat"] {
		t.Errorf("missing qualified pool label, got %v", labelArgs)
	}
}

func TestAutomationDispatchRigCooldownIndependent(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent run for rig-A's automation (scoped name).
	_, err := store.Create(beads.Bead{
		Title:  "automation run",
		Labels: []string{"automation-run:db-health:rig:rig-a"},
	})
	if err != nil {
		t.Fatal(err)
	}

	aa := []automations.Automation{
		{Name: "db-health", Gate: "cooldown", Interval: "1h", Formula: "mol-db-health", Rig: "rig-a"},
		{Name: "db-health", Gate: "cooldown", Interval: "1h", Formula: "mol-db-health", Rig: "rig-b"},
	}
	ad := buildAutomationDispatcherFromList(aa, store, nil, noopRunner)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	dispatched, err := ad.dispatch(t.TempDir(), time.Now())
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
		{"dog", "", "dog"}, // city automation
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
