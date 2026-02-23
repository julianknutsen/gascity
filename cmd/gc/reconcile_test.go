package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/session"
)

// fakeReconcileOps is a test double for reconcileOps.
type fakeReconcileOps struct {
	running map[string]bool   // session names that exist
	hashes  map[string]string // stored config hashes

	listErr      error // injected error for listRunning
	storeHashErr error // injected error for storeConfigHash
}

func newFakeReconcileOps() *fakeReconcileOps {
	return &fakeReconcileOps{
		running: make(map[string]bool),
		hashes:  make(map[string]string),
	}
}

func (f *fakeReconcileOps) listRunning(prefix string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var names []string
	for name := range f.running {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, nil
}

func (f *fakeReconcileOps) storeConfigHash(name, hash string) error {
	if f.storeHashErr != nil {
		return f.storeHashErr
	}
	f.hashes[name] = hash
	return nil
}

func (f *fakeReconcileOps) configHash(name string) (string, error) {
	h, ok := f.hashes[name]
	if !ok {
		return "", nil
	}
	return h, nil
}

func TestReconcileStartsNewAgents(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Agent should have been started.
	if !f.Running {
		t.Error("agent not started")
	}
	if !strings.Contains(stdout.String(), "Started agent 'mayor'") {
		t.Errorf("stdout = %q, want start message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}

	// Config hash should have been stored.
	if rops.hashes["gc-city-mayor"] == "" {
		t.Error("config hash not stored after start")
	}
}

func TestReconcileSkipsHealthy(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	// Store the same hash that the agent's config would produce.
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should NOT have started or stopped.
	for _, c := range f.Calls {
		if c.Method == "Start" || c.Method == "Stop" {
			t.Errorf("unexpected call: %s", c.Method)
		}
	}
	if strings.Contains(stdout.String(), "Started") {
		t.Errorf("stdout should not contain 'Started': %q", stdout.String())
	}
}

func TestReconcileStopsOrphans(t *testing.T) {
	// No desired agents, but an orphan session exists.
	rops := newFakeReconcileOps()
	rops.running["gc-city-oldagent"] = true
	sp := session.NewFake()
	_ = sp.Start("gc-city-oldagent", session.Config{})
	sp.Calls = nil // reset spy

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-oldagent'") {
		t.Errorf("stdout = %q, want orphan stop message", stdout.String())
	}

	// Verify provider Stop was called.
	found := false
	for _, c := range sp.Calls {
		if c.Method == "Stop" && c.Name == "gc-city-oldagent" {
			found = true
		}
	}
	if !found {
		t.Error("provider.Stop not called for orphan")
	}
}

func TestReconcileRestartsOnDrift(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude --new-flag"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	// Store old hash (different command).
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude --old-flag"})
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should have stopped and restarted.
	var sawStop, sawStart bool
	for _, c := range f.Calls {
		if c.Method == "Stop" {
			sawStop = true
		}
		if c.Method == "Start" {
			sawStart = true
		}
	}
	if !sawStop {
		t.Error("expected Stop call for drift restart")
	}
	if !sawStart {
		t.Error("expected Start call for drift restart")
	}
	if !strings.Contains(stdout.String(), "Config changed") {
		t.Errorf("stdout missing drift message: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Restarted agent 'mayor'") {
		t.Errorf("stdout missing restart message: %q", stdout.String())
	}

	// New hash should be stored.
	expected := session.ConfigFingerprint(session.Config{Command: "claude --new-flag"})
	if rops.hashes["gc-city-mayor"] != expected {
		t.Errorf("hash after restart = %q, want %q", rops.hashes["gc-city-mayor"], expected)
	}
}

func TestReconcileNoDriftWithoutHash(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	// No stored hash — simulates graceful upgrade.
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should NOT have stopped or started.
	for _, c := range f.Calls {
		if c.Method == "Stop" || c.Method == "Start" {
			t.Errorf("unexpected call: %s (graceful upgrade should skip)", c.Method)
		}
	}
}

func TestReconcileStartErrorNonFatal(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.StartErr = fmt.Errorf("boom")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (errors are non-fatal)", code)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
	// City still starts.
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileOrphanStopErrorNonFatal(t *testing.T) {
	rops := newFakeReconcileOps()
	rops.running["gc-city-orphan"] = true
	sp := session.NewFailFake() // Stop will fail.

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (errors are non-fatal)", code)
	}
	if !strings.Contains(stderr.String(), "stopping orphan") {
		t.Errorf("stderr = %q, want orphan stop error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileNilReconcileOps(t *testing.T) {
	// When reconcileOps is nil (e.g., fake provider), should degrade gracefully.
	f := agent.NewFake("mayor", "gc-city-mayor")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, nil, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should still be started.
	if !f.Running {
		t.Error("agent not started with nil reconcileOps")
	}
	if !strings.Contains(stdout.String(), "Started agent 'mayor'") {
		t.Errorf("stdout missing start message: %q", stdout.String())
	}
}

func TestDoStopOrphans(t *testing.T) {
	rops := newFakeReconcileOps()
	rops.running["gc-city-orphan"] = true
	rops.running["gc-city-mayor"] = true
	sp := session.NewFake()
	_ = sp.Start("gc-city-orphan", session.Config{})
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil

	desired := map[string]bool{"gc-city-mayor": true}
	var stdout, stderr bytes.Buffer
	doStopOrphans(sp, rops, desired, "gc-city-", &stdout, &stderr)

	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-orphan'") {
		t.Errorf("stdout = %q, want orphan stop message", stdout.String())
	}
	// Mayor should not have been stopped.
	if strings.Contains(stdout.String(), "gc-city-mayor") {
		t.Errorf("stdout should not mention mayor: %q", stdout.String())
	}
}

func TestDoStopOrphansNilOps(t *testing.T) {
	// Should be a no-op when rops is nil.
	sp := session.NewFake()
	var stdout, stderr bytes.Buffer
	doStopOrphans(sp, nil, nil, "gc-city-", &stdout, &stderr)
	if stdout.Len() > 0 || stderr.Len() > 0 {
		t.Errorf("expected no output with nil rops, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
