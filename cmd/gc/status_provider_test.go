package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/runtime"
)

// statusProbeProvider is a runtime.Provider whose IsRunning the test can
// delay, gate, and count.
type statusProbeProvider struct {
	runtime.Provider
	delay        atomic.Int64
	running      atomic.Bool
	liveness     atomic.Value
	observeCall  atomic.Int32
	runningCalls atomic.Int32
	metaCalls    atomic.Int32
	gate         chan struct{} // when set, IsRunning blocks until it is closed
}

func newStatusProbeProvider() *statusProbeProvider {
	p := &statusProbeProvider{Provider: runtime.NewFake()}
	p.liveness.Store(runtime.Liveness{})
	return p
}

func (p *statusProbeProvider) IsRunning(string) bool {
	p.runningCalls.Add(1)
	if p.gate != nil {
		<-p.gate
	}
	time.Sleep(time.Duration(p.delay.Load()))
	return p.running.Load()
}

func (p *statusProbeProvider) GetMeta(name, key string) (string, error) {
	p.metaCalls.Add(1)
	return p.Provider.GetMeta(name, key)
}

func (p *statusProbeProvider) ObserveLiveness(string, []string) runtime.Liveness {
	p.observeCall.Add(1)
	return p.liveness.Load().(runtime.Liveness)
}

// setStatusProviderBudgets overrides the probe budgets for one test and
// replaces the stderr warning with a counter.
func setStatusProviderBudgets(t *testing.T, call, pass time.Duration) *atomic.Int32 {
	t.Helper()
	origCall, origPass, origWarn := statusProviderCallTimeout, statusProviderPassBudget, statusProviderTimeoutWarning
	t.Cleanup(func() {
		statusProviderCallTimeout = origCall
		statusProviderPassBudget = origPass
		statusProviderTimeoutWarning = origWarn
	})
	statusProviderCallTimeout = call
	statusProviderPassBudget = pass
	var warnings atomic.Int32
	statusProviderTimeoutWarning = func() { warnings.Add(1) }
	return &warnings
}

// A status pass is a point-in-time snapshot: the same probe asked twice
// reaches the runtime once. Before this, a nine-session city spawned 486
// herdr subprocesses per `gc status` because pool discovery and per-agent
// observation kept re-asking ListRunning and IsRunning for the same names.
func TestStatusProviderMemoizesIdenticalProbesWithinPass(t *testing.T) {
	base := newStatusProbeProvider()
	base.running.Store(true)
	wrapped := newBoundedStatusProvider(base)

	for i := 0; i < 20; i++ {
		if !wrapped.IsRunning("worker") {
			t.Fatalf("IsRunning call %d = false, want true", i)
		}
	}
	if got := base.runningCalls.Load(); got != 1 {
		t.Fatalf("IsRunning reached the provider %d times for one name, want 1", got)
	}
	wrapped.IsRunning("other")
	if got := base.runningCalls.Load(); got != 2 {
		t.Fatalf("IsRunning reached the provider %d times for two names, want 2", got)
	}

	// Arguments are part of a probe's identity: two keys, two reads.
	for _, key := range []string{"a", "a", "b"} {
		_, _ = wrapped.GetMeta("worker", key)
	}
	if got := base.metaCalls.Load(); got != 2 {
		t.Fatalf("GetMeta reached the provider %d times for two keys, want 2", got)
	}
}

// Concurrent callers of one probe (the observation pool runs eight wide)
// share a single in-flight runtime call instead of each spawning their own.
func TestStatusProviderConcurrentIdenticalProbesShareOneCall(t *testing.T) {
	base := newStatusProbeProvider()
	base.running.Store(true)
	base.gate = make(chan struct{})
	wrapped := newBoundedStatusProvider(base)

	results := make([]bool, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = wrapped.IsRunning("worker")
		}(i)
	}
	close(base.gate)
	wg.Wait()

	for i, running := range results {
		if !running {
			t.Fatalf("caller %d got false, want the shared probe's true", i)
		}
	}
	if got := base.runningCalls.Load(); got != 1 {
		t.Fatalf("IsRunning reached the provider %d times for eight concurrent callers, want 1", got)
	}
}

// statusProbeDone returns the done channel of the probe memoized under key,
// so a test can wait for a late probe's answer on the probe's own lifecycle
// signal instead of polling the wrapper.
func statusProbeDone(t *testing.T, sp runtime.Provider, key string) <-chan struct{} {
	t.Helper()
	p, ok := sp.(*statusProvider)
	if !ok {
		t.Fatalf("provider is %T, want *statusProvider", sp)
	}
	p.mu.Lock()
	pr := p.probes[key]
	p.mu.Unlock()
	if pr == nil {
		t.Fatalf("no probe memoized under %q", key)
	}
	return pr.done
}

