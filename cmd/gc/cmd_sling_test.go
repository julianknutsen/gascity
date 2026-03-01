package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

// errStore wraps a beads.Store and injects errors into MolCook/MolCookOn.
type errStore struct {
	beads.Store
	cookErr error
}

func (e *errStore) MolCook(formula, title string, vars []string) (string, error) {
	if e.cookErr != nil {
		return "", e.cookErr
	}
	return e.Store.MolCook(formula, title, vars)
}

func (e *errStore) MolCookOn(formula, beadID, title string, vars []string) (string, error) {
	if e.cookErr != nil {
		return "", e.cookErr
	}
	return e.Store.MolCookOn(formula, beadID, title, vars)
}

// selectiveErrStore wraps a beads.Store and injects errors for specific bead IDs
// in MolCookOn. Used to test partial cook failures in batch operations.
type selectiveErrStore struct {
	beads.Store
	failOnBeadIDs map[string]error
}

func (s *selectiveErrStore) MolCookOn(formula, beadID, title string, vars []string) (string, error) {
	if err, ok := s.failOnBeadIDs[beadID]; ok {
		return "", err
	}
	return s.Store.MolCookOn(formula, beadID, title, vars)
}

// fakeRunnerRule maps a command substring to a canned response.
type fakeRunnerRule struct {
	prefix string
	out    string
	err    error
}

// fakeRunner records the commands it receives and returns canned output.
// Rules are matched in order (first match wins), providing deterministic behavior.
type fakeRunner struct {
	calls []string
	rules []fakeRunnerRule
}

func newFakeRunner() *fakeRunner { return &fakeRunner{} }

// on registers a rule: if a command contains prefix, return (out, err).
func (r *fakeRunner) on(prefix, out string, err error) {
	r.rules = append(r.rules, fakeRunnerRule{prefix: prefix, out: out, err: err})
}

func (r *fakeRunner) run(command string) (string, error) {
	r.calls = append(r.calls, command)
	for _, rule := range r.rules {
		if strings.Contains(command, rule.prefix) {
			return rule.out, rule.err
		}
	}
	return "", nil
}

// testOpts constructs a slingOpts for testing with the given agent and bead.
func testOpts(a config.Agent, beadOrFormula string) slingOpts {
	return slingOpts{Target: a, BeadOrFormula: beadOrFormula}
}

// testDeps constructs a slingDeps for testing, returning the deps and
// stdout/stderr buffers for inspection.
func testDeps(cfg *config.City, sp session.Provider, runner SlingRunner) (slingDeps, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return slingDeps{
		CityName: "test-city",
		Cfg:      cfg,
		SP:       sp,
		Runner:   runner,
		Store:    beads.NewMemStore(),
		Stdout:   &stdout,
		Stderr:   &stderr,
	}, &stdout, &stderr
}

func TestBuildSlingCommand(t *testing.T) {
	tests := []struct {
		template string
		beadID   string
		want     string
	}{
		{"bd update {} --assignee=mayor", "BL-42", "bd update BL-42 --assignee=mayor"},
		{"bd update {} --label=pool:hw/polecat", "XY-7", "bd update XY-7 --label=pool:hw/polecat"},
		{"custom {} script {}", "ID-1", "custom ID-1 script ID-1"},
	}
	for _, tt := range tests {
		got := buildSlingCommand(tt.template, tt.beadID)
		if got != tt.want {
			t.Errorf("buildSlingCommand(%q, %q) = %q, want %q", tt.template, tt.beadID, got, tt.want)
		}
	}
}

func TestDoSlingBeadToFixedAgent(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	want := "bd update BL-42 --assignee=mayor"
	if runner.calls[0] != want {
		t.Errorf("runner call = %q, want %q", runner.calls[0], want)
	}
	if !strings.Contains(stdout.String(), "Slung BL-42") {
		t.Errorf("stdout = %q, want to contain 'Slung BL-42'", stdout.String())
	}
}

func TestDoSlingBeadToPool(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "hello-world",
		Pool: &config.PoolConfig{Min: 1, Max: 3},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-7")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "bd update HW-7 --label=pool:hello-world/polecat"
	if runner.calls[0] != want {
		t.Errorf("runner call = %q, want %q", runner.calls[0], want)
	}
}

func TestDoSlingFormulaToAgent(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "code-review")
	opts.IsFormula = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCook goes through the store, only the routing call goes through runner.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	// The MemStore generates IDs like "gc-1".
	wantSling := "bd update gc-1 --assignee=mayor"
	if runner.calls[0] != wantSling {
		t.Errorf("runner call = %q, want %q", runner.calls[0], wantSling)
	}
	if !strings.Contains(stdout.String(), "formula") && !strings.Contains(stdout.String(), "wisp root gc-1") {
		t.Errorf("stdout = %q, want mention of formula/wisp", stdout.String())
	}
}

func TestDoSlingFormulaWithTitle(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "code-review")
	opts.IsFormula = true
	opts.Title = "my-review"
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCook goes through the store; verify the bead was created with the title.
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.Title != "my-review" {
		t.Errorf("bead title = %q, want %q", b.Title, "my-review")
	}
}

func TestDoSlingSuspendedAgentWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (still routes)", code)
	}
	if !strings.Contains(stderr.String(), "suspended") {
		t.Errorf("stderr = %q, want suspended warning", stderr.String())
	}
	// Bead should still be routed.
	if len(runner.calls) != 1 {
		t.Errorf("got %d runner calls, want 1 (bead routed despite suspension)", len(runner.calls))
	}
}

func TestDoSlingSuspendedAgentForce(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Force = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "suspended") {
		t.Errorf("--force should suppress warning; stderr = %q", stderr.String())
	}
}

func TestDoSlingPoolMaxZeroWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "rig",
		Pool: &config.PoolConfig{Min: 0, Max: 0},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (still routes)", code)
	}
	if !strings.Contains(stderr.String(), "max=0") {
		t.Errorf("stderr = %q, want max=0 warning", stderr.String())
	}
}

func TestDoSlingPoolMaxZeroForce(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "rig",
		Pool: &config.PoolConfig{Min: 0, Max: 0},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Force = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "max=0") {
		t.Errorf("--force should suppress warning; stderr = %q", stderr.String())
	}
}

func TestDoSlingRunnerError(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd update", "", fmt.Errorf("bd not found"))
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bd not found") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
}

func TestDoSlingFormulaInstantiationError(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	deps.Store = &errStore{Store: beads.NewMemStore(), cookErr: fmt.Errorf("formula not found")}
	opts := testOpts(a, "nonexistent")
	opts.IsFormula = true
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "formula not found") {
		t.Errorf("stderr = %q, want formula error", stderr.String())
	}
}

