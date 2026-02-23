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

	listErr       error // injected error for listRunning
	storeHashErr  error // injected error for storeConfigHash
	configHashErr error // injected error for configHash
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
	if f.configHashErr != nil {
		return "", f.configHashErr
	}
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents(nil, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents(nil, nil, sp, rops, "gc-city-", &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, nil, "gc-city-", &stdout, &stderr)
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

func TestDoStopOrphansListError(t *testing.T) {
	rops := newFakeReconcileOps()
	rops.listErr = fmt.Errorf("tmux not running")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	doStopOrphans(sp, rops, nil, "gc-city-", &stdout, &stderr)

	if !strings.Contains(stderr.String(), "tmux not running") {
		t.Errorf("stderr = %q, want listRunning error", stderr.String())
	}
	// No orphans stopped.
	if strings.Contains(stdout.String(), "Stopped") {
		t.Errorf("stdout should not contain stop messages: %q", stdout.String())
	}
}

func TestReconcileConfigHashErrorSkipsDrift(t *testing.T) {
	// When configHash returns an error, treat it like no hash (graceful upgrade).
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.configHashErr = fmt.Errorf("tmux env read failed")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should NOT restart — configHash error means "no hash," not "drift."
	for _, c := range f.Calls {
		if c.Method == "Stop" || c.Method == "Start" {
			t.Errorf("unexpected call: %s (configHash error should skip drift)", c.Method)
		}
	}
}

func TestReconcileStoreHashErrorNonFatal(t *testing.T) {
	// storeConfigHash fails after start — should not break the flow.
	f := agent.NewFake("mayor", "gc-city-mayor")
	rops := newFakeReconcileOps()
	rops.storeHashErr = fmt.Errorf("env write failed")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should still have been started successfully.
	if !f.Running {
		t.Error("agent not started despite storeConfigHash error")
	}
	if !strings.Contains(stdout.String(), "Started agent 'mayor'") {
		t.Errorf("stdout missing start message: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileDriftStopErrorSkipsRestart(t *testing.T) {
	// When Stop fails during drift restart, Start should NOT be attempted.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.StopErr = fmt.Errorf("session stuck")
	f.FakeSessionConfig = session.Config{Command: "claude --new"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude --old"})
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (non-fatal)", code)
	}

	if !strings.Contains(stderr.String(), "session stuck") {
		t.Errorf("stderr = %q, want stop error", stderr.String())
	}
	// Start should NOT have been called after Stop failed.
	for _, c := range f.Calls {
		if c.Method == "Start" {
			t.Error("Start called after Stop failed — should have been skipped")
		}
	}
	// City still starts.
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileListRunningError(t *testing.T) {
	// When listRunning fails, orphan cleanup is skipped but city starts.
	rops := newFakeReconcileOps()
	rops.listErr = fmt.Errorf("no tmux server")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stderr.String(), "no tmux server") {
		t.Errorf("stderr = %q, want listRunning error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileMixedStates(t *testing.T) {
	// Multiple agents: one new, one healthy, one drifted. Plus an orphan.
	newAgent := agent.NewFake("worker", "gc-city-worker")
	// Not running — should start.

	healthy := agent.NewFake("mayor", "gc-city-mayor")
	healthy.Running = true
	healthy.FakeSessionConfig = session.Config{Command: "claude"}

	drifted := agent.NewFake("builder", "gc-city-builder")
	drifted.Running = true
	drifted.FakeSessionConfig = session.Config{Command: "claude --v2"}

	rops := newFakeReconcileOps()
	// Healthy agent: hash matches.
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	// Drifted agent: hash differs.
	rops.running["gc-city-builder"] = true
	rops.hashes["gc-city-builder"] = session.ConfigFingerprint(session.Config{Command: "claude --v1"})
	// Orphan session: not in config.
	rops.running["gc-city-oldagent"] = true

	sp := session.NewFake()
	_ = sp.Start("gc-city-oldagent", session.Config{})
	sp.Calls = nil

	agents := []agent.Agent{newAgent, healthy, drifted}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(agents, nil, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	out := stdout.String()

	// New agent started.
	if !newAgent.Running {
		t.Error("worker not started")
	}
	if !strings.Contains(out, "Started agent 'worker'") {
		t.Errorf("stdout missing worker start: %q", out)
	}

	// Healthy agent untouched.
	for _, c := range healthy.Calls {
		if c.Method == "Start" || c.Method == "Stop" {
			t.Errorf("healthy agent got unexpected call: %s", c.Method)
		}
	}

	// Drifted agent restarted.
	if !strings.Contains(out, "Config changed for 'builder'") {
		t.Errorf("stdout missing drift message for builder: %q", out)
	}
	if !strings.Contains(out, "Restarted agent 'builder'") {
		t.Errorf("stdout missing restart message for builder: %q", out)
	}

	// Orphan stopped.
	if !strings.Contains(out, "Stopped orphan session 'gc-city-oldagent'") {
		t.Errorf("stdout missing orphan stop: %q", out)
	}

	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileSkipsSuspendedAgent(t *testing.T) {
	// Suspended + not running → not started.
	f := agent.NewFake("worker", "gc-city-worker")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	suspended := map[string]bool{"gc-city-worker": true}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, suspended, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should NOT have been started.
	if f.Running {
		t.Error("suspended agent was started")
	}
	for _, c := range f.Calls {
		if c.Method == "Start" {
			t.Error("Start called on suspended agent")
		}
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileStopsSuspendedRunning(t *testing.T) {
	// Suspended + running → stopped.
	f := agent.NewFake("worker", "gc-city-worker")
	f.Running = true
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	suspended := map[string]bool{"gc-city-worker": true}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, suspended, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should have been stopped.
	if f.Running {
		t.Error("suspended running agent not stopped")
	}
	if !strings.Contains(stdout.String(), "Stopped suspended agent 'worker'") {
		t.Errorf("stdout = %q, want suspended stop message", stdout.String())
	}
}

func TestReconcileSuspendedStopErrorNonFatal(t *testing.T) {
	// Suspended + running + stop fails → error reported, continues.
	f := agent.NewFake("worker", "gc-city-worker")
	f.Running = true
	f.StopErr = fmt.Errorf("session stuck")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	suspended := map[string]bool{"gc-city-worker": true}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, suspended, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (non-fatal)", code)
	}

	if !strings.Contains(stderr.String(), "session stuck") {
		t.Errorf("stderr = %q, want stop error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileMixedWithSuspended(t *testing.T) {
	// One normal agent (should start), one suspended running (should stop),
	// one suspended not running (should skip).
	normal := agent.NewFake("mayor", "gc-city-mayor")
	// Not running — should start.

	suspRunning := agent.NewFake("worker", "gc-city-worker")
	suspRunning.Running = true

	suspStopped := agent.NewFake("builder", "gc-city-builder")
	// Not running, suspended — should skip.

	rops := newFakeReconcileOps()
	rops.running["gc-city-worker"] = true
	sp := session.NewFake()

	suspended := map[string]bool{
		"gc-city-worker":  true,
		"gc-city-builder": true,
	}
	agents := []agent.Agent{normal, suspRunning, suspStopped}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(agents, suspended, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	out := stdout.String()

	// Normal agent started.
	if !normal.Running {
		t.Error("normal agent not started")
	}
	if !strings.Contains(out, "Started agent 'mayor'") {
		t.Errorf("stdout missing mayor start: %q", out)
	}

	// Suspended running agent stopped.
	if suspRunning.Running {
		t.Error("suspended running agent not stopped")
	}
	if !strings.Contains(out, "Stopped suspended agent 'worker'") {
		t.Errorf("stdout missing worker suspend stop: %q", out)
	}

	// Suspended stopped agent untouched.
	for _, c := range suspStopped.Calls {
		if c.Method == "Start" || c.Method == "Stop" {
			t.Errorf("suspended stopped agent got unexpected call: %s", c.Method)
		}
	}

	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.'")
	}
}

func TestReconcileSuspendedFalseStartsNormally(t *testing.T) {
	// An agent with suspended=false in the map should start normally
	// (false in the map is treated as not suspended).
	f := agent.NewFake("worker", "gc-city-worker")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	suspended := map[string]bool{"gc-city-worker": false}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, suspended, sp, rops, "gc-city-", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should have been started — suspended=false means not suspended.
	if !f.Running {
		t.Error("agent with suspended=false was not started")
	}
	if !strings.Contains(stdout.String(), "Started agent 'worker'") {
		t.Errorf("stdout = %q, want start message", stdout.String())
	}
}
