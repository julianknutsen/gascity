package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

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
	code := doSling(a, "BL-42", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "HW-7", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "code-review", true, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "code-review", true, false, false, "my-review",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, true, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, true, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "nonexistent", true, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, true, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-1", false, true, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
	code := doSling(a, "BL-99", false, false, false, "",
		"test-city", cfg, sp, runner.run, &stdout, &stderr)

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