func TestDoSlingNudgeFixedAgent(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	_ = sp.Start("gc-test-city-mayor", session.Config{})
	sp.Calls = nil // clear start call
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Check that nudge was sent.
	var nudged bool
	for _, c := range sp.Calls {
		if c.Method == "Nudge" && c.Name == "gc-test-city-mayor" {
			nudged = true
		}
	}
	if !nudged {
		t.Errorf("expected nudge call for gc-test-city-mayor; calls: %+v", sp.Calls)
	}
	if !strings.Contains(stdout.String(), "Nudged mayor") {
		t.Errorf("stdout = %q, want nudge confirmation", stdout.String())
	}
}

func TestDoSlingNudgeNoSession(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	// Don't start the session — agent has no running session.
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (sling succeeds, nudge warns)", code)
	}
	if !strings.Contains(stderr.String(), "cannot nudge") {
		t.Errorf("stderr = %q, want 'cannot nudge' warning", stderr.String())
	}
}

func TestDoSlingNudgeSuspended(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	opts.Force = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "cannot nudge") {
		t.Errorf("stderr = %q, want 'cannot nudge: suspended' warning", stderr.String())
	}
}

func TestDoSlingNudgePoolMember(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	// Start pool instance 2 (instance 1 not running).
	_ = sp.Start("gc-test-city-hw--polecat-2", session.Config{})
	sp.Calls = nil
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "hw",
		Pool: &config.PoolConfig{Min: 1, Max: 3},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Should nudge pool instance 2 (first running one found).
	var nudged bool
	for _, c := range sp.Calls {
		if c.Method == "Nudge" {
			nudged = true
		}
	}
	if !nudged {
		t.Errorf("expected nudge call for pool member; calls: %+v", sp.Calls)
	}
}

func TestDoSlingNudgePoolNoMembers(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	// No pool instances running.
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "hw",
		Pool: &config.PoolConfig{Min: 1, Max: 3},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (sling succeeds, nudge warns)", code)
	}
	if !strings.Contains(stderr.String(), "no running pool members") {
		t.Errorf("stderr = %q, want 'no running pool members' warning", stderr.String())
	}
}

func TestDoSlingCustomSlingQuery(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name:       "worker",
		SlingQuery: "custom-dispatch {} --queue=priority",
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-99")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "custom-dispatch BL-99 --queue=priority"
	if runner.calls[0] != want {
		t.Errorf("runner call = %q, want %q", runner.calls[0], want)
	}
}

func TestTargetType(t *testing.T) {
	fixed := config.Agent{Name: "mayor"}
	if got := targetType(&fixed); got != "agent" {
		t.Errorf("targetType(fixed) = %q, want %q", got, "agent")
	}

	pool := config.Agent{Name: "polecat", Pool: &config.PoolConfig{Min: 1, Max: 3}}
	if got := targetType(&pool); got != "pool" {
		t.Errorf("targetType(pool) = %q, want %q", got, "pool")
	}
}

func TestNewSlingCmdArgs(t *testing.T) {
	cmd := newSlingCmd(&bytes.Buffer{}, &bytes.Buffer{})
	if cmd.Use != "sling [target] <bead-or-formula>" {
		t.Errorf("Use = %q", cmd.Use)
	}
	// Verify flags exist.
	for _, name := range []string{"formula", "nudge", "force", "title", "on", "no-formula"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag %q", name)
		}
	}
	// Verify -f shorthand for --formula.
	if f := cmd.Flags().ShorthandLookup("f"); f == nil || f.Name != "formula" {
		t.Error("missing -f shorthand for --formula")
	}
	// Verify -t shorthand for --title.
	if f := cmd.Flags().ShorthandLookup("t"); f == nil || f.Name != "title" {
		t.Error("missing -t shorthand for --title")
	}
}

// fakeQuerier implements BeadQuerier for testing pre-flight checks.
type fakeQuerier struct {
	bead beads.Bead
	err  error
}

func (q *fakeQuerier) Get(_ string) (beads.Bead, error) {
	return q.bead, q.err
}

// fakeChildQuerier implements BeadChildQuerier for testing batch dispatch.
type fakeChildQuerier struct {
	beadsByID   map[string]beads.Bead
	childrenOf  map[string][]beads.Bead
	getErr      error
	childrenErr error
}

func newFakeChildQuerier() *fakeChildQuerier {
	return &fakeChildQuerier{
		beadsByID:  make(map[string]beads.Bead),
		childrenOf: make(map[string][]beads.Bead),
	}
}

func (q *fakeChildQuerier) Get(id string) (beads.Bead, error) {
	if q.getErr != nil {
		return beads.Bead{}, q.getErr
	}
	b, ok := q.beadsByID[id]
	if !ok {
		return beads.Bead{}, beads.ErrNotFound
	}
	return b, nil
}

func (q *fakeChildQuerier) Children(parentID string) ([]beads.Bead, error) {
	if q.childrenErr != nil {
		return nil, q.childrenErr
	}
	return q.childrenOf[parentID], nil
}

func TestCheckBeadStateAssigneeWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "other-agent"}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "already assigned to \"other-agent\"") {
		t.Errorf("stderr = %q, want assignee warning", stderr.String())
	}
	// Bead should still be routed.
	if len(runner.calls) != 1 {
		t.Errorf("got %d runner calls, want 1", len(runner.calls))
	}
}

func TestCheckBeadStatePoolLabelWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Labels: []string{"pool:hw/polecat"}}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "already has pool label \"pool:hw/polecat\"") {
		t.Errorf("stderr = %q, want pool label warning", stderr.String())
	}
}

func TestCheckBeadStateBothWarnings(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{
		ID:       "BL-42",
		Assignee: "other-agent",
		Labels:   []string{"pool:hw/polecat"},
	}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "already assigned") {
		t.Errorf("stderr = %q, want assignee warning", stderr.String())
	}
	if !strings.Contains(stderr.String(), "already has pool label") {
		t.Errorf("stderr = %q, want pool label warning", stderr.String())
	}
}

func TestCheckBeadStateCleanNoWarning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42"}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("clean bead should produce no warnings; stderr = %q", stderr.String())
	}
}

func TestCheckBeadStateQueryFailsNoWarning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{err: fmt.Errorf("bd not available")}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("query failure should produce no warnings; stderr = %q", stderr.String())
	}
}

func TestCheckBeadStateNilQuerierNoWarning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("nil querier should produce no warnings; stderr = %q", stderr.String())
	}
}

func TestCheckBeadStateForceSkipsCheck(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "other-agent"}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.Force = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "already assigned") {
		t.Errorf("--force should suppress pre-flight warnings; stderr = %q", stderr.String())
	}
}

func TestCheckBeadStateFormulaChecksResolvedBead(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-99\n", nil)
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	// The querier returns a clean bead for the wisp root — verifies check
	// runs on WP-99, not the formula name "my-formula".
	q := &fakeQuerier{bead: beads.Bead{ID: "WP-99"}}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "my-formula")
	opts.IsFormula = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("clean wisp root should produce no warnings; stderr = %q", stderr.String())
	}
}

// --- Batch dispatch (doSlingBatch) tests ---

