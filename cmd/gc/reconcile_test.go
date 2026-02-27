package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (errors are non-fatal)", code)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
	// City still starts.
}

func TestReconcileOrphanStopErrorNonFatal(t *testing.T) {
	rops := newFakeReconcileOps()
	rops.running["gc-city-orphan"] = true
	sp := session.NewFailFake() // Stop will fail.

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (errors are non-fatal)", code)
	}
	if !strings.Contains(stderr.String(), "stopping orphan") {
		t.Errorf("stderr = %q, want orphan stop error", stderr.String())
	}
}

func TestReconcileNilReconcileOps(t *testing.T) {
	// When reconcileOps is nil (e.g., fake provider), should degrade gracefully.
	f := agent.NewFake("mayor", "gc-city-mayor")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, nil, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	doStopOrphans(sp, rops, desired, "gc-city-", 0, events.Discard, &stdout, &stderr)

	if !strings.Contains(stdout.String(), "Stopped agent 'gc-city-orphan'") {
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
	doStopOrphans(sp, nil, nil, "gc-city-", 0, events.Discard, &stdout, &stderr)
	if stdout.Len() > 0 || stderr.Len() > 0 {
		t.Errorf("expected no output with nil rops, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDoStopOrphansListError(t *testing.T) {
	rops := newFakeReconcileOps()
	rops.listErr = fmt.Errorf("tmux not running")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	doStopOrphans(sp, rops, nil, "gc-city-", 0, events.Discard, &stdout, &stderr)

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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
}

func TestReconcileListRunningError(t *testing.T) {
	// When listRunning fails, orphan cleanup is skipped but city starts.
	rops := newFakeReconcileOps()
	rops.listErr = fmt.Errorf("no tmux server")
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stderr.String(), "no tmux server") {
		t.Errorf("stderr = %q, want listRunning error", stderr.String())
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
	code := doReconcileAgents(agents, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
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
}

func TestReconcileRecordsStartEvent(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	rops := newFakeReconcileOps()
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if len(rec.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(rec.Events))
	}
	e := rec.Events[0]
	if e.Type != events.AgentStarted {
		t.Errorf("event type = %q, want %q", e.Type, events.AgentStarted)
	}
	if e.Actor != "gc" {
		t.Errorf("event actor = %q, want %q", e.Actor, "gc")
	}
	if e.Subject != "mayor" {
		t.Errorf("event subject = %q, want %q", e.Subject, "mayor")
	}
}

func TestReconcileRecordsEventOnDriftRestart(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude --new"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude --old"})
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if len(rec.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(rec.Events))
	}
	e := rec.Events[0]
	if e.Type != events.AgentStarted {
		t.Errorf("event type = %q, want %q", e.Type, events.AgentStarted)
	}
	if e.Subject != "mayor" {
		t.Errorf("event subject = %q, want %q", e.Subject, "mayor")
	}
}

func TestReconcileNoEventOnSkip(t *testing.T) {
	// Healthy agent — no start/stop, so no events.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if len(rec.Events) != 0 {
		t.Errorf("got %d events, want 0 (healthy skip should not record)", len(rec.Events))
	}
}

func TestReconcileNoEventOnStartError(t *testing.T) {
	// Start fails — no event should be recorded.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.StartErr = fmt.Errorf("boom")
	rops := newFakeReconcileOps()
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)

	if len(rec.Events) != 0 {
		t.Errorf("got %d events, want 0 (failed start should not record)", len(rec.Events))
	}
}

// ---------------------------------------------------------------------------
// Zombie crash capture tests
// ---------------------------------------------------------------------------

