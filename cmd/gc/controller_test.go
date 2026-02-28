package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

func TestControllerLoopCancel(t *testing.T) {
	sp := session.NewFake()
	a := agent.New("mayor", "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp)

	var reconcileCount atomic.Int32
	buildFn := func(_ *config.City) []agent.Agent {
		reconcileCount.Add(1)
		return []agent.Agent{a}
	}

	cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer

	// Cancel immediately after initial reconciliation completes.
	go func() {
		for reconcileCount.Load() < 1 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	controllerLoop(ctx, time.Hour, cfg, "test", "", nil, buildFn, sp, nil, nil, nil, nil, nil, nil, events.Discard, "gc-test-", nil, nil, &stdout, &stderr)

	if reconcileCount.Load() < 1 {
		t.Error("expected at least one reconciliation")
	}
	// Agent should have been started by reconciliation.
	if !sp.IsRunning("gc-test-mayor") {
		t.Error("agent should be running after initial reconcile")
	}
}

func TestControllerLoopTick(t *testing.T) {
	sp := session.NewFake()
	a := agent.New("mayor", "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp)

	var reconcileCount atomic.Int32
	buildFn := func(_ *config.City) []agent.Agent {
		reconcileCount.Add(1)
		return []agent.Agent{a}
	}

	cfg := &config.City{Workspace: config.Workspace{Name: "test"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer

	// Use a very short interval so the tick fires quickly.
	go func() {
		for reconcileCount.Load() < 2 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	controllerLoop(ctx, 10*time.Millisecond, cfg, "test", "", nil, buildFn, sp, nil, nil, nil, nil, nil, nil, events.Discard, "gc-test-", nil, nil, &stdout, &stderr)

	if got := reconcileCount.Load(); got < 2 {
		t.Errorf("reconcile count = %d, want >= 2", got)
	}
}

func TestControllerLockExclusion(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// First lock should succeed.
	lock1, err := acquireControllerLock(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer lock1.Close() //nolint:errcheck // test cleanup

	// Second lock should fail.
	_, err = acquireControllerLock(dir)
	if err == nil {
		t.Fatal("expected error for second lock, got nil")
	}
}

func TestControllerShutdown(t *testing.T) {
	sp := session.NewFake()
	// Pre-start an agent to verify shutdown stops it.
	_ = sp.Start("gc-test-mayor", session.Config{Command: "echo hello"})
	a := agent.New("mayor", "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp)

	buildFn := func(_ *config.City) []agent.Agent {
		return []agent.Agent{a}
	}

	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents:    []config.Agent{{Name: "mayor", StartCommand: "echo hello"}},
		Daemon:    config.DaemonConfig{ShutdownTimeout: "0s"},
	}

	var stdout, stderr bytes.Buffer

	// Run controller in a goroutine; it will block until canceled.
	done := make(chan int, 1)
	go func() {
		done <- runController(dir, "", cfg, buildFn, sp, nil, nil, nil, events.Discard, &stdout, &stderr)
	}()

	// Wait for controller to start, then send stop via socket.
	time.Sleep(100 * time.Millisecond)
	if !tryStopController(dir, &bytes.Buffer{}) {
		t.Fatal("tryStopController returned false, expected true")
	}

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("runController exit code = %d, want 0; stderr: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runController did not exit after stop")
	}

	// Agent should have been stopped during shutdown.
	if sp.IsRunning("gc-test-mayor") {
		t.Error("agent should be stopped after controller shutdown")
	}
}

// writeCityTOML is a test helper that writes a city.toml with the given agents.
func writeCityTOML(t *testing.T, dir string, cityName string, agentNames ...string) string {
	t.Helper()
	tomlPath := filepath.Join(dir, "city.toml")
	var buf bytes.Buffer
	buf.WriteString("[workspace]\nname = " + `"` + cityName + `"` + "\n\n")
	for _, name := range agentNames {
		buf.WriteString("[[agents]]\nname = " + `"` + name + `"` + "\n")
		buf.WriteString("start_command = \"echo hello\"\n\n")
	}
	if err := os.WriteFile(tomlPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return tomlPath
}

func TestControllerReloadsConfig(t *testing.T) {
	old := debounceDelay
	debounceDelay = 5 * time.Millisecond
	t.Cleanup(func() { debounceDelay = old })

	dir := t.TempDir()
	tomlPath := writeCityTOML(t, dir, "test", "mayor")

	cfg, err := config.Load(osFS{}, tomlPath)
	if err != nil {
		t.Fatal(err)
	}

	sp := session.NewFake()

	// buildFn creates agents from the config it receives.
	var lastAgentNames atomic.Value
	var reconcileCount atomic.Int32
	buildFn := func(c *config.City) []agent.Agent {
		reconcileCount.Add(1)
		var names []string
		var agents []agent.Agent
		for _, a := range c.Agents {
			names = append(names, a.Name)
			agents = append(agents, agent.New(a.Name, "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp))
		}
		lastAgentNames.Store(names)
		return agents
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer

	go controllerLoop(ctx, 20*time.Millisecond, cfg, "test", tomlPath, nil,
		buildFn, sp, nil, nil, nil, nil, nil, nil, events.Discard, "gc-test-", nil, nil, &stdout, &stderr)

	// Wait for initial reconcile.
	for reconcileCount.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Overwrite city.toml with a new agent.
	writeCityTOML(t, dir, "test", "mayor", "worker")

	// Wait for at least one more reconcile after the file change.
	target := reconcileCount.Load() + 2 // +2 to be safe (need tick after dirty flag)
	deadline := time.After(3 * time.Second)
	for reconcileCount.Load() < target {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for config reload")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()

	if !strings.Contains(stdout.String(), "Config reloaded") {
		t.Errorf("expected 'Config reloaded' in stdout, got: %s", stdout.String())
	}

	names, _ := lastAgentNames.Load().([]string)
	if len(names) != 2 || names[0] != "mayor" || names[1] != "worker" {
		t.Errorf("expected [mayor worker], got %v", names)
	}
}

func TestControllerReloadInvalidConfig(t *testing.T) {
	old := debounceDelay
	debounceDelay = 5 * time.Millisecond
	t.Cleanup(func() { debounceDelay = old })

	dir := t.TempDir()
	tomlPath := writeCityTOML(t, dir, "test", "mayor")

	cfg, err := config.Load(osFS{}, tomlPath)
	if err != nil {
		t.Fatal(err)
	}

	sp := session.NewFake()
	var reconcileCount atomic.Int32
	buildFn := func(c *config.City) []agent.Agent {
		reconcileCount.Add(1)
		var agents []agent.Agent
		for _, a := range c.Agents {
			agents = append(agents, agent.New(a.Name, "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp))
		}
		return agents
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer

	go controllerLoop(ctx, 20*time.Millisecond, cfg, "test", tomlPath, nil,
		buildFn, sp, nil, nil, nil, nil, nil, nil, events.Discard, "gc-test-", nil, nil, &stdout, &stderr)

	// Wait for initial reconcile.
	for reconcileCount.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Write invalid TOML.
	if err := os.WriteFile(tomlPath, []byte("[[[ bad toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for a tick to process the bad config.
	target := reconcileCount.Load() + 2
	deadline := time.After(3 * time.Second)
	for reconcileCount.Load() < target {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for tick after invalid config")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()

	if !strings.Contains(stderr.String(), "config reload") {
		t.Errorf("expected config reload error in stderr, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Config reloaded.") {
		t.Error("should not have reloaded invalid config")
	}
}

func TestControllerReloadCityNameChange(t *testing.T) {
	old := debounceDelay
	debounceDelay = 5 * time.Millisecond
	t.Cleanup(func() { debounceDelay = old })

	dir := t.TempDir()
	tomlPath := writeCityTOML(t, dir, "test", "mayor")

	cfg, err := config.Load(osFS{}, tomlPath)
	if err != nil {
		t.Fatal(err)
	}

	sp := session.NewFake()
	var reconcileCount atomic.Int32
	buildFn := func(c *config.City) []agent.Agent {
		reconcileCount.Add(1)
		var agents []agent.Agent
		for _, a := range c.Agents {
			agents = append(agents, agent.New(a.Name, "test", "echo hello", "", nil, agent.StartupHints{}, "", "", nil, sp))
		}
		return agents
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer

	go controllerLoop(ctx, 20*time.Millisecond, cfg, "test", tomlPath, nil,
		buildFn, sp, nil, nil, nil, nil, nil, nil, events.Discard, "gc-test-", nil, nil, &stdout, &stderr)

	// Wait for initial reconcile.
	for reconcileCount.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Change the city name.
	writeCityTOML(t, dir, "different-city", "mayor")

	// Wait for tick.
	target := reconcileCount.Load() + 2
	deadline := time.After(3 * time.Second)
	for reconcileCount.Load() < target {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for tick after name change")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()

	if !strings.Contains(stderr.String(), "workspace.name changed") {
		t.Errorf("expected workspace.name change rejection in stderr, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Config reloaded.") {
		t.Error("should not have reloaded config with changed city name")
	}
}

// osFS is a minimal fsys.FS for test helpers that delegates to the os package.
type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error)                 { return os.ReadFile(name) }
func (osFS) WriteFile(name string, d []byte, p os.FileMode) error { return os.WriteFile(name, d, p) }
func (osFS) MkdirAll(path string, perm os.FileMode) error         { return os.MkdirAll(path, perm) }
func (osFS) Stat(name string) (os.FileInfo, error)                { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)           { return os.ReadDir(name) }
func (osFS) Rename(oldpath, newpath string) error                 { return os.Rename(oldpath, newpath) }