func TestDoSlingBatchConvoyExpandsChildren(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
		{ID: "BL-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 3 {
		t.Fatalf("got %d runner calls, want 3: %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(stdout.String(), "Expanding convoy CVY-1") {
		t.Errorf("stdout = %q, want expansion header", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Slung 3/3 children") {
		t.Errorf("stdout = %q, want summary line", stdout.String())
	}
}

func TestDoSlingBatchConvoyMixedStatus(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-2"] = beads.Bead{ID: "CVY-2", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-2"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "closed"},
		{ID: "BL-3", Status: "open"},
		{ID: "BL-4", Status: "in_progress"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-2")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2: %v", len(runner.calls), runner.calls)
	}
	out := stdout.String()
	if !strings.Contains(out, "Expanding convoy CVY-2 (4 children, 2 open)") {
		t.Errorf("stdout = %q, want header with counts", out)
	}
	if !strings.Contains(out, "Skipped BL-2 (status: closed)") {
		t.Errorf("stdout = %q, want skipped BL-2", out)
	}
	if !strings.Contains(out, "Skipped BL-4 (status: in_progress)") {
		t.Errorf("stdout = %q, want skipped BL-4", out)
	}
	if !strings.Contains(out, "Slung 2/4 children") {
		t.Errorf("stdout = %q, want summary", out)
	}
}

func TestDoSlingBatchConvoyNoOpenChildren(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-3"] = beads.Bead{ID: "CVY-3", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-3"] = []beads.Bead{
		{ID: "BL-1", Status: "closed"},
		{ID: "BL-2", Status: "closed"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-3")
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no open children") {
		t.Errorf("stderr = %q, want 'no open children'", stderr.String())
	}
}

func TestDoSlingBatchEpicExpands(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["EP-1"] = beads.Bead{ID: "EP-1", Type: "epic", Status: "open"}
	q.childrenOf["EP-1"] = []beads.Bead{
		{ID: "BL-10", Status: "open"},
		{ID: "BL-11", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "EP-1")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2: %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(stdout.String(), "Expanding epic EP-1") {
		t.Errorf("stdout = %q, want epic expansion header", stdout.String())
	}
}

func TestDoSlingBatchRegularBeadPassthrough(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Should route the bead directly, not expand.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(stdout.String(), "Slung BL-42") {
		t.Errorf("stdout = %q, want direct sling output", stdout.String())
	}
	if strings.Contains(stdout.String(), "Expanding") {
		t.Errorf("stdout = %q, should not expand a regular bead", stdout.String())
	}
}

func TestDoSlingBatchFormulaPassthrough(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	// Even if the querier has a convoy, --formula bypasses container check.
	q.beadsByID["convoy-formula"] = beads.Bead{ID: "convoy-formula", Type: "convoy"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "convoy-formula")
	opts.IsFormula = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Should have gone through formula path.
	if !strings.Contains(stdout.String(), "formula") {
		t.Errorf("stdout = %q, want formula output", stdout.String())
	}
}

func TestDoSlingBatchNilQuerier(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSlingBatch(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Slung BL-42") {
		t.Errorf("stdout = %q, want direct sling output", stdout.String())
	}
}

func TestDoSlingBatchGetFails(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.getErr = fmt.Errorf("bd not available")

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0 (falls through to doSling); stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Slung BL-42") {
		t.Errorf("stdout = %q, want direct sling output", stdout.String())
	}
}

func TestDoSlingBatchChildrenFails(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenErr = fmt.Errorf("storage error")

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "listing children") {
		t.Errorf("stderr = %q, want children error", stderr.String())
	}
}

func TestDoSlingBatchPartialFailure(t *testing.T) {
	runner := newFakeRunner()
	// Fail on BL-2 only.
	runner.on("BL-2", "", fmt.Errorf("bd update failed"))
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
		{ID: "BL-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1 (partial failure)", code)
	}
	// BL-1 and BL-3 should have been routed.
	if !strings.Contains(stdout.String(), "Slung BL-1") {
		t.Errorf("stdout = %q, want BL-1 routed", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Slung BL-3") {
		t.Errorf("stdout = %q, want BL-3 routed", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Failed BL-2") {
		t.Errorf("stderr = %q, want BL-2 failure", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Slung 2/3 children") {
		t.Errorf("stdout = %q, want summary", stdout.String())
	}
}

func TestDoSlingBatchAllChildrenFail(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd update", "", fmt.Errorf("bd broken"))
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
	}

	deps, stdout, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "Slung 0/2 children") {
		t.Errorf("stdout = %q, want 0/2 summary", stdout.String())
	}
}

func TestDoSlingBatchNudgeOnceAfterAll(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	_ = sp.Start("gc-test-city-mayor", session.Config{})
	sp.Calls = nil
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.Nudge = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Count nudge calls — should be exactly one.
	nudgeCount := 0
	for _, c := range sp.Calls {
		if c.Method == "Nudge" {
			nudgeCount++
		}
	}
	if nudgeCount != 1 {
		t.Errorf("got %d nudge calls, want 1; calls: %+v", nudgeCount, sp.Calls)
	}
}

func TestDoSlingBatchForceSkipsPerChildWarnings(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	// Children already assigned — would normally warn.
	q.beadsByID["BL-1"] = beads.Bead{ID: "BL-1", Status: "open", Assignee: "other"}
	q.beadsByID["BL-2"] = beads.Bead{ID: "BL-2", Status: "open", Assignee: "other"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open", Assignee: "other"},
		{ID: "BL-2", Status: "open", Assignee: "other"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.Force = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "already assigned") {
		t.Errorf("--force should suppress per-child warnings; stderr = %q", stderr.String())
	}
}

// --- On-formula (--on) tests ---

func TestOnAndFormulaMutuallyExclusive(t *testing.T) {
	cmd := newSlingCmd(&bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"mayor", "BL-1", "--formula", "--on=code-review"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for mutually exclusive --formula and --on")
	}
	if !strings.Contains(err.Error(), "if any flags in the group") {
		t.Errorf("err = %v, want mutual exclusion error", err)
	}
}

func TestOnFormulaAttachesAndRoutes(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; only the routing call goes through runner.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	// --on routes the ORIGINAL bead (not the wisp root).
	wantSling := "bd update BL-42 --assignee=mayor"
	if runner.calls[0] != wantSling {
		t.Errorf("runner call = %q, want %q", runner.calls[0], wantSling)
	}
	// Verify wisp was created in the store with correct ParentID.
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.ParentID != "BL-42" {
		t.Errorf("wisp ParentID = %q, want %q", b.ParentID, "BL-42")
	}
	if b.Ref != "code-review" {
		t.Errorf("wisp Ref = %q, want %q", b.Ref, "code-review")
	}
}

func TestOnFormulaWithTitle(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.Title = "my-review"
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; verify bead was created with title and parent.
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.Title != "my-review" {
		t.Errorf("bead title = %q, want %q", b.Title, "my-review")
	}
	if b.ParentID != "BL-42" {
		t.Errorf("bead ParentID = %q, want %q", b.ParentID, "BL-42")
	}
}

func TestOnFormulaCookError(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	deps.Store = &errStore{Store: beads.NewMemStore(), cookErr: fmt.Errorf("formula not found")}
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "bad-formula"
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "formula not found") {
		t.Errorf("stderr = %q, want formula error", stderr.String())
	}
}

