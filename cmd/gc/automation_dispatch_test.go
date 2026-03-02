package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
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

	ad.dispatch(context.Background(), t.TempDir(), time.Now())

	// Wait briefly for goroutine to complete.
	time.Sleep(50 * time.Millisecond)

	// Verify tracking bead was created.
	all, _ := store.List()
	if len(all) == 0 {
		t.Fatal("expected tracking bead to be created")
	}
	found := false
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "automation-run:test-automation" {
				found = true
			}
		}
	}
	if !found {
		t.Error("tracking bead missing automation-run:test-automation label")
	}

	// Verify wisp was labeled with pool routing.
	foundPool := false
	for _, a := range labelArgs {
		if a == "--add-label=pool:worker" {
			foundPool = true
		}
	}
	if !foundPool {
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

	ad.dispatch(context.Background(), t.TempDir(), time.Now())

	// Wait briefly.
	time.Sleep(50 * time.Millisecond)

	// Should still have only the seed bead.
	all, _ := store.List()
	if len(all) != 1 {
		t.Errorf("expected 1 bead (seed only), got %d", len(all))
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

	ad.dispatch(context.Background(), t.TempDir(), time.Now())

	// Wait briefly for goroutine.
	time.Sleep(50 * time.Millisecond)

	// Should have the seed bead + 1 tracking bead for automation-a.
	all, _ := store.List()
	trackingCount := 0
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "automation-run:automation-a" {
				trackingCount++
			}
		}
	}
	if trackingCount != 1 {
		t.Errorf("expected 1 tracking bead for automation-a, got %d", trackingCount)
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
	ad.dispatch(context.Background(), t.TempDir(), time.Now())

	// Wait briefly for goroutine.
	time.Sleep(50 * time.Millisecond)
}

// --- exec automation dispatch tests ---

func TestAutomationDispatchExecDue(t *testing.T) {
	store := beads.NewMemStore()
	var rec memRecorder

	ran := false
	fakeExec := func(_ context.Context, _, _ string, _ []string) ([]byte, error) {
		ran = true
		return []byte("ok\n"), nil
	}

	aa := []automations.Automation{{
		Name:     "wasteland-poll",
		Gate:     "cooldown",
		Interval: "2m",
		Exec:     "$AUTOMATION_DIR/scripts/poll.sh",
		Source:   "/city/formulas/automations/wasteland-poll/automation.toml",
	}}
	ad := buildAutomationDispatcherFromListExec(aa, store, nil, noopRunner, fakeExec, &rec)
	if ad == nil {
		t.Fatal("expected non-nil dispatcher")
	}

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(100 * time.Millisecond)

	if !ran {
		t.Error("exec runner was not called")
	}

	// Check tracking bead exists with exec label.
	all, _ := store.List()
	found := false
	hasExec := false
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "automation-run:wasteland-poll" {
				found = true
			}
			if l == "exec" {
				hasExec = true
			}
		}
	}
	if !found {
		t.Error("tracking bead missing automation-run label")
	}
	if !hasExec {
		t.Error("tracking bead missing exec label")
	}

	// Check events.
	if !rec.hasType(events.AutomationFired) {
		t.Error("missing automation.fired event")
	}
	if !rec.hasType(events.AutomationCompleted) {
		t.Error("missing automation.completed event")
	}
}

func TestAutomationDispatchExecFailure(t *testing.T) {
	store := beads.NewMemStore()
	var rec memRecorder
	var stderr bytes.Buffer

	fakeExec := func(_ context.Context, _, _ string, _ []string) ([]byte, error) {
		return []byte("error output\n"), fmt.Errorf("exit status 1")
	}

	aa := []automations.Automation{{
		Name:     "fail-exec",
		Gate:     "cooldown",
		Interval: "2m",
		Exec:     "scripts/fail.sh",
	}}
	ad := buildAutomationDispatcherFromListExec(aa, store, nil, noopRunner, fakeExec, &rec)
	mad := ad.(*memoryAutomationDispatcher)
	mad.stderr = &stderr

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(100 * time.Millisecond)

	// Check tracking bead has exec-failed label.
	all, _ := store.List()
	hasFailed := false
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "exec-failed" {
				hasFailed = true
			}
		}
	}
	if !hasFailed {
		t.Error("tracking bead missing exec-failed label")
	}

	// Check automation.failed event.
	if !rec.hasType(events.AutomationFailed) {
		t.Error("missing automation.failed event")
	}
}

