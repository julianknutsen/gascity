package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gastownhall/gascity/internal/runtime"
)

// gc status observes every configured agent through the runtime provider,
// and each observation is several probes (IsRunning, ObserveLiveness,
// GetMeta, GetLastActivity, ...). Subprocess-backed providers (tmux, herdr)
// answer each probe with a fork+exec, and the status code asks the same
// question many times over: pool discovery calls ListRunning("") once per
// pool, a herdr ListRunning fans out to one `agent get` per bound session,
// and every observation re-asks IsRunning for a name that was just listed.
// On a nine-session city that measured 486 herdr spawns per `gc status`,
// ~3 s of system time, with most probes hitting the old 50 ms bound: the
// command reported partial status and showed live sessions as not running.
//
// statusProvider therefore treats one status pass as a point-in-time
// snapshot of the runtime:
//
//   - identical read-only probes (same method, same arguments) run once per
//     wrapper; concurrent and later callers share that one result;
//   - a probe gets a realistic per-call budget (statusProviderCallTimeout),
//     so a healthy runtime under host load is not reported as absent;
//   - the pass as a whole has a wall-clock ceiling
//     (statusProviderPassBudget), so a hung runtime costs one bounded wait
//     rather than one per probe.
//
// A probe that outlives its caller's budget keeps running; its result is
// kept and served to the next caller instead of being thrown away. Any
// mutation through the wrapper (Start, Stop, SetMeta, ...) drops the
// snapshot so a later probe re-reads the runtime.
var (
	statusProviderCallTimeout    = 500 * time.Millisecond
	statusProviderPassBudget     = 5 * time.Second
	statusProviderTimeoutWarning = func() {
		fmt.Fprintln(os.Stderr, "gc status: runtime status probe timed out; using partial status")
	}
)

type statusProvider struct {
	base     runtime.Provider
	warnOnce sync.Once
	partial  atomic.Bool

	mu       sync.Mutex
	deadline time.Time // pass deadline; zero until the first probe
	probes   map[string]*statusProbe
}

// statusProbe is one in-flight or completed runtime probe. done is closed
// once result holds the provider's answer.
type statusProbe struct {
	done   chan struct{}
	result any
}

var _ runtime.RelaunchProvider = (*statusProvider)(nil)

func statusProviderPartial(sp any) bool {
	p, ok := sp.(*statusProvider)
	return ok && p.partial.Load()
}

func markStatusProviderPartial(sp any) {
	if p, ok := sp.(*statusProvider); ok {
		p.partial.Store(true)
	}
}

func (p *statusProvider) StatusPartial() bool {
	return p.partial.Load()
}

func newBoundedStatusProvider(base runtime.Provider) runtime.Provider {
	if sp, ok := base.(*statusProvider); ok {
		return sp
	}
	return &statusProvider{base: base, probes: make(map[string]*statusProbe)}
}

// statusProbeKey identifies a probe by method and arguments. NUL separates
// the parts because provider arguments are free-form strings that may
// contain any printable separator.
func statusProbeKey(method string, args ...string) string {
	return method + "\x00" + strings.Join(args, "\x00")
}

// probe returns the memoized probe for key, starting fn on a goroutine when
// no probe with that key exists yet.
func (p *statusProvider) probe(key string, fn func() any) *statusProbe {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pr, ok := p.probes[key]; ok {
		return pr
	}
	pr := &statusProbe{done: make(chan struct{})}
	p.probes[key] = pr
	go func() {
		pr.result = fn()
		close(pr.done)
	}()
	return pr
}

// invalidate drops the memoized probes. Called by every mutating method so
// a probe issued after a mutation observes the runtime again.
func (p *statusProvider) invalidate() {
	p.mu.Lock()
	p.probes = make(map[string]*statusProbe)
	p.mu.Unlock()
}

// callBudget returns how long a probe issued at now may wait: the per-call
// timeout, shortened to whatever is left of the pass budget. The pass
// budget starts at the first probe, not at construction, because the
// wrapper is built before the bead store is opened and that can take
// seconds on its own. ok is false once the pass budget is spent.
func (p *statusProvider) callBudget(now time.Time) (wait time.Duration, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deadline.IsZero() {
		p.deadline = now.Add(statusProviderPassBudget)
	}
	remaining := p.deadline.Sub(now)
	if remaining <= 0 {
		return 0, false
	}
	return min(statusProviderCallTimeout, remaining), true
}

func (p *statusProvider) markPartial() {
	p.partial.Store(true)
	p.warnOnce.Do(statusProviderTimeoutWarning)
}

// boundedStatusCall answers a read-only probe from the pass snapshot,
// running fn at most once per key. It returns fallback, and marks the status
// partial, when the probe does not complete within the caller's budget; the
// probe itself keeps running so its answer serves the next caller.
func boundedStatusCall[T any](p *statusProvider, key string, fallback T, fn func() T) T {
	pr := p.probe(key, func() any { return fn() })
	if statusProviderCallTimeout <= 0 {
		<-pr.done
		return pr.result.(T)
	}
	if wait, ok := p.callBudget(time.Now()); ok {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-pr.done:
			return pr.result.(T)
		case <-timer.C:
		}
	}
	p.markPartial()
	return fallback
}

func (p *statusProvider) Start(ctx context.Context, name string, cfg runtime.Config) error {
	p.invalidate()
	return p.base.Start(ctx, name, cfg)
}