func TestOnFormulaCookEmpty(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	deps.Store = &errStore{Store: beads.NewMemStore(), cookErr: fmt.Errorf("empty output from mol cook")}
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "empty output") {
		t.Errorf("stderr = %q, want 'empty output'", stderr.String())
	}
}

func TestOnFormulaExistingMoleculeErrors(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{
		{ID: "MOL-1", Type: "molecule", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already has attached molecule MOL-1") {
		t.Errorf("stderr = %q, want molecule error", stderr.String())
	}
	// No runner calls — should fail before routing.
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0 (should not route)", len(runner.calls))
	}
}

func TestOnFormulaExistingWispErrors(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{
		{ID: "WP-5", Type: "wisp", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already has attached wisp WP-5") {
		t.Errorf("stderr = %q, want wisp error", stderr.String())
	}
}

func TestOnFormulaCleanBead(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{
		{ID: "STEP-1", Type: "step", Status: "open"}, // step, not molecule
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; only the routing call goes through runner.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
}

func TestOnFormulaNilQuerier(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	// nil querier → molecule check skipped, should succeed.
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
}

func TestOnFormulaOutput(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// MemStore generates IDs like "gc-1".
	if !strings.Contains(out, "Attached wisp gc-1 (formula \"code-review\") to BL-42") {
		t.Errorf("stdout = %q, want attach message", out)
	}
	if !strings.Contains(out, "Slung BL-42 (with formula \"code-review\")") {
		t.Errorf("stdout = %q, want slung with formula message", out)
	}
}

func TestBatchOnConvoy(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
		{ID: "BL-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.OnFormula = "code-review"
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store, verify 3 wisps were created.
	all, _ := deps.Store.List()
	molCount := 0
	for _, b := range all {
		if b.Type == "molecule" {
			molCount++
		}
	}
	if molCount != 3 {
		t.Errorf("got %d molecule beads in store, want 3", molCount)
	}
	out := stdout.String()
	// MemStore generates IDs like gc-1, gc-2, gc-3.
	if !strings.Contains(out, "Attached wisp gc-1") {
		t.Errorf("stdout = %q, want gc-1 attach", out)
	}
	if !strings.Contains(out, "Attached wisp gc-2") {
		t.Errorf("stdout = %q, want gc-2 attach", out)
	}
	if !strings.Contains(out, "Attached wisp gc-3") {
		t.Errorf("stdout = %q, want gc-3 attach", out)
	}
	if !strings.Contains(out, "Slung 3/3 children") {
		t.Errorf("stdout = %q, want summary", out)
	}
}

func TestBatchOnFailFastMolecule(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
	}
	// BL-2 has an existing molecule child.
	q.childrenOf["BL-2"] = []beads.Bead{
		{ID: "MOL-1", Type: "molecule", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.OnFormula = "code-review"
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "cannot use --on") {
		t.Errorf("stderr = %q, want '--on' error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "BL-2 (has molecule MOL-1)") {
		t.Errorf("stderr = %q, want BL-2 details", stderr.String())
	}
	// Nothing should be routed — fail-fast.
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0 (fail-fast)", len(runner.calls))
	}
}

func TestBatchOnPartialCookFailure(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
		{ID: "BL-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	// Fail MolCookOn for BL-2 only.
	deps.Store = &selectiveErrStore{
		Store:         beads.NewMemStore(),
		failOnBeadIDs: map[string]error{"BL-2": fmt.Errorf("cook failed for BL-2")},
	}
	opts := testOpts(a, "CVY-1")
	opts.OnFormula = "code-review"
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1 (partial failure)", code)
	}
	out := stdout.String()
	// BL-1 and BL-3 should be routed.
	if !strings.Contains(out, "Slung BL-1") {
		t.Errorf("stdout = %q, want BL-1 routed", out)
	}
	if !strings.Contains(out, "Slung BL-3") {
		t.Errorf("stdout = %q, want BL-3 routed", out)
	}
	if !strings.Contains(stderr.String(), "Failed BL-2") {
		t.Errorf("stderr = %q, want BL-2 failure", stderr.String())
	}
	if !strings.Contains(out, "Slung 2/3 children") {
		t.Errorf("stdout = %q, want summary", out)
	}
}

func TestBatchOnNudgeOnce(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	_ = sp.Start("gc-test-city-mayor", session.Config{})
	sp.Calls = nil
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.Nudge = true
	opts.OnFormula = "code-review"
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	nudgeCount := 0
	for _, c := range sp.Calls {
		if c.Method == "Nudge" {
			nudgeCount++
		}
	}
	if nudgeCount != 1 {
		t.Errorf("got %d nudge calls, want 1; calls: %+v", nudgeCount, sp.Calls)
	}
}

func TestBatchOnRegularPassthrough(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	// Non-container bead + --on → should fall through to doSling.
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// MemStore generates IDs like "gc-1".
	if !strings.Contains(out, "Attached wisp gc-1") {
		t.Errorf("stdout = %q, want attach message", out)
	}
	if !strings.Contains(out, "Slung BL-42 (with formula") {
		t.Errorf("stdout = %q, want slung with formula", out)
	}
	if strings.Contains(out, "Expanding") {
		t.Errorf("stdout = %q, should not expand a regular bead", out)
	}
}

// --- Dry-run tests ---

func TestDryRunFlagExists(t *testing.T) {
	cmd := newSlingCmd(&bytes.Buffer{}, &bytes.Buffer{})
	f := cmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("missing --dry-run flag")
	}
	if f.Shorthand != "n" {
		t.Errorf("--dry-run shorthand = %q, want %q", f.Shorthand, "n")
	}
}

func TestDryRunSingleBead(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Title: "Implement login page", Type: "task", Status: "open"}}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.DryRun = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Target section.
	if !strings.Contains(out, "Agent:       mayor (fixed agent)") {
		t.Errorf("stdout missing agent info: %s", out)
	}
	if !strings.Contains(out, "Sling query: bd update {} --assignee=mayor") {
		t.Errorf("stdout missing sling query: %s", out)
	}
	// Work section.
	if !strings.Contains(out, "BL-42") {
		t.Errorf("stdout missing bead ID: %s", out)
	}
	if !strings.Contains(out, "Implement login page") {
		t.Errorf("stdout missing bead title: %s", out)
	}
	// Route command.
	if !strings.Contains(out, "bd update BL-42 --assignee=mayor") {
		t.Errorf("stdout missing route command: %s", out)
	}
	// Footer.
	if !strings.Contains(out, "No side effects executed (--dry-run).") {
		t.Errorf("stdout missing footer: %s", out)
	}
	// Zero mutations.
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0 (dry-run): %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunFormula(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "code-review")
	opts.IsFormula = true
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Formula:") {
		t.Errorf("stdout missing Formula section: %s", out)
	}
	if !strings.Contains(out, "Name: code-review") {
		t.Errorf("stdout missing formula name: %s", out)
	}
	if !strings.Contains(out, "Would run: bd mol cook --formula=code-review") {
		t.Errorf("stdout missing cook command: %s", out)
	}
	if !strings.Contains(out, "<wisp-root>") {
		t.Errorf("stdout missing wisp-root placeholder: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunOnFormula(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{} // no molecule children

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	opts.DryRun = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Attach formula:") {
		t.Errorf("stdout missing attach section: %s", out)
	}
	if !strings.Contains(out, "Would run: bd mol cook --formula=code-review --on=BL-42") {
		t.Errorf("stdout missing cook command: %s", out)
	}
	if !strings.Contains(out, "Pre-check: BL-42 has no existing molecule/wisp children") {
		t.Errorf("stdout missing pre-check: %s", out)
	}
	if !strings.Contains(out, "bd update BL-42 --assignee=mayor") {
		t.Errorf("stdout missing route command: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunPool(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{
		Name: "polecat",
		Dir:  "hw",
		Pool: &config.PoolConfig{Min: 1, Max: 3},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Pool:        hw/polecat (min=1 max=3)") {
		t.Errorf("stdout missing pool info: %s", out)
	}
	if !strings.Contains(out, "bd update {} --label=pool:hw/polecat") {
		t.Errorf("stdout missing sling query: %s", out)
	}
	if !strings.Contains(out, "Pool agents share a work queue via labels") {
		t.Errorf("stdout missing pool explanation: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunConvoy(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open", Title: "Sprint 12 tasks"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Title: "Login page", Status: "open"},
		{ID: "BL-2", Title: "Auth backend", Status: "closed"},
		{ID: "BL-3", Title: "Session mgmt", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.DryRun = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Container explanation.
	if !strings.Contains(out, "convoy") {
		t.Errorf("stdout missing convoy type: %s", out)
	}
	// Children list.
	if !strings.Contains(out, "Children (3 total, 2 open)") {
		t.Errorf("stdout missing children summary: %s", out)
	}
	if !strings.Contains(out, "BL-1") || !strings.Contains(out, "would route") {
		t.Errorf("stdout missing BL-1 route: %s", out)
	}
	if !strings.Contains(out, "BL-2") {
		t.Errorf("stdout missing BL-2: %s", out)
	}
	if !strings.Contains(out, "skip") {
		t.Errorf("stdout missing skip indicator: %s", out)
	}
	// Route commands.
	if !strings.Contains(out, "bd update BL-1 --assignee=mayor") {
		t.Errorf("stdout missing BL-1 route command: %s", out)
	}
	if !strings.Contains(out, "bd update BL-3 --assignee=mayor") {
		t.Errorf("stdout missing BL-3 route command: %s", out)
	}
	if strings.Contains(out, "bd update BL-2 --assignee=mayor") {
		t.Errorf("stdout should not route closed BL-2: %s", out)
	}
	// Zero mutations.
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunBatchOnFormula(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "closed"},
		{ID: "BL-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.OnFormula = "code-review"
	opts.DryRun = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Per-child cook commands.
	if !strings.Contains(out, "bd mol cook --formula=code-review --on=BL-1") {
		t.Errorf("stdout missing BL-1 cook command: %s", out)
	}
	if !strings.Contains(out, "bd mol cook --formula=code-review --on=BL-3") {
		t.Errorf("stdout missing BL-3 cook command: %s", out)
	}
	if strings.Contains(out, "bd mol cook --formula=code-review --on=BL-2") {
		t.Errorf("stdout should not cook for closed BL-2: %s", out)
	}
	// Route commands.
	if !strings.Contains(out, "bd update BL-1 --assignee=mayor") {
		t.Errorf("stdout missing BL-1 route: %s", out)
	}
	if !strings.Contains(out, "bd update BL-3 --assignee=mayor") {
		t.Errorf("stdout missing BL-3 route: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunNudgeRunning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	_ = sp.Start("gc-test-city-mayor", session.Config{})
	sp.Calls = nil
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Nudge:") {
		t.Errorf("stdout missing Nudge section: %s", out)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("stdout missing running status: %s", out)
	}
	// No actual nudge should have been sent.
	for _, c := range sp.Calls {
		if c.Method == "Nudge" {
			t.Error("dry-run should not send an actual nudge")
		}
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunNudgeNotRunning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.Nudge = true
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "no running session") {
		t.Errorf("stdout missing 'no running session': %s", out)
	}
}

func TestDryRunNoMutations(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, _, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0", code)
	}
	if len(runner.calls) != 0 {
		t.Errorf("dry-run executed %d commands, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunSuspendedWarning(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-1")
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0", code)
	}
	// Suspended warning should still fire to stderr.
	if !strings.Contains(stderr.String(), "suspended") {
		t.Errorf("stderr = %q, want suspended warning", stderr.String())
	}
	// But no mutations.
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunOnExistingMolecule(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{
		{ID: "MOL-1", Type: "molecule", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "code-review"
	opts.DryRun = true
	code := doSling(opts, deps, q)

	if code != 1 {
		t.Fatalf("dry-run returned %d, want 1 (existing molecule)", code)
	}
	if !strings.Contains(stderr.String(), "already has attached molecule MOL-1") {
		t.Errorf("stderr = %q, want molecule error", stderr.String())
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunNilQuerier(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Should still show bead ID even without querier details.
	if !strings.Contains(out, "BL-42") {
		t.Errorf("stdout missing bead ID: %s", out)
	}
	if !strings.Contains(out, "No side effects executed (--dry-run).") {
		t.Errorf("stdout missing footer: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

// --- Idempotency detection (checkBeadState + integration) tests ---

func TestCheckBeadStateIdempotentFixedAgent(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "mayor"}}
	a := config.Agent{Name: "mayor"}

	result := checkBeadState(q, "BL-42", a)
	if !result.Idempotent {
		t.Error("expected Idempotent=true for matching assignee")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestCheckBeadStateIdempotentPool(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Labels: []string{"pool:hw/polecat"}}}
	a := config.Agent{Name: "polecat", Dir: "hw", Pool: &config.PoolConfig{Min: 1, Max: 3}}

	result := checkBeadState(q, "BL-42", a)
	if !result.Idempotent {
		t.Error("expected Idempotent=true for matching pool label")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestCheckBeadStateIdempotentPoolMultiLabels(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{
		ID:     "BL-42",
		Labels: []string{"priority:high", "pool:hw/polecat", "sprint:3"},
	}}
	a := config.Agent{Name: "polecat", Dir: "hw", Pool: &config.PoolConfig{Min: 1, Max: 3}}

	result := checkBeadState(q, "BL-42", a)
	if !result.Idempotent {
		t.Error("expected Idempotent=true for matching pool label among others")
	}
}

func TestCheckBeadStateCustomQueryNoIdempotency(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "mayor"}}
	a := config.Agent{Name: "mayor", SlingQuery: "custom-script {} --route"}

	result := checkBeadState(q, "BL-42", a)
	if result.Idempotent {
		t.Error("expected Idempotent=false for custom sling_query (can't detect)")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "already assigned") {
		t.Errorf("expected assignee warning, got %q", result.Warnings[0])
	}
}

func TestCheckBeadStateDifferentAssignee(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "other-agent"}}
	a := config.Agent{Name: "mayor"}

	result := checkBeadState(q, "BL-42", a)
	if result.Idempotent {
		t.Error("expected Idempotent=false for different assignee")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "already assigned to \"other-agent\"") {
		t.Errorf("expected assignee warning, got %q", result.Warnings[0])
	}
}

func TestCheckBeadStateDifferentPoolLabel(t *testing.T) {
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Labels: []string{"pool:other/pool"}}}
	a := config.Agent{Name: "polecat", Dir: "hw", Pool: &config.PoolConfig{Min: 1, Max: 3}}

	result := checkBeadState(q, "BL-42", a)
	if result.Idempotent {
		t.Error("expected Idempotent=false for different pool label")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "pool:other/pool") {
		t.Errorf("expected pool label warning, got %q", result.Warnings[0])
	}
}

func TestDoSlingIdempotentSkipsRouting(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "mayor"}}

	deps, stdout, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if len(runner.calls) != 0 {
		t.Errorf("idempotent bead should not be routed; got %d calls: %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(stdout.String(), "already routed to mayor") {
		t.Errorf("stdout = %q, want idempotent skip message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "skipping (idempotent)") {
		t.Errorf("stdout = %q, want idempotent label", stdout.String())
	}
}

func TestDoSlingIdempotentForceOverrides(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "mayor"}}

	deps, stdout, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.Force = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	// --force should bypass idempotency and route.
	if len(runner.calls) != 1 {
		t.Errorf("--force should route; got %d calls, want 1", len(runner.calls))
	}
	if strings.Contains(stdout.String(), "idempotent") {
		t.Errorf("--force should not print idempotent message; stdout = %q", stdout.String())
	}
}

func TestDoSlingIdempotentWithOnFormula(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	// Bead is already assigned to mayor — idempotent.
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Assignee: "mayor"}}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.OnFormula = "my-formula"
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Idempotent — should skip both wisp attachment and routing.
	if len(runner.calls) != 0 {
		t.Errorf("idempotent + --on should skip all mutations; got %d calls: %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(stdout.String(), "skipping (idempotent)") {
		t.Errorf("stdout = %q, want idempotent skip message", stdout.String())
	}
}

func TestDoSlingBatchIdempotentChildSkipped(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	// BL-1 is already assigned to mayor (idempotent).
	q.beadsByID["BL-1"] = beads.Bead{ID: "BL-1", Status: "open", Assignee: "mayor"}
	// BL-2 is clean.
	q.beadsByID["BL-2"] = beads.Bead{ID: "BL-2", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open", Assignee: "mayor"},
		{ID: "BL-2", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Only BL-2 should be routed.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1 (BL-1 idempotent): %v", len(runner.calls), runner.calls)
	}
	if !strings.Contains(runner.calls[0], "BL-2") {
		t.Errorf("expected BL-2 to be routed, got %q", runner.calls[0])
	}
	out := stdout.String()
	if !strings.Contains(out, "Skipped BL-1") {
		t.Errorf("stdout should mention skipped BL-1: %s", out)
	}
	if !strings.Contains(out, "already routed to mayor") {
		t.Errorf("stdout should mention idempotent skip: %s", out)
	}
	if !strings.Contains(out, "Slung 1/2 children") {
		t.Errorf("stdout summary should show 1/2 routed: %s", out)
	}
	if !strings.Contains(out, "(1 already routed)") {
		t.Errorf("stdout summary should mention idempotent count: %s", out)
	}
}

func TestDoSlingBatchAllIdempotent(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.beadsByID["BL-1"] = beads.Bead{ID: "BL-1", Status: "open", Assignee: "mayor"}
	q.beadsByID["BL-2"] = beads.Bead{ID: "BL-2", Status: "open", Assignee: "mayor"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open", Assignee: "mayor"},
		{ID: "BL-2", Status: "open", Assignee: "mayor"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 0 {
		t.Errorf("all idempotent: got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
	out := stdout.String()
	if !strings.Contains(out, "Slung 0/2 children") {
		t.Errorf("stdout summary should show 0/2 routed: %s", out)
	}
	if !strings.Contains(out, "(2 already routed)") {
		t.Errorf("stdout summary should mention both idempotent: %s", out)
	}
}

func TestDryRunIdempotentBead(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	q := &fakeQuerier{bead: beads.Bead{ID: "BL-42", Title: "Login page", Assignee: "mayor", Status: "open"}}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "BL-42")
	opts.DryRun = true
	code := doSling(opts, deps, q)

	// Dry-run reaches the full preview — including the Idempotency section.
	if code != 0 {
		t.Fatalf("returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	// Should show idempotency section.
	if !strings.Contains(out, "Idempotency:") {
		t.Errorf("stdout missing Idempotency section: %s", out)
	}
	if !strings.Contains(out, "already routed to mayor") {
		t.Errorf("stdout should show idempotent info: %s", out)
	}
	// Should show the footer (proving dryRunSingle was reached).
	if !strings.Contains(out, "No side effects executed (--dry-run).") {
		t.Errorf("stdout missing footer: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

// --- Cross-rig guard tests ---

func TestBeadPrefix(t *testing.T) {
	tests := []struct {
		beadID string
		want   string
	}{
		{"HW-7", "hw"},
		{"FE-123", "fe"},
		{"BL-42", "bl"},
		{"bad", ""},
		{"", ""},
		{"-1", ""},
	}
	for _, tt := range tests {
		got := beadPrefix(tt.beadID)
		if got != tt.want {
			t.Errorf("beadPrefix(%q) = %q, want %q", tt.beadID, got, tt.want)
		}
	}
}

func TestRigPrefixForAgentCityWide(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "mayor", Dir: ""} // city-wide

	got := rigPrefixForAgent(a, cfg)
	if got != "" {
		t.Errorf("rigPrefixForAgent(city-wide) = %q, want empty (exempt)", got)
	}
}

func TestRigPrefixForAgentRigScoped(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	got := rigPrefixForAgent(a, cfg)
	// DeriveBeadsPrefix("hello-world") = "hw"
	if got != "hw" {
		t.Errorf("rigPrefixForAgent(rig-scoped) = %q, want %q", got, "hw")
	}
}

func TestRigPrefixForAgentExplicitPrefix(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw", Prefix: "HELLO"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	got := rigPrefixForAgent(a, cfg)
	if got != "hello" {
		t.Errorf("rigPrefixForAgent(explicit prefix) = %q, want %q", got, "hello")
	}
}

func TestRigPrefixForAgentOrphanDir(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "nonexistent"}

	got := rigPrefixForAgent(a, cfg)
	if got != "" {
		t.Errorf("rigPrefixForAgent(orphan dir) = %q, want empty (best-effort skip)", got)
	}
}

func TestCheckCrossRigSameRig(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	msg := checkCrossRig("HW-7", a, cfg)
	if msg != "" {
		t.Errorf("checkCrossRig(same rig) = %q, want empty", msg)
	}
}

func TestCheckCrossRigDifferentRig(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	msg := checkCrossRig("FE-123", a, cfg)
	if msg == "" {
		t.Fatal("checkCrossRig(different rig) = empty, want error message")
	}
	if !strings.Contains(msg, "cross-rig") {
		t.Errorf("message = %q, want 'cross-rig'", msg)
	}
	if !strings.Contains(msg, `prefix "fe"`) {
		t.Errorf("message = %q, want bead prefix", msg)
	}
	if !strings.Contains(msg, `rig prefix "hw"`) {
		t.Errorf("message = %q, want rig prefix", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("message = %q, want --force hint", msg)
	}
}

func TestCheckCrossRigCityAgent(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "mayor", Dir: ""} // city-wide

	msg := checkCrossRig("FE-123", a, cfg)
	if msg != "" {
		t.Errorf("checkCrossRig(city-wide agent) = %q, want empty (exempt)", msg)
	}
}

func TestDoSlingCrossRigBlocks(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-123")
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1 (cross-rig block)", code)
	}
	if !strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("stderr = %q, want cross-rig error", stderr.String())
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0 (should not route)", len(runner.calls))
	}
}

func TestDoSlingCrossRigForceOverrides(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-123")
	opts.Force = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (--force overrides cross-rig); stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("--force should suppress cross-rig block; stderr = %q", stderr.String())
	}
	if len(runner.calls) != 1 {
		t.Errorf("got %d runner calls, want 1 (should route with --force)", len(runner.calls))
	}
}

func TestDoSlingCrossRigSameRigAllowed(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-7")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (same rig); stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("same-rig bead should not trigger cross-rig block; stderr = %q", stderr.String())
	}
}

func TestDoSlingBatchCrossRigBlocks(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	q := newFakeChildQuerier()
	q.beadsByID["FE-1"] = beads.Bead{ID: "FE-1", Type: "convoy", Status: "open"}
	q.childrenOf["FE-1"] = []beads.Bead{
		{ID: "FE-2", Status: "open"},
		{ID: "FE-3", Status: "open"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-1")
	code := doSlingBatch(opts, deps, q)

	if code != 1 {
		t.Fatalf("doSlingBatch returned %d, want 1 (cross-rig block)", code)
	}
	if !strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("stderr = %q, want cross-rig error", stderr.String())
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0 (should not route)", len(runner.calls))
	}
}

func TestDryRunCrossRigSection(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}
	q := &fakeQuerier{bead: beads.Bead{ID: "FE-123", Type: "task", Status: "open"}}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-123")
	opts.DryRun = true
	code := doSling(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Cross-rig:") {
		t.Errorf("stdout missing Cross-rig section: %s", out)
	}
	if !strings.Contains(out, `prefix "fe"`) {
		t.Errorf("stdout missing bead prefix: %s", out)
	}
	if !strings.Contains(out, `rig prefix "hw"`) {
		t.Errorf("stdout missing rig prefix: %s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("stdout missing --force hint: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDryRunBatchCrossRigSection(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	q := newFakeChildQuerier()
	q.beadsByID["FE-1"] = beads.Bead{ID: "FE-1", Type: "convoy", Status: "open"}
	q.childrenOf["FE-1"] = []beads.Bead{
		{ID: "FE-2", Status: "open"},
		{ID: "FE-3", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-1")
	opts.DryRun = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Cross-rig:") {
		t.Errorf("stdout missing Cross-rig section: %s", out)
	}
	if !strings.Contains(out, `prefix "fe"`) {
		t.Errorf("stdout missing bead prefix: %s", out)
	}
	if !strings.Contains(out, `rig prefix "hw"`) {
		t.Errorf("stdout missing rig prefix: %s", out)
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0: %v", len(runner.calls), runner.calls)
	}
}

func TestDoSlingCrossRigFormulaExempt(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "code-review")
	opts.IsFormula = true
	// Formula mode — cross-rig check should not apply.
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0 (formula exempt from cross-rig); stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("formula mode should not trigger cross-rig; stderr = %q", stderr.String())
	}
}

// --- New tests for shell quoting, helpers, and edge cases ---

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"it's", "'it'\\''s'"},
		{"a b c", "'a b c'"},
		{"", "''"},
		{"$(rm -rf /)", "'$(rm -rf /)'"},
		{"`evil`", "'`evil`'"},
		{"hello'world'end", "'hello'\\''world'\\''end'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatBeadLabel(t *testing.T) {
	if got := formatBeadLabel("BL-42", ""); got != "BL-42" {
		t.Errorf("formatBeadLabel(no title) = %q, want %q", got, "BL-42")
	}
	if got := formatBeadLabel("BL-42", "Login page"); !strings.Contains(got, "BL-42") || !strings.Contains(got, "Login page") {
		t.Errorf("formatBeadLabel(with title) = %q, want BL-42 and title", got)
	}
}

func TestDoSlingOnFormulaCrossRigBlocked(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-123")
	opts.OnFormula = "code-review"
	code := doSling(opts, deps, nil)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1 (cross-rig block with --on)", code)
	}
	if !strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("stderr = %q, want cross-rig error", stderr.String())
	}
	if len(runner.calls) != 0 {
		t.Errorf("got %d runner calls, want 0", len(runner.calls))
	}
}

func TestDoSlingOnFormulaCrossRigForceOverrides(t *testing.T) {
	runner := newFakeRunner()
	runner.on("bd mol cook", "WP-1\n", nil)
	sp := session.NewFake()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test-city"},
		Rigs:      []config.Rig{{Name: "hello-world", Path: "/tmp/hw"}},
	}
	a := config.Agent{Name: "polecat", Dir: "hello-world"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "FE-123")
	opts.OnFormula = "code-review"
	opts.Force = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "cross-rig") {
		t.Errorf("--force should suppress cross-rig; stderr = %q", stderr.String())
	}
}

func TestDoSlingBatchAllIdempotentNoNudge(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	_ = sp.Start("gc-test-city-mayor", session.Config{})
	sp.Calls = nil
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.beadsByID["BL-1"] = beads.Bead{ID: "BL-1", Status: "open", Assignee: "mayor"}
	q.beadsByID["BL-2"] = beads.Bead{ID: "BL-2", Status: "open", Assignee: "mayor"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open", Assignee: "mayor"},
		{ID: "BL-2", Status: "open", Assignee: "mayor"},
	}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	opts.Nudge = true
	code := doSlingBatch(opts, deps, q)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// All idempotent → 0 routed → no nudge should fire.
	for _, c := range sp.Calls {
		if c.Method == "Nudge" {
			t.Error("all-idempotent batch should not nudge")
		}
	}
}

func TestBeadPrefixMultiDash(t *testing.T) {
	tests := []struct {
		beadID string
		want   string
	}{
		{"A-B-C", "a"},
		{"A-", "a"},
		{"ABC-DEF-123", "abc"},
	}
	for _, tt := range tests {
		got := beadPrefix(tt.beadID)
		if got != tt.want {
			t.Errorf("beadPrefix(%q) = %q, want %q", tt.beadID, got, tt.want)
		}
	}
}

// --- Default sling formula tests ---

func TestDefaultFormulaApplied(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-42")
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; only the routing call goes through runner.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	// Verify the store created a wisp with the default formula.
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.Ref != "mol-polecat-work" {
		t.Errorf("bead Ref = %q, want %q", b.Ref, "mol-polecat-work")
	}
	if b.ParentID != "HW-42" {
		t.Errorf("bead ParentID = %q, want %q", b.ParentID, "HW-42")
	}
	if !strings.Contains(stdout.String(), "default formula") {
		t.Errorf("stdout = %q, want mention of default formula", stdout.String())
	}
}

func TestDefaultFormulaNoFormulaOverride(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-42")
	opts.NoFormula = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// Only 1 call: the sling command, no wisp creation.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1 (no wisp): %v", len(runner.calls), runner.calls)
	}
	if strings.Contains(runner.calls[0], "bd mol cook") {
		t.Errorf("--no-formula should suppress default formula; call = %q", runner.calls[0])
	}
}

func TestDefaultFormulaExplicitOnOverrides(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-42")
	opts.OnFormula = "custom-formula"
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; verify the explicit formula was used.
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.Ref != "custom-formula" {
		t.Errorf("bead Ref = %q, want explicit custom-formula", b.Ref)
	}
	// Output should mention explicit formula, not default.
	if strings.Contains(stdout.String(), "default formula") {
		t.Errorf("stdout should not mention default formula when --on is explicit: %q", stdout.String())
	}
}

func TestDefaultFormulaExplicitFormulaOverrides(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	deps, _, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "code-review")
	opts.IsFormula = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCook goes through the store; only routing call goes through runner.
	if len(runner.calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(runner.calls), runner.calls)
	}
	// Verify the explicit formula was used (not the default).
	b, err := deps.Store.Get("gc-1")
	if err != nil {
		t.Fatalf("store.Get(gc-1): %v", err)
	}
	if b.Ref != "code-review" {
		t.Errorf("bead Ref = %q, want explicit code-review", b.Ref)
	}
}

