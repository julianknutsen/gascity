package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
	a := agent.New("mayor", "gc-test-mayor", "echo hello", "", nil, agent.StartupHints{}, sp)

	var reconcileCount atomic.Int32
	buildFn := func() []agent.Agent {
		reconcileCount.Add(1)
		return []agent.Agent{a}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer

	// Cancel immediately after initial reconciliation completes.
	go func() {
		for reconcileCount.Load() < 1 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	controllerLoop(ctx, time.Hour, buildFn, sp, nil, events.Discard, "gc-test-", &stdout, &stderr)

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
	a := agent.New("mayor", "gc-test-mayor", "echo hello", "", nil, agent.StartupHints{}, sp)

	var reconcileCount atomic.Int32
	buildFn := func() []agent.Agent {
		reconcileCount.Add(1)
		return []agent.Agent{a}
	}

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

	controllerLoop(ctx, 10*time.Millisecond, buildFn, sp, nil, events.Discard, "gc-test-", &stdout, &stderr)

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
	a := agent.New("mayor", "gc-test-mayor", "echo hello", "", nil, agent.StartupHints{}, sp)

	buildFn := func() []agent.Agent {
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
	}

	var stdout, stderr bytes.Buffer

	// Run controller in a goroutine; it will block until canceled.
	done := make(chan int, 1)
	go func() {
		done <- runController(dir, cfg, buildFn, sp, events.Discard, &stdout, &stderr)
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