func TestReconcileZombieCaptureEmitsEvent(t *testing.T) {
	// Agent thinks it's dead, but tmux session still exists (zombie).
	// Reconcile should peek at the pane output and emit agent.crashed.
	f := agent.NewFake("worker", "gc-city-worker")
	f.Running = false // agent process dead

	sp := session.NewFake()
	_ = sp.Start("gc-city-worker", session.Config{}) // tmux session alive
	sp.SetPeekOutput("gc-city-worker", "panic: runtime error: index out of range\ngoroutine 1 [running]:")
	sp.Calls = nil // reset spy

	rops := newFakeReconcileOps()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)

	// Should have emitted an agent.crashed event with the pane output.
	var crashEvent *events.Event
	for i := range rec.Events {
		if rec.Events[i].Type == events.AgentCrashed {
			crashEvent = &rec.Events[i]
			break
		}
	}
	if crashEvent == nil {
		t.Fatal("expected agent.crashed event, got none")
	}
	if crashEvent.Subject != "worker" {
		t.Errorf("crash event subject = %q, want %q", crashEvent.Subject, "worker")
	}
	if !strings.Contains(crashEvent.Message, "panic: runtime error") {
		t.Errorf("crash event message = %q, want panic output", crashEvent.Message)
	}

	// Agent should still have been started after capture.
	if !f.Running {
		t.Error("agent not started after zombie capture")
	}
}

func TestReconcileNoZombieEventWhenSessionMissing(t *testing.T) {
	// Agent is dead and no tmux session exists (clean start, not a zombie).
	// No agent.crashed event should be emitted.
	f := agent.NewFake("worker", "gc-city-worker")
	f.Running = false // agent process dead
	// No sp.Start — session doesn't exist.

	sp := session.NewFake()
	rops := newFakeReconcileOps()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)

	// No agent.crashed event.
	for _, e := range rec.Events {
		if e.Type == events.AgentCrashed {
			t.Errorf("unexpected agent.crashed event: %+v", e)
		}
	}

	// Agent should still have been started.
	if !f.Running {
		t.Error("agent not started")
	}
}

func TestReconcileZombieEmptyPeekIgnored(t *testing.T) {
	// Zombie exists but peek returns empty output — no event emitted.
	f := agent.NewFake("worker", "gc-city-worker")
	f.Running = false

	sp := session.NewFake()
	_ = sp.Start("gc-city-worker", session.Config{}) // zombie session
	// No SetPeekOutput — defaults to empty string.
	sp.Calls = nil

	rops := newFakeReconcileOps()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)

	// No agent.crashed event for empty output.
	for _, e := range rec.Events {
		if e.Type == events.AgentCrashed {
			t.Errorf("unexpected agent.crashed event for empty peek: %+v", e)
		}
	}

	// Agent should still have been started.
	if !f.Running {
		t.Error("agent not started")
	}
}

// ---------------------------------------------------------------------------
// newReconcileOps factory tests
// ---------------------------------------------------------------------------

func TestNewReconcileOpsAlwaysReturnsNonNil(t *testing.T) {
	// newReconcileOps works with any Provider — no type assertions.
	fp := session.NewFake()
	rops := newReconcileOps(fp)
	if rops == nil {
		t.Fatal("newReconcileOps(Fake) = nil, want non-nil")
	}
	if _, ok := rops.(*providerReconcileOps); !ok {
		t.Errorf("newReconcileOps returned %T, want *providerReconcileOps", rops)
	}
}