func TestDefaultFormulaBatchApplied(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	querier := newFakeChildQuerier()
	querier.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	querier.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "HW-1", Type: "task", Status: "open"},
		{ID: "HW-2", Type: "task", Status: "open"},
	}

	deps, stdout, stderr := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "CVY-1")
	code := doSlingBatch(opts, deps, querier)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	// MolCookOn goes through the store; verify 2 molecule beads were created.
	all, _ := deps.Store.List()
	molCount := 0
	for _, b := range all {
		if b.Type == "molecule" {
			molCount++
		}
	}
	if molCount != 2 {
		t.Errorf("got %d molecule beads in store, want 2 (one per child)", molCount)
	}
	if !strings.Contains(stdout.String(), "default formula") {
		t.Errorf("stdout should mention default formula: %q", stdout.String())
	}
}

func TestDefaultFormulaDryRun(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "polecat", Dir: "hw", DefaultSlingFormula: "mol-polecat-work"}

	deps, stdout, _ := testDeps(cfg, sp, runner.run)
	opts := testOpts(a, "HW-42")
	opts.DryRun = true
	code := doSling(opts, deps, nil)

	if code != 0 {
		t.Fatalf("dryRunSingle returned %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Default formula:") {
		t.Errorf("dry-run should show Default formula section; got:\n%s", out)
	}
	if !strings.Contains(out, "mol-polecat-work") {
		t.Errorf("dry-run should show formula name; got:\n%s", out)
	}
	if !strings.Contains(out, "--no-formula") {
		t.Errorf("dry-run should mention --no-formula suppression; got:\n%s", out)
	}
	// No runner calls in dry-run.
	if len(runner.calls) != 0 {
		t.Errorf("dry-run should not execute commands; got %v", runner.calls)
	}
}