func (p *statusProvider) Stop(name string) error {
	p.invalidate()
	return p.base.Stop(name)
}

func (p *statusProvider) Interrupt(name string) error {
	p.invalidate()
	return p.base.Interrupt(name)
}

func (p *statusProvider) IsRunning(name string) bool {
	return boundedStatusCall(p, statusProbeKey("IsRunning", name), false, func() bool {
		return p.base.IsRunning(name)
	})
}

func (p *statusProvider) IsAttached(name string) bool {
	return boundedStatusCall(p, statusProbeKey("IsAttached", name), false, func() bool {
		return p.base.IsAttached(name)
	})
}

func (p *statusProvider) Attach(name string) error {
	p.invalidate()
	return p.base.Attach(name)
}

func (p *statusProvider) ProcessAlive(name string, processNames []string) bool {
	key := statusProbeKey("ProcessAlive", name, strings.Join(processNames, ","))
	return boundedStatusCall(p, key, false, func() bool {
		return p.base.ProcessAlive(name, processNames)
	})
}

func (p *statusProvider) ObserveLiveness(name string, processNames []string) runtime.Liveness {
	key := statusProbeKey("ObserveLiveness", name, strings.Join(processNames, ","))
	return boundedStatusCall(p, key, runtime.Liveness{}, func() runtime.Liveness {
		return runtime.ObserveLiveness(p.base, name, processNames)
	})
}

func (p *statusProvider) Nudge(name string, content []runtime.ContentBlock) error {
	p.invalidate()
	return p.base.Nudge(name, content)
}

func (p *statusProvider) SetMeta(name, key, value string) error {
	p.invalidate()
	return p.base.SetMeta(name, key, value)
}

func (p *statusProvider) GetMeta(name, key string) (string, error) {
	type result struct {
		value string
		err   error
	}
	r := boundedStatusCall(p, statusProbeKey("GetMeta", name, key), result{}, func() result {
		value, err := p.base.GetMeta(name, key)
		return result{value: value, err: err}
	})
	return r.value, r.err
}

func (p *statusProvider) RemoveMeta(name, key string) error {
	p.invalidate()
	return p.base.RemoveMeta(name, key)
}

func (p *statusProvider) Peek(name string, lines int) (string, error) {
	type result struct {
		value string
		err   error
	}
	r := boundedStatusCall(p, statusProbeKey("Peek", name, strconv.Itoa(lines)), result{}, func() result {
		value, err := p.base.Peek(name, lines)
		return result{value: value, err: err}
	})
	return r.value, r.err
}

func (p *statusProvider) ListRunning(prefix string) ([]string, error) {
	type result struct {
		value []string
		err   error
	}
	r := boundedStatusCall(p, statusProbeKey("ListRunning", prefix), result{}, func() result {
		value, err := p.base.ListRunning(prefix)
		return result{value: value, err: err}
	})
	return r.value, r.err
}

func (p *statusProvider) RouteACP(name string) {
	if router, ok := p.base.(interface{ RouteACP(string) }); ok {
		p.invalidate()
		router.RouteACP(name)
	}
}

func (p *statusProvider) GetLastActivity(name string) (time.Time, error) {
	type result struct {
		value time.Time
		err   error
	}
	r := boundedStatusCall(p, statusProbeKey("GetLastActivity", name), result{}, func() result {
		value, err := p.base.GetLastActivity(name)
		return result{value: value, err: err}
	})
	return r.value, r.err
}

func (p *statusProvider) ClearScrollback(name string) error {
	p.invalidate()
	return p.base.ClearScrollback(name)
}

func (p *statusProvider) CopyTo(name, src, relDst string) error {
	p.invalidate()
	return p.base.CopyTo(name, src, relDst)
}

func (p *statusProvider) SendKeys(name string, keys ...string) error {
	p.invalidate()
	return p.base.SendKeys(name, keys...)
}

func (p *statusProvider) RunLive(name string, cfg runtime.Config) error {
	p.invalidate()
	return p.base.RunLive(name, cfg)
}

// Relaunch forwards a warm-box agent relaunch to the wrapped provider when it
// supports one, so the reconciler's RelaunchProvider type-assert is not masked
// by the status wrapper. Not bounded — it is a mutation, not a status probe.
func (p *statusProvider) Relaunch(ctx context.Context, name string, cfg runtime.Config) error {
	if rp, ok := p.base.(runtime.RelaunchProvider); ok {
		p.invalidate()
		return rp.Relaunch(ctx, name, cfg)
	}
	return runtime.ErrRelaunchUnsupported
}

func (p *statusProvider) Capabilities() runtime.ProviderCapabilities {
	return p.base.Capabilities()
}

func (p *statusProvider) Pending(name string) (*runtime.PendingInteraction, error) {
	ip, ok := p.base.(runtime.InteractionProvider)
	if !ok {
		return nil, nil
	}
	type result struct {
		value *runtime.PendingInteraction
		err   error
	}
	r := boundedStatusCall(p, statusProbeKey("Pending", name), result{}, func() result {
		value, err := ip.Pending(name)
		return result{value: value, err: err}
	})
	return r.value, r.err
}

func (p *statusProvider) Respond(name string, response runtime.InteractionResponse) error {
	ip, ok := p.base.(runtime.InteractionProvider)
	if !ok {
		return runtime.ErrInteractionUnsupported
	}
	p.invalidate()
	return ip.Respond(name, response)
}
