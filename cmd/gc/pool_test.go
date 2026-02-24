package main

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

func TestEvaluatePoolSuccess(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 0, Max: 10, ScaleCheck: "echo 5"}
	runner := func(_ string) (string, error) { return "5", nil }

	got, err := evaluatePool(pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestEvaluatePoolClampToMax(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 0, Max: 10, ScaleCheck: "echo 20"}
	runner := func(_ string) (string, error) { return "20", nil }

	got, err := evaluatePool(pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 10 {
		t.Errorf("got %d, want 10 (max)", got)
	}
}

func TestEvaluatePoolClampToMin(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 2, Max: 10, ScaleCheck: "echo 0"}
	runner := func(_ string) (string, error) { return "0", nil }

	got, err := evaluatePool(pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (min)", got)
	}
}

func TestEvaluatePoolRunnerError(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 2, Max: 10, ScaleCheck: "fail"}
	runner := func(_ string) (string, error) {
		return "", fmt.Errorf("command failed")
	}

	got, err := evaluatePool(pool, runner)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (min on error)", got)
	}
}

func TestEvaluatePoolNonInteger(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 1, Max: 10, ScaleCheck: "echo abc"}
	runner := func(_ string) (string, error) { return "abc", nil }

	got, err := evaluatePool(pool, runner)
	if err == nil {
		t.Fatal("expected error for non-integer output")
	}
	if got != 1 {
		t.Errorf("got %d, want 1 (min on error)", got)
	}
}

func TestEvaluatePoolWhitespace(t *testing.T) {
	pool := &config.Pool{Name: "worker", Min: 0, Max: 10, ScaleCheck: "echo 3"}
	runner := func(_ string) (string, error) { return " 3\n", nil }

	got, err := evaluatePool(pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestPoolAgentsNaming(t *testing.T) {
	pool := &config.Pool{
		Name:         "worker",
		StartCommand: "echo hello",
		Min:          0,
		Max:          5,
		ScaleCheck:   "echo 3",
	}
	sp := session.NewFake()
	agents, err := poolAgents(pool, 3, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp)
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("len(agents) = %d, want 3", len(agents))
	}
	want := []string{"worker-1", "worker-2", "worker-3"}
	for i, a := range agents {
		if a.Name() != want[i] {
			t.Errorf("agents[%d].Name() = %q, want %q", i, a.Name(), want[i])
		}
	}
}

func TestPoolAgentsSessionNames(t *testing.T) {
	pool := &config.Pool{
		Name:         "worker",
		StartCommand: "echo hello",
		Min:          0,
		Max:          5,
		ScaleCheck:   "echo 3",
	}
	sp := session.NewFake()
	agents, err := poolAgents(pool, 3, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp)
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	want := []string{"gc-city-worker-1", "gc-city-worker-2", "gc-city-worker-3"}
	for i, a := range agents {
		if a.SessionName() != want[i] {
			t.Errorf("agents[%d].SessionName() = %q, want %q", i, a.SessionName(), want[i])
		}
	}
}

func TestPoolAgentsZeroDesired(t *testing.T) {
	pool := &config.Pool{
		Name:         "worker",
		StartCommand: "echo hello",
		Min:          0,
		Max:          5,
		ScaleCheck:   "echo 0",
	}
	sp := session.NewFake()
	agents, err := poolAgents(pool, 0, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp)
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("len(agents) = %d, want 0", len(agents))
	}
}

func TestPoolAgentsEnv(t *testing.T) {
	pool := &config.Pool{
		Name:         "worker",
		StartCommand: "echo hello",
		Min:          0,
		Max:          5,
		ScaleCheck:   "echo 2",
		Env:          map[string]string{"POOL_VAR": "yes"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(pool, 2, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp)
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("len(agents) = %d, want 2", len(agents))
	}
	// Check that GC_AGENT is set correctly for each agent.
	cfg1 := agents[0].SessionConfig()
	if cfg1.Env["GC_AGENT"] != "worker-1" {
		t.Errorf("agent[0] GC_AGENT = %q, want %q", cfg1.Env["GC_AGENT"], "worker-1")
	}
	cfg2 := agents[1].SessionConfig()
	if cfg2.Env["GC_AGENT"] != "worker-2" {
		t.Errorf("agent[1] GC_AGENT = %q, want %q", cfg2.Env["GC_AGENT"], "worker-2")
	}
	// Check pool-level env is passed through.
	if cfg1.Env["POOL_VAR"] != "yes" {
		t.Errorf("agent[0] POOL_VAR = %q, want %q", cfg1.Env["POOL_VAR"], "yes")
	}
}

// fakeLookPath always succeeds — tests don't need real binaries.
func fakeLookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}