// --- 1-arg sling tests (via doSling, not cmdSling which needs a real city) ---

func TestFindRigByPrefix(t *testing.T) {
	cfg := &config.City{
		Rigs: []config.Rig{
			{Name: "hello-world", Path: "/tmp/hw", Prefix: "hw"},
			{Name: "my-project", Path: "/tmp/mp"},
		},
	}

	// Exact match.
	rig, ok := findRigByPrefix(cfg, "hw")
	if !ok {
		t.Fatal("expected to find rig with prefix hw")
	}
	if rig.Name != "hello-world" {
		t.Errorf("rig.Name = %q, want hello-world", rig.Name)
	}

	// Case-insensitive match.
	rig, ok = findRigByPrefix(cfg, "HW")
	if !ok {
		t.Fatal("expected case-insensitive match for HW")
	}
	if rig.Name != "hello-world" {
		t.Errorf("rig.Name = %q, want hello-world", rig.Name)
	}

	// Derived prefix match.
	rig, ok = findRigByPrefix(cfg, "mp")
	if !ok {
		t.Fatal("expected to find rig with derived prefix mp")
	}
	if rig.Name != "my-project" {
		t.Errorf("rig.Name = %q, want my-project", rig.Name)
	}

	// No match.
	_, ok = findRigByPrefix(cfg, "zz")
	if ok {
		t.Error("expected no match for prefix zz")
	}
}

func TestOneArgSlingNoPrefix(t *testing.T) {
	// A bead ID with no dash can't derive a prefix.
	// We test this through cmdSling but that requires a city on disk.
	// Instead, test the beadPrefix helper directly — already tested above.
	// The cmdSling path uses beadPrefix then errors, so this is coverage
	// via the TestNewSlingCmdArgs validation + beadPrefix tests.
	got := beadPrefix("nodash")
	if got != "" {
		t.Errorf("beadPrefix(%q) = %q, want empty", "nodash", got)
	}
}

func TestOneArgSlingFormulaRequiresTarget(t *testing.T) {
	// --formula with 1 arg is checked in newSlingCmd via cobra.RangeArgs.
	// Verify the flag exists and the error path message.
	cmd := newSlingCmd(&bytes.Buffer{}, &bytes.Buffer{})
	cmd.SetArgs([]string{"--formula", "code-review"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --formula with 1 arg")
	}
}