func TestProviderReconcileOpsRoundTrip(t *testing.T) {
	// Verify reconcile ops work through Provider meta/list interface.
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	_ = sp.Start("gc-city-worker", session.Config{})
	_ = sp.Start("gc-other-agent", session.Config{})
	rops := newReconcileOps(sp)

	// listRunning with prefix filter.
	names, err := rops.listRunning("gc-city-")
	if err != nil {
		t.Fatalf("listRunning: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("listRunning = %v, want 2 sessions with gc-city- prefix", names)
	}

	// Store and retrieve config hash.
	if err := rops.storeConfigHash("gc-city-mayor", "abc123"); err != nil {
		t.Fatalf("storeConfigHash: %v", err)
	}
	hash, err := rops.configHash("gc-city-mayor")
	if err != nil {
		t.Fatalf("configHash: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("configHash = %q, want %q", hash, "abc123")
	}

	// No hash returns empty.
	hash, _ = rops.configHash("gc-city-worker")
	if hash != "" {
		t.Errorf("configHash for unset = %q, want empty", hash)
	}
}

// ---------------------------------------------------------------------------
// Drain-aware reconciliation tests
// ---------------------------------------------------------------------------

func TestReconcileDrainsExcessPool(t *testing.T) {
	// 2 desired workers, 3 running; worker-3 is in poolSessions → drain, not kill.
	w1 := agent.NewFake("worker-1", "gc-city-worker-1")
	w1.Running = true
	w1.FakeSessionConfig = session.Config{Command: "claude"}
	w2 := agent.NewFake("worker-2", "gc-city-worker-2")
	w2.Running = true
	w2.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-1"] = true
	rops.running["gc-city-worker-2"] = true
	rops.running["gc-city-worker-3"] = true
	rops.hashes["gc-city-worker-1"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	rops.hashes["gc-city-worker-2"] = session.ConfigFingerprint(session.Config{Command: "claude"})

	dops := newFakeDrainOps()
	poolSessions := map[string]time.Duration{
		"gc-city-worker-1": 5 * time.Minute,
		"gc-city-worker-2": 5 * time.Minute,
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	agents := []agent.Agent{w1, w2}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(agents, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	// worker-3 should have been drained, not killed.
	if !dops.draining["gc-city-worker-3"] {
		t.Error("worker-3 not drained")
	}
	if strings.Contains(stdout.String(), "Stopped orphan") {
		t.Errorf("should not contain orphan stop: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Draining 'gc-city-worker-3' (scaling down)") {
		t.Errorf("stdout missing drain message: %q", stdout.String())
	}
	// provider.Stop should NOT have been called for worker-3.
	for _, c := range sp.Calls {
		if c.Method == "Stop" && c.Name == "gc-city-worker-3" {
			t.Error("provider.Stop called for pool member — should have been drained")
		}
	}
}

func TestReconcileKillsTrueOrphan(t *testing.T) {
	// Unknown session not in poolSessions → killed (existing behavior).
	rops := newFakeReconcileOps()
	rops.running["gc-city-unknown"] = true
	dops := newFakeDrainOps()
	poolSessions := map[string]time.Duration{
		"gc-city-worker-1": 5 * time.Minute,
		"gc-city-worker-2": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-unknown", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-unknown'") {
		t.Errorf("stdout = %q, want orphan stop message", stdout.String())
	}
	// Should NOT have been drained.
	if dops.draining["gc-city-unknown"] {
		t.Error("true orphan should be killed, not drained")
	}
}

func TestReconcileAlreadyDraining(t *testing.T) {
	// worker-3 already draining → no duplicate setDrain call.
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true // already draining
	poolSessions := map[string]time.Duration{
		"gc-city-worker-1": 5 * time.Minute,
		"gc-city-worker-2": 5 * time.Minute,
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// setDrain should NOT have been called (already draining).
	if len(dops.setDrainCalls) != 0 {
		t.Errorf("setDrain called %d times, want 0 (already draining)", len(dops.setDrainCalls))
	}
	// No drain message in stdout (silent skip).
	if strings.Contains(stdout.String(), "Draining") {
		t.Errorf("stdout should not contain drain message for already-draining: %q", stdout.String())
	}
}

func TestReconcileDrainAckReap(t *testing.T) {
	// worker-3 is draining AND ack'd → controller stops the session.
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true
	dops.acked["gc-city-worker-3"] = true
	poolSessions := map[string]time.Duration{
		"gc-city-worker-1": 5 * time.Minute,
		"gc-city-worker-2": 5 * time.Minute,
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Session should have been stopped.
	if sp.IsRunning("gc-city-worker-3") {
		t.Error("worker-3 should have been stopped after drain ack")
	}
	if !strings.Contains(stdout.String(), "Stopped drained session 'gc-city-worker-3'") {
		t.Errorf("stdout = %q, want drained stop message", stdout.String())
	}
}

func TestReconcileUndrainOnScaleUp(t *testing.T) {
	// worker-3 is draining but is now in the desired set → undrain.
	w3 := agent.NewFake("worker-3", "gc-city-worker-3")
	w3.Running = true
	w3.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	rops.hashes["gc-city-worker-3"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true // was draining
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{w3}, sp, rops, dops, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// clearDrain should have been called.
	if len(dops.clearDrainCalls) != 1 || dops.clearDrainCalls[0] != "gc-city-worker-3" {
		t.Errorf("clearDrainCalls = %v, want [gc-city-worker-3]", dops.clearDrainCalls)
	}
	// Should no longer be draining.
	if dops.draining["gc-city-worker-3"] {
		t.Error("worker-3 still draining after undrain")
	}
}

func TestReconcileNilDrainOpsFallback(t *testing.T) {
	// dops=nil → excess pool members are killed (fallback to old behavior).
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	poolSessions := map[string]time.Duration{
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// With nil dops, pool members fall through to the kill path.
	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-worker-3'") {
		t.Errorf("stdout = %q, want orphan stop (nil dops fallback)", stdout.String())
	}
}

func TestReconcilePoolSessionsNil(t *testing.T) {
	// poolSessions=nil → all non-desired sessions killed (backward compat).
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// poolSessions[name] is zero-value when map is nil → kill path.
	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-worker-3'") {
		t.Errorf("stdout = %q, want orphan stop (nil poolSessions)", stdout.String())
	}
}

func TestReconcileDrainTimeout(t *testing.T) {
	// worker-3 is draining but NOT acked, and drain started long ago → force-kill.
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true
	dops.drainTimes["gc-city-worker-3"] = time.Now().Add(-10 * time.Minute) // started 10m ago
	poolSessions := map[string]time.Duration{
		"gc-city-worker-1": 5 * time.Minute,
		"gc-city-worker-2": 5 * time.Minute,
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Session should have been force-killed.
	if sp.IsRunning("gc-city-worker-3") {
		t.Error("worker-3 should have been force-killed after drain timeout")
	}
	if !strings.Contains(stdout.String(), "Killed drained session 'gc-city-worker-3' (timeout after 5m0s)") {
		t.Errorf("stdout = %q, want timeout kill message", stdout.String())
	}
}

func TestReconcileDrainNotTimedOut(t *testing.T) {
	// worker-3 is draining but NOT acked, drain started recently → still winding down.
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true
	dops.drainTimes["gc-city-worker-3"] = time.Now().Add(-1 * time.Minute) // started 1m ago
	poolSessions := map[string]time.Duration{
		"gc-city-worker-3": 5 * time.Minute,
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Session should still be running — not timed out yet.
	if !sp.IsRunning("gc-city-worker-3") {
		t.Error("worker-3 should still be running (not timed out)")
	}
	if strings.Contains(stdout.String(), "Killed") {
		t.Errorf("stdout should not contain kill message: %q", stdout.String())
	}
}

func TestReconcileDrainTimeoutZero(t *testing.T) {
	// drainTimeout=0 → no timeout enforcement (backward compat).
	rops := newFakeReconcileOps()
	rops.running["gc-city-worker-3"] = true
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker-3"] = true
	dops.drainTimes["gc-city-worker-3"] = time.Now().Add(-1 * time.Hour) // long ago
	poolSessions := map[string]time.Duration{
		"gc-city-worker-3": 0, // no timeout
	}
	sp := session.NewFake()
	_ = sp.Start("gc-city-worker-3", session.Config{})
	sp.Calls = nil

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, dops, nil, nil, events.Discard, "gc-city-", poolSessions, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should still be running — zero timeout means no enforcement.
	if !sp.IsRunning("gc-city-worker-3") {
		t.Error("worker-3 should still be running (zero timeout)")
	}
	if strings.Contains(stdout.String(), "Killed") {
		t.Errorf("stdout should not contain kill message: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Crash tracker / quarantine reconciliation tests
// ---------------------------------------------------------------------------

func TestReconcileQuarantineSkipsStart(t *testing.T) {
	// Agent is quarantined → reconciler skips start silently.
	f := agent.NewFake("mayor", "gc-city-mayor")
	ct := newFakeCrashTracker()
	ct.quarantined["gc-city-mayor"] = true // pre-quarantined

	rops := newFakeReconcileOps()
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, ct, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should NOT have been started.
	if f.Running {
		t.Error("quarantined agent should not be started")
	}
	if strings.Contains(stdout.String(), "Started") {
		t.Errorf("stdout should not contain 'Started': %q", stdout.String())
	}
	// No events (quarantine event was emitted earlier, not on skip).
	if len(rec.Events) != 0 {
		t.Errorf("got %d events, want 0 (quarantine skip should not record)", len(rec.Events))
	}
}

func TestReconcileNilCrashTrackerNoQuarantine(t *testing.T) {
	// nil crash tracker → agent starts normally (backward compat).
	f := agent.NewFake("mayor", "gc-city-mayor")
	rops := newFakeReconcileOps()
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !f.Running {
		t.Error("agent should be started with nil crash tracker")
	}
}

func TestReconcileRecordsStartAndQuarantine(t *testing.T) {
	// Agent below threshold → starts normally. Hitting threshold emits quarantine event.
	f := agent.NewFake("mayor", "gc-city-mayor")
	// Use a real tracker with threshold=1 so the first start triggers quarantine.
	ct := newCrashTracker(1, time.Hour)
	rops := newFakeReconcileOps()
	sp := session.NewFake()
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, ct, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent was started (last chance).
	if !f.Running {
		t.Error("agent should be started (Nth start succeeds)")
	}

	// Should have both agent.started AND agent.quarantined events.
	var startedCount, quarantinedCount int
	for _, e := range rec.Events {
		switch e.Type {
		case events.AgentStarted:
			startedCount++
		case events.AgentQuarantined:
			quarantinedCount++
			if e.Message != "crash loop detected" {
				t.Errorf("quarantine message = %q, want %q", e.Message, "crash loop detected")
			}
		}
	}
	if startedCount != 1 {
		t.Errorf("agent.started events = %d, want 1", startedCount)
	}
	if quarantinedCount != 1 {
		t.Errorf("agent.quarantined events = %d, want 1", quarantinedCount)
	}

	// Quarantine message should be in stderr.
	if !strings.Contains(stderr.String(), "quarantined") {
		t.Errorf("stderr = %q, want quarantine message", stderr.String())
	}
}

func TestReconcileQuarantineAutoClears(t *testing.T) {
	// After window expires, agent should start again.
	ct := newCrashTracker(2, 10*time.Minute)
	now := time.Now()

	// Two starts in the past → quarantined.
	ct.recordStart("gc-city-mayor", now)
	ct.recordStart("gc-city-mayor", now.Add(time.Minute))

	// Verify quarantined.
	if !ct.isQuarantined("gc-city-mayor", now.Add(2*time.Minute)) {
		t.Fatal("precondition: should be quarantined")
	}

	// After 11 minutes, window slides past → auto-cleared.
	if ct.isQuarantined("gc-city-mayor", now.Add(11*time.Minute)) {
		t.Error("should auto-clear after restart window expires")
	}
}

// ---------------------------------------------------------------------------
// Suspended agent messaging tests
// ---------------------------------------------------------------------------

func TestReconcileSuspendedAgentMessaging(t *testing.T) {
	// Suspended agent shows up as running orphan → should get
	// "Stopped suspended agent" message and agent.suspended event.
	rops := newFakeReconcileOps()
	rops.running["gc-city-builder"] = true
	sp := session.NewFake()
	_ = sp.Start("gc-city-builder", session.Config{})
	sp.Calls = nil
	rec := events.NewFake()

	suspended := map[string]bool{"gc-city-builder": true}
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, rec, "gc-city-", nil, suspended, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stdout.String(), "Stopped suspended agent 'gc-city-builder'") {
		t.Errorf("stdout = %q, want suspended stop message", stdout.String())
	}
	// Should NOT say "orphan".
	if strings.Contains(stdout.String(), "orphan") {
		t.Errorf("stdout should not contain 'orphan': %q", stdout.String())
	}
	// Event should be agent.suspended, not agent.stopped.
	if len(rec.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(rec.Events))
	}
	if rec.Events[0].Type != events.AgentSuspended {
		t.Errorf("event type = %q, want %q", rec.Events[0].Type, events.AgentSuspended)
	}
}

func TestReconcileOrphanNotSuspended(t *testing.T) {
	// True orphan (not in suspendedNames) still gets "Stopped orphan session".
	rops := newFakeReconcileOps()
	rops.running["gc-city-oldagent"] = true
	sp := session.NewFake()
	_ = sp.Start("gc-city-oldagent", session.Config{})
	sp.Calls = nil

	// Empty suspended set — everything is an orphan.
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents(nil, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if !strings.Contains(stdout.String(), "Stopped orphan session 'gc-city-oldagent'") {
		t.Errorf("stdout = %q, want orphan stop message", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Restart-requested tests
// ---------------------------------------------------------------------------

func TestReconcileRestartRequestedRestartsAgent(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()
	rec := events.NewFake()

	dops := newFakeDrainOps()
	dops.restartRequested["gc-city-mayor"] = true

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, dops, nil, nil, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Should see restart messages.
	if !strings.Contains(stdout.String(), "requested restart") {
		t.Errorf("stdout = %q, want restart-requested message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Restarted agent 'mayor'") {
		t.Errorf("stdout = %q, want restart confirmation", stdout.String())
	}

	// Events: AgentStopped + AgentStarted.
	var stoppedCount, startedCount int
	for _, e := range rec.Events {
		switch e.Type {
		case events.AgentStopped:
			stoppedCount++
			if e.Message != "restart requested by agent" {
				t.Errorf("stopped event message = %q, want %q", e.Message, "restart requested by agent")
			}
		case events.AgentStarted:
			startedCount++
		}
	}
	if stoppedCount != 1 {
		t.Errorf("agent.stopped events = %d, want 1", stoppedCount)
	}
	if startedCount != 1 {
		t.Errorf("agent.started events = %d, want 1", startedCount)
	}
}

func TestReconcileRestartRequestedNotSet(t *testing.T) {
	// Agent running, no restart requested → no restart.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	dops := newFakeDrainOps()
	// restartRequested NOT set

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, dops, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if strings.Contains(stdout.String(), "restart") {
		t.Errorf("stdout = %q, should not contain restart message", stdout.String())
	}
}

func TestReconcileRestartRequestedRecordsCrashTracker(t *testing.T) {
	// Restart-requested should count in crash tracker.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	dops := newFakeDrainOps()
	dops.restartRequested["gc-city-mayor"] = true

	ct := newFakeCrashTracker()

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, dops, ct, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)

	if len(ct.starts["gc-city-mayor"]) != 1 {
		t.Errorf("crash tracker starts = %d, want 1", len(ct.starts["gc-city-mayor"]))
	}
}

func TestReconcileRestartRequestedNilDops(t *testing.T) {
	// nil dops → restart check skipped, no panic.
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if strings.Contains(stdout.String(), "restart") {
		t.Errorf("stdout = %q, should not contain restart message with nil dops", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Idle detection tests
// ---------------------------------------------------------------------------

func TestReconcileIdleAgentRestarted(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()
	rec := events.NewFake()

	it := newFakeIdleTracker()
	it.idle["gc-city-mayor"] = true

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, it, rec, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	// Agent should have been stopped then restarted.
	if !strings.Contains(stdout.String(), "idle too long") {
		t.Errorf("stdout = %q, want idle message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Restarted agent 'mayor'") {
		t.Errorf("stdout = %q, want restart message", stdout.String())
	}

	// Events: AgentIdleKilled + AgentStarted.
	if len(rec.Events) < 2 {
		t.Fatalf("got %d events, want >= 2", len(rec.Events))
	}
	if rec.Events[0].Type != events.AgentIdleKilled {
		t.Errorf("event[0].Type = %q, want %q", rec.Events[0].Type, events.AgentIdleKilled)
	}
	if rec.Events[1].Type != events.AgentStarted {
		t.Errorf("event[1].Type = %q, want %q", rec.Events[1].Type, events.AgentStarted)
	}
}

func TestReconcileNonIdleAgentLeftAlone(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	it := newFakeIdleTracker()
	// idle["gc-city-mayor"] not set → not idle

	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, it, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if strings.Contains(stdout.String(), "idle") {
		t.Errorf("stdout = %q, should not contain idle message", stdout.String())
	}
}

func TestReconcileNilIdleTrackerSkips(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	// nil idle tracker → backward compatible, no idle check.
	var stdout, stderr bytes.Buffer
	code := doReconcileAgents([]agent.Agent{f}, sp, rops, nil, nil, nil, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}

	if strings.Contains(stdout.String(), "idle") {
		t.Errorf("stdout = %q, should not contain idle message with nil tracker", stdout.String())
	}
}

func TestReconcileIdleKillCountsAsRestart(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.FakeSessionConfig = session.Config{Command: "claude"}

	rops := newFakeReconcileOps()
	rops.running["gc-city-mayor"] = true
	rops.hashes["gc-city-mayor"] = session.ConfigFingerprint(session.Config{Command: "claude"})
	sp := session.NewFake()

	ct := newFakeCrashTracker()
	it := newFakeIdleTracker()
	it.idle["gc-city-mayor"] = true

	var stdout, stderr bytes.Buffer
	doReconcileAgents([]agent.Agent{f}, sp, rops, nil, ct, it, events.Discard, "gc-city-", nil, nil, &stdout, &stderr)

	// Crash tracker should have recorded a start for this session.
	if len(ct.starts["gc-city-mayor"]) != 1 {
		t.Errorf("crash tracker starts = %d, want 1 (idle kill should count as restart)", len(ct.starts["gc-city-mayor"]))
	}
}

// ---------------------------------------------------------------------------
// gracefulStopAll tests
// ---------------------------------------------------------------------------

func TestGracefulStopAllZeroTimeout(t *testing.T) {
	// Zero timeout → immediate kill, no Interrupt calls.
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	_ = sp.Start("gc-city-worker", session.Config{})
	sp.Calls = nil
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	gracefulStopAll([]string{"gc-city-mayor", "gc-city-worker"}, sp, 0, rec, &stdout, &stderr)

	// No Interrupt calls.
	for _, c := range sp.Calls {
		if c.Method == "Interrupt" {
			t.Error("Interrupt should not be called with zero timeout")
		}
	}
	// Both agents stopped.
	if sp.IsRunning("gc-city-mayor") {
		t.Error("mayor should be stopped")
	}
	if sp.IsRunning("gc-city-worker") {
		t.Error("worker should be stopped")
	}
	if !strings.Contains(stdout.String(), "Stopped agent 'gc-city-mayor'") {
		t.Errorf("stdout missing mayor stop: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Stopped agent 'gc-city-worker'") {
		t.Errorf("stdout missing worker stop: %q", stdout.String())
	}
	// Events recorded.
	if len(rec.Events) != 2 {
		t.Errorf("got %d events, want 2", len(rec.Events))
	}
}

func TestGracefulStopAllWithTimeout(t *testing.T) {
	// Non-zero timeout → Interrupt called for all, then Stop survivors.
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil
	rec := events.NewFake()

	var stdout, stderr bytes.Buffer
	// Use a very short timeout so the test doesn't slow down.
	gracefulStopAll([]string{"gc-city-mayor"}, sp, 1*time.Millisecond, rec, &stdout, &stderr)

	// Interrupt should have been called.
	var sawInterrupt bool
	for _, c := range sp.Calls {
		if c.Method == "Interrupt" && c.Name == "gc-city-mayor" {
			sawInterrupt = true
		}
	}
	if !sawInterrupt {
		t.Error("Interrupt not called for mayor")
	}
	// Agent should be stopped (it was still running after timeout).
	if sp.IsRunning("gc-city-mayor") {
		t.Error("mayor should be stopped after graceful shutdown")
	}
	if !strings.Contains(stdout.String(), "Sent interrupt to 1 agent(s)") {
		t.Errorf("stdout missing interrupt message: %q", stdout.String())
	}
}

func TestGracefulStopAllEmpty(t *testing.T) {
	// Empty list → no-op.
	sp := session.NewFake()
	rec := events.NewFake()
	var stdout, stderr bytes.Buffer
	gracefulStopAll(nil, sp, 5*time.Second, rec, &stdout, &stderr)

	if stdout.Len() > 0 || stderr.Len() > 0 {
		t.Errorf("expected no output for empty list, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(rec.Events) != 0 {
		t.Errorf("got %d events, want 0", len(rec.Events))
	}
}

func TestGracefulStopAllAgentExitsGracefully(t *testing.T) {
	// Agent is already gone by the time pass 2 checks → "exited gracefully".
	sp := session.NewFake()
	_ = sp.Start("gc-city-mayor", session.Config{})
	sp.Calls = nil
	rec := events.NewFake()

	// Stop the session immediately so IsRunning returns false in pass 2.
	_ = sp.Stop("gc-city-mayor")

	var stdout, stderr bytes.Buffer
	gracefulStopAll([]string{"gc-city-mayor"}, sp, 1*time.Millisecond, rec, &stdout, &stderr)

	if !strings.Contains(stdout.String(), "Agent 'gc-city-mayor' exited gracefully") {
		t.Errorf("stdout missing graceful exit message: %q", stdout.String())
	}
	// Event still recorded.
	if len(rec.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(rec.Events))
	}
	if rec.Events[0].Type != events.AgentStopped {
		t.Errorf("event type = %q, want %q", rec.Events[0].Type, events.AgentStopped)
	}
}

// ---------------------------------------------------------------------------
// computeSuspendedNames tests
// ---------------------------------------------------------------------------

func TestComputeSuspendedNames(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "builder", Suspended: true},
			{Name: "polecat", Dir: "frontend", Suspended: true},
		},
	}
	got := computeSuspendedNames(cfg, "test", t.TempDir())
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got["gc-test-builder"] {
		t.Error("missing gc-test-builder")
	}
	if !got["gc-test-frontend--polecat"] {
		t.Error("missing gc-test-frontend--polecat")
	}
	// Non-suspended agent should NOT be present.
	if got["gc-test-mayor"] {
		t.Error("non-suspended mayor should not be in set")
	}
}

func TestComputeSuspendedNamesIncludesRigSuspended(t *testing.T) {
	// Agent in a suspended rig should appear in suspended names.
	rigPath := t.TempDir()
	cityPath := t.TempDir()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents: []config.Agent{
			{Name: "mayor"},                 // city-wide, not affected
			{Name: "polecat", Dir: rigPath}, // rig-scoped, rig is suspended
			{Name: "builder", Dir: rigPath}, // rig-scoped, rig is suspended
		},
		Rigs: []config.Rig{
			{Name: "frontend", Path: rigPath, Suspended: true},
		},
	}
	got := computeSuspendedNames(cfg, "test", cityPath)

	// Both rig-scoped agents should be in the suspended set.
	polecatSession := agent.SessionNameFor("test", rigPath+"/polecat", "")
	builderSession := agent.SessionNameFor("test", rigPath+"/builder", "")
	if !got[polecatSession] {
		t.Errorf("missing %s in suspended names", polecatSession)
	}
	if !got[builderSession] {
		t.Errorf("missing %s in suspended names", builderSession)
	}
	// City-wide mayor should NOT be present.
	mayorSession := agent.SessionNameFor("test", "mayor", "")
	if got[mayorSession] {
		t.Errorf("non-rig mayor should not be in suspended names")
	}
}

func TestComputeSuspendedNamesCitySuspended(t *testing.T) {
	// When workspace.suspended is true, ALL agents are suspended.
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test", Suspended: true},
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
			{Name: "polecat", Dir: "frontend"},
		},
	}
	got := computeSuspendedNames(cfg, "test", t.TempDir())
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (all agents)", len(got))
	}
	if !got["gc-test-mayor"] {
		t.Error("missing gc-test-mayor")
	}
	if !got["gc-test-worker"] {
		t.Error("missing gc-test-worker")
	}
	if !got["gc-test-frontend--polecat"] {
		t.Error("missing gc-test-frontend--polecat")
	}
}