func TestAutomationDispatchExecCooldown(t *testing.T) {
	store := beads.NewMemStore()

	// Seed a recent exec run.
	_, err := store.Create(beads.Bead{
		Title:  "automation:wasteland-poll",
		Labels: []string{"automation-run:wasteland-poll"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ran := false
	fakeExec := func(_ context.Context, _, _ string, _ []string) ([]byte, error) {
		ran = true
		return nil, nil
	}

	aa := []automations.Automation{{
		Name:     "wasteland-poll",
		Gate:     "cooldown",
		Interval: "1h",
		Exec:     "scripts/poll.sh",
	}}
	ad := buildAutomationDispatcherFromListExec(aa, store, nil, noopRunner, fakeExec, nil)

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(50 * time.Millisecond)

	if ran {
		t.Error("exec should not have run — cooldown not elapsed")
	}
}

func TestAutomationDispatchExecAutomationDir(t *testing.T) {
	store := beads.NewMemStore()
	var gotEnv []string

	fakeExec := func(_ context.Context, _, _ string, env []string) ([]byte, error) {
		gotEnv = env
		return nil, nil
	}

	aa := []automations.Automation{{
		Name:     "poll",
		Gate:     "cooldown",
		Interval: "1m",
		Exec:     "$AUTOMATION_DIR/scripts/poll.sh",
		Source:   "/city/formulas/automations/poll/automation.toml",
	}}
	ad := buildAutomationDispatcherFromListExec(aa, store, nil, noopRunner, fakeExec, nil)

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(100 * time.Millisecond)

	foundDir := false
	for _, e := range gotEnv {
		if e == "AUTOMATION_DIR=/city/formulas/automations/poll" {
			foundDir = true
		}
	}
	if !foundDir {
		t.Errorf("AUTOMATION_DIR not set correctly, got env: %v", gotEnv)
	}
}

func TestAutomationDispatchExecTimeout(t *testing.T) {
	store := beads.NewMemStore()
	var rec memRecorder

	fakeExec := func(ctx context.Context, _, _ string, _ []string) ([]byte, error) {
		// Simulate a command that blocks until context is canceled.
		<-ctx.Done()
		return nil, ctx.Err()
	}

	aa := []automations.Automation{{
		Name:     "slow-exec",
		Gate:     "cooldown",
		Interval: "1m",
		Exec:     "scripts/slow.sh",
		Timeout:  "100ms",
	}}
	ad := buildAutomationDispatcherFromListExec(aa, store, nil, noopRunner, fakeExec, &rec)

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(300 * time.Millisecond)

	// Should have failed due to timeout.
	if !rec.hasType(events.AutomationFailed) {
		t.Error("missing automation.failed event after timeout")
	}
}

func TestEffectiveTimeout(t *testing.T) {
	tests := []struct {
		name       string
		a          automations.Automation
		maxTimeout time.Duration
		want       time.Duration
	}{
		{"exec default", automations.Automation{Exec: "x.sh"}, 0, 60 * time.Second},
		{"formula default", automations.Automation{Formula: "mol-x"}, 0, 30 * time.Second},
		{"custom timeout", automations.Automation{Exec: "x.sh", Timeout: "90s"}, 0, 90 * time.Second},
		{"capped by max", automations.Automation{Exec: "x.sh", Timeout: "120s"}, 60 * time.Second, 60 * time.Second},
		{"not capped under max", automations.Automation{Exec: "x.sh", Timeout: "30s"}, 60 * time.Second, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveTimeout(tt.a, tt.maxTimeout)
			if got != tt.want {
				t.Errorf("effectiveTimeout() = %v, want %v", got, tt.want)
			}
		})
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
	return buildAutomationDispatcherFromListExec(aa, store, ep, runner, nil, nil)
}

// buildAutomationDispatcherFromListExec builds a dispatcher with exec runner support.
func buildAutomationDispatcherFromListExec(aa []automations.Automation, store beads.Store, ep events.Provider, runner beads.CommandRunner, execRun ExecRunner, rec events.Recorder) automationDispatcher {
	var auto []automations.Automation
	for _, a := range aa {
		if a.Gate != "manual" {
			auto = append(auto, a)
		}
	}
	if len(auto) == 0 {
		return nil
	}
	if rec == nil {
		rec = events.Discard
	}
	if execRun == nil {
		execRun = shellExecRunner
	}
	return &memoryAutomationDispatcher{
		aa:      auto,
		store:   store,
		ep:      ep,
		runner:  runner,
		execRun: execRun,
		rec:     rec,
		stderr:  &bytes.Buffer{},
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

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(50 * time.Millisecond)

	found := map[string]bool{}
	for _, a := range labelArgs {
		found[a] = true
	}
	// Scoped label.
	if !found["--add-label=automation-run:db-health:rig:demo-repo"] {
		t.Errorf("missing scoped automation-run label, got %v", labelArgs)
	}
	// Auto-qualified pool.
	if !found["--add-label=pool:demo-repo/polecat"] {
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

	ad.dispatch(context.Background(), t.TempDir(), time.Now())
	time.Sleep(50 * time.Millisecond)

	// rig-b should have a tracking bead, rig-a should not.
	all, _ := store.List()
	rigBTracked := false
	rigATracked := false
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "automation-run:db-health:rig:rig-b" {
				rigBTracked = true
			}
			// Check that no NEW bead was created for rig-a (only the seed).
			// The seed bead is the only one with rig-a label.
		}
	}
	if !rigBTracked {
		t.Error("missing tracking bead for rig-b")
	}

	// Count rig-a beads — should be exactly 1 (the seed).
	rigACount := 0
	for _, b := range all {
		for _, l := range b.Labels {
			if l == "automation-run:db-health:rig:rig-a" {
				rigACount++
			}
		}
	}
	if rigACount != 1 {
		t.Errorf("rig-a bead count = %d, want 1 (seed only)", rigACount)
	}
	_ = rigATracked
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

// --- city topology layer tests ---

func TestBuildAutomationDispatcherCityTopologyLayers(t *testing.T) {
	// Simulate system formulas + topology formulas as two city layers.
	sysDir := t.TempDir()
	topoDir := t.TempDir()

	// System dir: beads-health automation.
	sysAutoDir := sysDir + "/automations/beads-health"
	if err := mkdirAll(sysAutoDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sysAutoDir+"/automation.toml", `[automation]
exec = "scripts/beads-health.sh"
gate = "cooldown"
interval = "30s"
`)

	// Topology dir: wasteland-poll automation.
	topoAutoDir := topoDir + "/automations/wasteland-poll"
	if err := mkdirAll(topoAutoDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, topoAutoDir+"/automation.toml", `[automation]
exec = "scripts/wasteland-poll.sh"
gate = "cooldown"
interval = "2m"
`)

	cfg := &config.City{
		FormulaLayers: config.FormulaLayers{
			City: []string{sysDir, topoDir},
		},
	}

	var stderr bytes.Buffer
	ad := buildAutomationDispatcher(t.TempDir(), cfg, noopRunner, events.Discard, &stderr)
	if ad == nil {
		t.Fatalf("expected non-nil dispatcher; stderr: %s", stderr.String())
	}

	mad := ad.(*memoryAutomationDispatcher)
	if len(mad.aa) != 2 {
		t.Fatalf("got %d automations, want 2; stderr: %s", len(mad.aa), stderr.String())
	}

	names := map[string]bool{}
	for _, a := range mad.aa {
		names[a.Name] = true
	}
	if !names["beads-health"] {
		t.Error("missing beads-health automation")
	}
	if !names["wasteland-poll"] {
		t.Error("missing wasteland-poll automation")
	}
}

func TestBuildAutomationDispatcherCityTopologyWithOverride(t *testing.T) {
	// Same two-layer setup, plus a config override on wasteland-poll interval.
	sysDir := t.TempDir()
	topoDir := t.TempDir()

	sysAutoDir := sysDir + "/automations/beads-health"
	if err := mkdirAll(sysAutoDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sysAutoDir+"/automation.toml", `[automation]
exec = "scripts/beads-health.sh"
gate = "cooldown"
interval = "30s"
`)

	topoAutoDir := topoDir + "/automations/wasteland-poll"
	if err := mkdirAll(topoAutoDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, topoAutoDir+"/automation.toml", `[automation]
exec = "scripts/wasteland-poll.sh"
gate = "cooldown"
interval = "2m"
`)

	tenSec := "10s"
	cfg := &config.City{
		FormulaLayers: config.FormulaLayers{
			City: []string{sysDir, topoDir},
		},
		Automations: config.AutomationsConfig{
			Overrides: []config.AutomationOverride{
				{Name: "wasteland-poll", Interval: &tenSec},
			},
		},
	}

	var stderr bytes.Buffer
	ad := buildAutomationDispatcher(t.TempDir(), cfg, noopRunner, events.Discard, &stderr)
	if ad == nil {
		t.Fatalf("expected non-nil dispatcher; stderr: %s", stderr.String())
	}

	mad := ad.(*memoryAutomationDispatcher)
	if len(mad.aa) != 2 {
		t.Fatalf("got %d automations, want 2", len(mad.aa))
	}

	// Verify wasteland-poll interval was overridden to 10s.
	for _, a := range mad.aa {
		if a.Name == "wasteland-poll" {
			if a.Interval != "10s" {
				t.Errorf("wasteland-poll interval = %q, want %q", a.Interval, "10s")
			}
			return
		}
	}
	t.Error("wasteland-poll not found in dispatcher automations")
}

func TestBuildAutomationDispatcherOverrideNotFoundNonFatal(t *testing.T) {
	// Single formula layer with beads-health only.
	// Override targets wasteland-poll (nonexistent).
	// Verify beads-health is still dispatched and stderr contains warning.
	sysDir := t.TempDir()

	sysAutoDir := sysDir + "/automations/beads-health"
	if err := mkdirAll(sysAutoDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sysAutoDir+"/automation.toml", `[automation]
exec = "scripts/beads-health.sh"
gate = "cooldown"
interval = "30s"
`)

	tenSec := "10s"
	cfg := &config.City{
		FormulaLayers: config.FormulaLayers{
			City: []string{sysDir},
		},
		Automations: config.AutomationsConfig{
			Overrides: []config.AutomationOverride{
				{Name: "wasteland-poll", Interval: &tenSec},
			},
		},
	}

	var stderr bytes.Buffer
	ad := buildAutomationDispatcher(t.TempDir(), cfg, noopRunner, events.Discard, &stderr)
	if ad == nil {
		t.Fatalf("expected non-nil dispatcher (beads-health should still be found); stderr: %s", stderr.String())
	}

	mad := ad.(*memoryAutomationDispatcher)
	if len(mad.aa) != 1 {
		t.Fatalf("got %d automations, want 1", len(mad.aa))
	}
	if mad.aa[0].Name != "beads-health" {
		t.Errorf("automation name = %q, want %q", mad.aa[0].Name, "beads-health")
	}

	// Verify stderr contains the "not found" warning from ApplyOverrides.
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected stderr to contain 'not found' warning, got: %s", stderr.String())
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

// memRecorder records events in memory for test assertions.
type memRecorder struct {
	events []events.Event
}

func (r *memRecorder) Record(e events.Event) {
	r.events = append(r.events, e)
}

func (r *memRecorder) hasType(typ string) bool {
	for _, e := range r.events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func (r *memRecorder) hasSubject(subject string) bool {
	for _, e := range r.events {
		if e.Subject == subject {
			return true
		}
	}
	return false
}

// Unused but keep for future event assertion tests.
var (
	_ = (*memRecorder).hasSubject
	_ = strings.Contains
)
