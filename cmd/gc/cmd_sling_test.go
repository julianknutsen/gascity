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

// fakeRunner records the commands it receives and returns canned output.
type fakeRunner struct {
	calls []string
	out   map[string]string // command prefix → output
	err   map[string]error  // command prefix → error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		out: make(map[string]string),
		err: make(map[string]error),
	}
}

func (r *fakeRunner) run(command string) (string, error) {
	r.calls = append(r.calls, command)
	for prefix, err := range r.err {
		if strings.Contains(command, prefix) {
			return r.out[prefix], err
		}
	}
	for prefix, out := range r.out {
		if strings.Contains(command, prefix) {
			return out, nil
		}
	}
	return "", nil
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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "HW-7", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2: %v", len(runner.calls), runner.calls)
	}
	// First call: instantiate wisp.
	if !strings.Contains(runner.calls[0], "bd mol cook --formula=code-review") {
		t.Errorf("first call = %q, want bd mol cook", runner.calls[0])
	}
	// Second call: sling the root bead.
	wantSling := "bd update WP-1 --assignee=mayor"
	if runner.calls[1] != wantSling {
		t.Errorf("second call = %q, want %q", runner.calls[1], wantSling)
	}
	if !strings.Contains(stdout.String(), "formula") && !strings.Contains(stdout.String(), "wisp root WP-1") {
		t.Errorf("stdout = %q, want mention of formula/wisp", stdout.String())
	}
}

func TestDoSlingFormulaWithTitle(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-2\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "my-review", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(runner.calls[0], "--title=my-review") {
		t.Errorf("wisp call = %q, want --title=my-review", runner.calls[0])
	}
}

func TestDoSlingFormulaWithVars(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-3\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "",
		[]string{"version=1.0", "pr=123"}, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(runner.calls[0], "--var version=1.0") {
		t.Errorf("wisp call = %q, want --var version=1.0", runner.calls[0])
	}
	if !strings.Contains(runner.calls[0], "--var pr=123") {
		t.Errorf("wisp call = %q, want --var pr=123", runner.calls[0])
	}
}

func TestDoSlingFormulaWithVarsAndTitle(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-4\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "my-review",
		[]string{"name=auth"}, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(runner.calls[0], "--title=my-review") {
		t.Errorf("wisp call = %q, want --title=my-review", runner.calls[0])
	}
	if !strings.Contains(runner.calls[0], "--var name=auth") {
		t.Errorf("wisp call = %q, want --var name=auth", runner.calls[0])
	}
}

func TestDoSlingNilVarsOmitsFlag(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-5\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(runner.calls[0], "--var") {
		t.Errorf("nil vars should not produce --var flag; call = %q", runner.calls[0])
	}
}

func TestDoSlingSuspendedAgentWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, true, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, true, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "max=0") {
		t.Errorf("--force should suppress warning; stderr = %q", stderr.String())
	}
}

func TestDoSlingRunnerError(t *testing.T) {
	runner := newFakeRunner()
	runner.err["bd update"] = fmt.Errorf("bd not found")
	runner.out["bd update"] = ""
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bd not found") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
}

func TestDoSlingFormulaInstantiationError(t *testing.T) {
	runner := newFakeRunner()
	runner.err["bd mol cook"] = fmt.Errorf("formula not found")
	runner.out["bd mol cook"] = ""
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "nonexistent", true, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, true, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-99", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	want := "custom-dispatch BL-99 --queue=priority"
	if runner.calls[0] != want {
		t.Errorf("runner call = %q, want %q", runner.calls[0], want)
	}
}

func TestInstantiateWisp(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "  WP-42\n"

	rootID, err := instantiateWisp("code-review", "", nil, runner.run)
	if err != nil {
		t.Fatalf("instantiateWisp: %v", err)
	}
	if rootID != "WP-42" {
		t.Errorf("rootID = %q, want %q", rootID, "WP-42")
	}
	if !strings.Contains(runner.calls[0], "--formula=code-review") {
		t.Errorf("call = %q, want --formula=code-review", runner.calls[0])
	}
}

func TestInstantiateWispEmptyOutput(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "   \n"

	_, err := instantiateWisp("bad-formula", "", nil, runner.run)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("err = %v, want 'empty output'", err)
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
	if cmd.Use != "sling <target> <bead-or-formula>" {
		t.Errorf("Use = %q", cmd.Use)
	}
	// Verify flags exist.
	for _, name := range []string{"formula", "nudge", "force", "title"} {
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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, true, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0", code)
	}
	if strings.Contains(stderr.String(), "already assigned") {
		t.Errorf("--force should suppress pre-flight warnings; stderr = %q", stderr.String())
	}
}

func TestCheckBeadStateFormulaChecksResolvedBead(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-99\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}
	// The querier returns a clean bead for the wisp root — verifies check
	// runs on WP-99, not the formula name "my-formula".
	q := &fakeQuerier{bead: beads.Bead{ID: "WP-99"}}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "my-formula", true, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-2", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-3", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "EP-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	// Even if the querier has a convoy, --formula bypasses container check.
	q.beadsByID["convoy-formula"] = beads.Bead{ID: "convoy-formula", Type: "convoy"}

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "convoy-formula", true, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "BL-42", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	runner.err["BL-2"] = fmt.Errorf("bd update failed")
	runner.out["BL-2"] = ""
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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	runner.err["bd update"] = fmt.Errorf("bd broken")
	runner.out["bd update"] = ""
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["CVY-1"] = beads.Bead{ID: "CVY-1", Type: "convoy", Status: "open"}
	q.childrenOf["CVY-1"] = []beads.Bead{
		{ID: "BL-1", Status: "open"},
		{ID: "BL-2", Status: "open"},
	}

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, true, false, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, true, "", nil,
		"test-city", cfg, sp, runner.run, q, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "already assigned") {
		t.Errorf("--force should suppress per-child warnings; stderr = %q", stderr.String())
	}
}