// A probe that outlives its caller's budget is not thrown away: it keeps
// running and its answer serves the next caller. The old wrapper discarded
// the late result and re-spawned the probe on the next call, so a slow
// runtime paid for every probe twice and still reported it as absent.
func TestStatusProviderTimedOutProbeServesItsResultToTheNextCaller(t *testing.T) {
	warnings := setStatusProviderBudgets(t, 10*time.Millisecond, 5*time.Second)
	base := newStatusProbeProvider()
	base.running.Store(true)
	base.gate = make(chan struct{})
	wrapped := newBoundedStatusProvider(base)

	if wrapped.IsRunning("worker") {
		t.Fatal("first IsRunning returned true, want timeout fallback false while the probe is blocked")
	}
	if !statusProviderPartial(wrapped) {
		t.Fatal("statusProviderPartial = false, want true after a probe timeout")
	}

	close(base.gate)
	select {
	case <-statusProbeDone(t, wrapped, statusProbeKey("IsRunning", "worker")):
	case <-time.After(5 * time.Second):
		t.Fatal("the timed-out probe never completed after the provider answered")
	}
	if !wrapped.IsRunning("worker") {
		t.Fatal("IsRunning returned false after the late probe answered, want its memoized true")
	}
	if got := base.runningCalls.Load(); got != 1 {
		t.Fatalf("IsRunning reached the provider %d times, want 1: the timed-out probe must be reused, not re-spawned", got)
	}
	if got := warnings.Load(); got != 1 {
		t.Fatalf("timeout warnings = %d, want 1", got)
	}
}

// The pass budget caps what a hung runtime can cost: once it is spent,
// further probes return their fallback at once instead of each waiting out
// the per-call timeout.
func TestStatusProviderPassBudgetStopsWaitingOnAHungRuntime(t *testing.T) {
	warnings := setStatusProviderBudgets(t, time.Second, 30*time.Millisecond)
	base := newStatusProbeProvider()
	base.running.Store(true)
	base.gate = make(chan struct{})
	t.Cleanup(func() { close(base.gate) }) // let the hung probes exit
	wrapped := newBoundedStatusProvider(base)

	start := time.Now()
	for _, name := range []string{"a", "b", "c"} {
		if wrapped.IsRunning(name) {
			t.Fatalf("IsRunning(%q) = true, want fallback false from a hung probe", name)
		}
	}
	if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
		t.Fatalf("three probes on a hung runtime took %s; the 30ms pass budget should have capped the wait well under the 1s per-call timeout", elapsed)
	}
	if !statusProviderPartial(wrapped) {
		t.Fatal("statusProviderPartial = false, want true after the pass budget was spent")
	}
	if got := warnings.Load(); got != 1 {
		t.Fatalf("timeout warnings = %d, want exactly 1 for the whole pass", got)
	}
}

// A mutation through the wrapper invalidates the snapshot, so a probe
// issued afterwards observes the runtime again.
func TestStatusProviderMutationInvalidatesSnapshot(t *testing.T) {
	base := newStatusProbeProvider()
	base.running.Store(true)
	wrapped := newBoundedStatusProvider(base)

	wrapped.IsRunning("worker")
	_ = wrapped.SetMeta("worker", "k", "v")
	wrapped.IsRunning("worker")
	if got := base.runningCalls.Load(); got != 2 {
		t.Fatalf("IsRunning reached the provider %d times across a mutation, want 2", got)
	}
}

func TestStatusProviderPreservesNativeLivenessObservation(t *testing.T) {
	base := newStatusProbeProvider()
	base.liveness.Store(runtime.Liveness{Running: true, Alive: true})
	wrapped := newBoundedStatusProvider(base)

	got := runtime.ObserveLiveness(wrapped, "worker", []string{"agent"})
	if !got.Running || !got.Alive {
		t.Fatalf("ObserveLiveness = %#v, want running+alive from native observer", got)
	}
	if calls := base.observeCall.Load(); calls != 1 {
		t.Fatalf("ObserveLiveness calls = %d, want 1", calls)
	}
}

func TestStatusProviderTimeoutMarksPartial(t *testing.T) {
	setStatusProviderBudgets(t, 10*time.Millisecond, 5*time.Second)

	base := newStatusProbeProvider()
	base.running.Store(true)
	base.delay.Store(int64(100 * time.Millisecond))
	wrapped := newBoundedStatusProvider(base)

	if wrapped.IsRunning("worker") {
		t.Fatal("IsRunning returned true, want timeout fallback false")
	}
	if !statusProviderPartial(wrapped) {
		t.Fatal("statusProviderPartial = false, want true after runtime probe timeout")
	}
}
