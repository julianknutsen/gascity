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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "HW-7", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "code-review", true, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "code-review", true, false, false, "my-review", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(runner.calls[0], "--title=my-review") {
		t.Errorf("wisp call = %q, want --title=my-review", runner.calls[0])
	}
}

func TestDoSlingSuspendedAgentWarns(t *testing.T) {
	runner := newFakeRunner()
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor", Suspended: true}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, true, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, true, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "nonexistent", true, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, true, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-99", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	rootID, err := instantiateWisp("code-review", "", runner.run)
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

	_, err := instantiateWisp("bad-formula", "", runner.run)
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
	for _, name := range []string{"formula", "nudge", "force", "title", "on"} {
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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSling(a, "BL-42", false, false, true, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSling(a, "my-formula", true, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-2", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-3", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "EP-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "convoy-formula", true, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
	code := doSlingBatch(a, "BL-42", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, true, false, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	code := doSlingBatch(a, "CVY-1", false, false, true, "", "",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2: %v", len(runner.calls), runner.calls)
	}
	// First call: instantiate wisp with --on.
	if !strings.Contains(runner.calls[0], "bd mol cook --formula=code-review --on=BL-42") {
		t.Errorf("first call = %q, want bd mol cook --on", runner.calls[0])
	}
	// Second call: sling the ORIGINAL bead (not the wisp root).
	wantSling := "bd update BL-42 --assignee=mayor"
	if runner.calls[1] != wantSling {
		t.Errorf("second call = %q, want %q", runner.calls[1], wantSling)
	}
}

func TestOnFormulaWithTitle(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-2\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "my-review", "code-review",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(runner.calls[0], "--title=my-review") {
		t.Errorf("wisp call = %q, want --title=my-review", runner.calls[0])
	}
	if !strings.Contains(runner.calls[0], "--on=BL-42") {
		t.Errorf("wisp call = %q, want --on=BL-42", runner.calls[0])
	}
}

func TestOnFormulaCookError(t *testing.T) {
	runner := newFakeRunner()
	runner.err["bd mol cook"] = fmt.Errorf("formula not found")
	runner.out["bd mol cook"] = ""
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "bad-formula",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "formula not found") {
		t.Errorf("stderr = %q, want formula error", stderr.String())
	}
}

func TestOnFormulaCookEmpty(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "   \n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("doSling returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already has attached wisp WP-5") {
		t.Errorf("stderr = %q, want wisp error", stderr.String())
	}
}

func TestOnFormulaCleanBead(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}
	q.childrenOf["BL-42"] = []beads.Bead{
		{ID: "STEP-1", Type: "step", Status: "open"}, // step, not molecule
	}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("got %d runner calls, want 2: %v", len(runner.calls), runner.calls)
	}
}

func TestOnFormulaNilQuerier(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	// nil querier → molecule check skipped, should succeed.
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
}

func TestOnFormulaOutput(t *testing.T) {
	runner := newFakeRunner()
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSling returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Attached wisp WP-1 (formula \"code-review\") to BL-42") {
		t.Errorf("stdout = %q, want attach message", out)
	}
	if !strings.Contains(out, "Slung BL-42 (with formula \"code-review\")") {
		t.Errorf("stdout = %q, want slung with formula message", out)
	}
}

func TestBatchOnConvoy(t *testing.T) {
	runner := newFakeRunner()
	// Return different wisp IDs for each cook call.
	callCount := 0
	countingRunner := func(command string) (string, error) {
		if strings.Contains(command, "bd mol cook") {
			callCount++
			return fmt.Sprintf("WP-%d\n", callCount), nil
		}
		return runner.run(command)
	}
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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, countingRunner, q, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	if callCount != 3 {
		t.Errorf("got %d cook calls, want 3", callCount)
	}
	out := stdout.String()
	if !strings.Contains(out, "Attached wisp WP-1") {
		t.Errorf("stdout = %q, want WP-1 attach", out)
	}
	if !strings.Contains(out, "Attached wisp WP-2") {
		t.Errorf("stdout = %q, want WP-2 attach", out)
	}
	if !strings.Contains(out, "Attached wisp WP-3") {
		t.Errorf("stdout = %q, want WP-3 attach", out)
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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	// Fail cook for BL-2, succeed for others.
	callCount := 0
	countingRunner := func(command string) (string, error) {
		if strings.Contains(command, "bd mol cook") {
			callCount++
			if strings.Contains(command, "--on=BL-2") {
				return "", fmt.Errorf("cook failed for BL-2")
			}
			return fmt.Sprintf("WP-%d\n", callCount), nil
		}
		return "", nil // sling commands succeed
	}
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
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, countingRunner, q, &stdout, &stderr)

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
	runner.out["bd mol cook"] = "WP-1\n"
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
	code := doSlingBatch(a, "CVY-1", false, true, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	runner.out["bd mol cook"] = "WP-1\n"
	sp := session.NewFake()
	cfg := &config.City{Workspace: config.Workspace{Name: "test-city"}}
	a := config.Agent{Name: "mayor"}

	q := newFakeChildQuerier()
	q.beadsByID["BL-42"] = beads.Bead{ID: "BL-42", Type: "task", Status: "open"}

	var stdout, stderr bytes.Buffer
	// Non-container bead + --on → should fall through to doSling.
	code := doSlingBatch(a, "BL-42", false, false, false, "", "code-review",
		false, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("doSlingBatch returned %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Attached wisp WP-1") {
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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "code-review", true, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		true, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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
	if !strings.Contains(out, "BL-1") && !strings.Contains(out, "would route") {
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

	var stdout, stderr bytes.Buffer
	code := doSlingBatch(a, "CVY-1", false, false, false, "", "code-review",
		true, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("dry-run returned %d, want 0; stderr: %s", code, stderr.String())
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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, true, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-1", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "code-review",
		true, "test-city", cfg, sp, runner.run, q, &stdout, &stderr)

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

	var stdout, stderr bytes.Buffer
	code := doSling(a, "BL-42", false, false, false, "", "",
		true, "test-city", cfg, sp, runner.run, nil, &stdout, &stderr)

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
