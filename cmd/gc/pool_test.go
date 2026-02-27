package main

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

func TestEvaluatePoolSuccess(t *testing.T) {
	pool := config.PoolConfig{Min: 0, Max: 10, Check: "echo 5"}
	runner := func(_ string) (string, error) { return "5", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestEvaluatePoolClampToMax(t *testing.T) {
	pool := config.PoolConfig{Min: 0, Max: 10, Check: "echo 20"}
	runner := func(_ string) (string, error) { return "20", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 10 {
		t.Errorf("got %d, want 10 (max)", got)
	}
}

func TestEvaluatePoolClampToMin(t *testing.T) {
	pool := config.PoolConfig{Min: 2, Max: 10, Check: "echo 0"}
	runner := func(_ string) (string, error) { return "0", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (min)", got)
	}
}

func TestEvaluatePoolRunnerError(t *testing.T) {
	pool := config.PoolConfig{Min: 2, Max: 10, Check: "fail"}
	runner := func(_ string) (string, error) {
		return "", fmt.Errorf("command failed")
	}

	got, err := evaluatePool("worker", pool, runner)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (min on error)", got)
	}
}

func TestEvaluatePoolNonInteger(t *testing.T) {
	pool := config.PoolConfig{Min: 1, Max: 10, Check: "echo abc"}
	runner := func(_ string) (string, error) { return "abc", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err == nil {
		t.Fatal("expected error for non-integer output")
	}
	if got != 1 {
		t.Errorf("got %d, want 1 (min on error)", got)
	}
}

func TestEvaluatePoolWhitespace(t *testing.T) {
	pool := config.PoolConfig{Min: 0, Max: 10, Check: "echo 3"}
	runner := func(_ string) (string, error) { return " 3\n", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err != nil {
		t.Fatalf("evaluatePool: %v", err)
	}
	if got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

// Regression: empty check output must be an error, not silent success.
func TestEvaluatePoolEmptyOutput(t *testing.T) {
	pool := config.PoolConfig{Min: 2, Max: 10, Check: "true"}
	runner := func(_ string) (string, error) { return "", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if got != 2 {
		t.Errorf("got %d, want 2 (min on error)", got)
	}
}

// Regression: whitespace-only output should also be treated as empty.
func TestEvaluatePoolWhitespaceOnly(t *testing.T) {
	pool := config.PoolConfig{Min: 1, Max: 10, Check: "echo"}
	runner := func(_ string) (string, error) { return "  \n", nil }

	got, err := evaluatePool("worker", pool, runner)
	if err == nil {
		t.Fatal("expected error for whitespace-only output")
	}
	if got != 1 {
		t.Errorf("got %d, want 1 (min on error)", got)
	}
}

func TestPoolAgentsNaming(t *testing.T) {
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 5, Check: "echo 3"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 3, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
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
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 5, Check: "echo 3"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 3, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
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
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 5, Check: "echo 0"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 0, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("len(agents) = %d, want 0", len(agents))
	}
}

func TestPoolAgentsEnv(t *testing.T) {
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 5, Check: "echo 2"},
		Env:          map[string]string{"POOL_VAR": "yes"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 2, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
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

func TestPoolAgentsMaxOneNoSuffix(t *testing.T) {
	// When max == 1, the agent should use the bare name (no -1 suffix).
	cfgAgent := &config.Agent{
		Name:         "refinery",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 1, Check: "echo 1"},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 1, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(agents))
	}
	if agents[0].Name() != "refinery" {
		t.Errorf("Name() = %q, want %q (bare name, no suffix)", agents[0].Name(), "refinery")
	}
	if agents[0].SessionName() != "gc-city-refinery" {
		t.Errorf("SessionName() = %q, want %q", agents[0].SessionName(), "gc-city-refinery")
	}
}

func TestPoolAgentsWorktreeIsolation(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	cfgAgent := &config.Agent{
		Name:         "worker",
		Dir:          repo,
		Isolation:    "worktree",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 3, Check: "echo 2"},
	}
	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 2, "city", cityPath,
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, rigs, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("len(agents) = %d, want 2", len(agents))
	}

	// Each instance should have its own worktree with GC_BRANCH set.
	for i, a := range agents {
		cfg := a.SessionConfig()
		if cfg.Env["GC_RIG"] != "my-rig" {
			t.Errorf("agent[%d] GC_RIG = %q, want %q", i, cfg.Env["GC_RIG"], "my-rig")
		}
		if cfg.Env["GC_BRANCH"] == "" {
			t.Errorf("agent[%d] GC_BRANCH is empty, want worktree branch", i)
		}
		// WorkDir should be the worktree path, not the repo.
		if cfg.WorkDir == repo {
			t.Errorf("agent[%d] WorkDir = repo, want worktree path", i)
		}
	}

	// The two instances should have different worktree paths.
	cfg1 := agents[0].SessionConfig()
	cfg2 := agents[1].SessionConfig()
	if cfg1.WorkDir == cfg2.WorkDir {
		t.Errorf("instances have same WorkDir %q, want different", cfg1.WorkDir)
	}
}

// ---------------------------------------------------------------------------
// Session setup template expansion tests
// ---------------------------------------------------------------------------

func TestExpandSessionSetup_Basic(t *testing.T) {
	ctx := SessionSetupContext{
		Session:  "gc-city-mayor",
		Agent:    "mayor",
		Rig:      "",
		CityRoot: "/home/user/city",
		CityName: "bright-lights",
		WorkDir:  "/home/user/city",
	}
	cmds := []string{
		"tmux set-option -t {{.Session}} status-style 'bg=blue'",
		"tmux set-option -t {{.Session}} status-left ' {{.Agent}} '",
	}
	got := expandSessionSetup(cmds, ctx)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != "tmux set-option -t gc-city-mayor status-style 'bg=blue'" {
		t.Errorf("cmd[0] = %q", got[0])
	}
	if got[1] != "tmux set-option -t gc-city-mayor status-left ' mayor '" {
		t.Errorf("cmd[1] = %q", got[1])
	}
}

func TestExpandSessionSetup_AllVariables(t *testing.T) {
	ctx := SessionSetupContext{
		Session:  "gc-bl-hw--polecat",
		Agent:    "hw/polecat",
		Rig:      "hello-world",
		CityRoot: "/city",
		CityName: "bl",
		WorkDir:  "/city/.gc/worktrees/polecat",
	}
	cmds := []string{
		"echo {{.Session}} {{.Agent}} {{.Rig}} {{.CityRoot}} {{.CityName}} {{.WorkDir}}",
	}
	got := expandSessionSetup(cmds, ctx)
	want := "echo gc-bl-hw--polecat hw/polecat hello-world /city bl /city/.gc/worktrees/polecat"
	if got[0] != want {
		t.Errorf("got %q, want %q", got[0], want)
	}
}

func TestExpandSessionSetup_InvalidTemplate(t *testing.T) {
	ctx := SessionSetupContext{Session: "test"}
	cmds := []string{
		"tmux {{.Session}}",    // valid
		"tmux {{.BadSyntax",    // invalid template
		"tmux {{.Session}} ok", // valid
	}
	got := expandSessionSetup(cmds, ctx)
	if got[0] != "tmux test" {
		t.Errorf("cmd[0] = %q, want expanded", got[0])
	}
	// Invalid template → raw command preserved.
	if got[1] != "tmux {{.BadSyntax" {
		t.Errorf("cmd[1] = %q, want raw (fallback)", got[1])
	}
	if got[2] != "tmux test ok" {
		t.Errorf("cmd[2] = %q, want expanded", got[2])
	}
}

func TestExpandSessionSetup_Nil(t *testing.T) {
	got := expandSessionSetup(nil, SessionSetupContext{})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestExpandSessionSetup_Empty(t *testing.T) {
	got := expandSessionSetup([]string{}, SessionSetupContext{})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestResolveSetupScript_Relative(t *testing.T) {
	got := resolveSetupScript("scripts/setup.sh", "/home/user/city")
	if got != "/home/user/city/scripts/setup.sh" {
		t.Errorf("got %q, want absolute path", got)
	}
}

func TestResolveSetupScript_Absolute(t *testing.T) {
	got := resolveSetupScript("/usr/local/bin/setup.sh", "/home/user/city")
	if got != "/usr/local/bin/setup.sh" {
		t.Errorf("got %q, want unchanged absolute path", got)
	}
}

func TestResolveSetupScript_Empty(t *testing.T) {
	got := resolveSetupScript("", "/home/user/city")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestPoolAgentsSessionSetup(t *testing.T) {
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 1, Check: "echo 1"},
		SessionSetup: []string{
			"tmux set-option -t {{.Session}} status-left ' {{.Agent}} '",
		},
		SessionSetupScript: "scripts/setup.sh",
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 1, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(agents))
	}
	cfg := agents[0].SessionConfig()

	// Template should be expanded with session name.
	if len(cfg.SessionSetup) != 1 {
		t.Fatalf("SessionSetup len = %d, want 1", len(cfg.SessionSetup))
	}
	want := "tmux set-option -t gc-city-worker status-left ' worker '"
	if cfg.SessionSetup[0] != want {
		t.Errorf("SessionSetup[0] = %q, want %q", cfg.SessionSetup[0], want)
	}

	// Script should be resolved to absolute path.
	if cfg.SessionSetupScript != "/tmp/city/scripts/setup.sh" {
		t.Errorf("SessionSetupScript = %q, want %q", cfg.SessionSetupScript, "/tmp/city/scripts/setup.sh")
	}
}

func TestExpandSessionSetup_ConfigDir(t *testing.T) {
	ctx := SessionSetupContext{
		Session:   "gc-city-mayor",
		Agent:     "mayor",
		CityRoot:  "/home/user/city",
		CityName:  "bright-lights",
		WorkDir:   "/home/user/city",
		ConfigDir: "/home/user/city/topologies/gastown",
	}
	cmds := []string{
		"{{.ConfigDir}}/scripts/status-line.sh {{.Agent}}",
	}
	got := expandSessionSetup(cmds, ctx)
	want := "/home/user/city/topologies/gastown/scripts/status-line.sh mayor"
	if got[0] != want {
		t.Errorf("got %q, want %q", got[0], want)
	}
}

func TestPoolAgentsConfigDir(t *testing.T) {
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 1, Check: "echo 1"},
		SourceDir:    "/city/topologies/gt",
		SessionSetup: []string{
			"{{.ConfigDir}}/scripts/setup.sh {{.Agent}}",
		},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 1, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(agents))
	}
	cfg := agents[0].SessionConfig()
	// ConfigDir should use SourceDir, not CityRoot.
	want := "/city/topologies/gt/scripts/setup.sh worker"
	if len(cfg.SessionSetup) != 1 || cfg.SessionSetup[0] != want {
		t.Errorf("SessionSetup = %v, want [%q]", cfg.SessionSetup, want)
	}
}

func TestPoolAgentsConfigDir_DefaultsToCityPath(t *testing.T) {
	cfgAgent := &config.Agent{
		Name:         "worker",
		StartCommand: "echo hello",
		Pool:         &config.PoolConfig{Min: 0, Max: 1, Check: "echo 1"},
		SessionSetup: []string{
			"{{.ConfigDir}}/scripts/setup.sh",
		},
	}
	sp := session.NewFake()
	agents, err := poolAgents(cfgAgent, 1, "city", "/tmp/city",
		&config.Workspace{Name: "city"}, nil, fakeLookPath, fsys.NewFake(), sp, nil, "", config.FormulaLayers{})
	if err != nil {
		t.Fatalf("poolAgents: %v", err)
	}
	cfg := agents[0].SessionConfig()
	// No SourceDir → ConfigDir defaults to cityPath.
	want := "/tmp/city/scripts/setup.sh"
	if len(cfg.SessionSetup) != 1 || cfg.SessionSetup[0] != want {
		t.Errorf("SessionSetup = %v, want [%q]", cfg.SessionSetup, want)
	}
}

// fakeLookPath always succeeds — tests don't need real binaries.
func fakeLookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}
