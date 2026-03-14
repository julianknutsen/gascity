package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/convergence"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/telemetry"
	"github.com/gastownhall/gascity/internal/workspacesvc"
)

// CityRuntime holds all running state for a single city's reconciliation
// loop. It encapsulates the per-city lifecycle that was previously spread
// across runController and controllerLoop. A machine-wide supervisor can
// instantiate multiple CityRuntimes — one per registered city.
type CityRuntime struct {
	cityPath  string
	cityName  string
	tomlPath  string
	watchDirs []string

	serviceStateMu sync.RWMutex
	cfg            *config.City
	sp             runtime.Provider
	buildFn        func(*config.City, runtime.Provider, beads.Store) map[string]TemplateParams

	rops reconcileOps
	dops drainOps
	ct   crashTracker
	it   idleTracker
	wg   wispGC
	ad   automationDispatcher

	rec events.Recorder
	cs  *controllerState // nil when API is disabled
	svc *workspacesvc.Manager

	poolSessions      map[string]time.Duration
	poolDeathHandlers map[string]poolDeathInfo
	suspendedNames    map[string]bool

	standaloneCityStore beads.Store // non-nil when API disabled; for chat auto-suspend

	// Bead-driven reconciler state (Phase 2f).
	sessionDrains *drainTracker // in-memory drain tracker; nil when bead reconciler disabled

	convHandler      *convergence.Handler     // nil until bead store available
	convStoreAdapter *convergenceStoreAdapter // typed reference; avoids type assertions in tick/reconcile
	convergenceReqCh chan convergenceRequest  // receives CLI commands from controller.sock
	pokeCh           chan struct{}            // non-blocking signal to trigger immediate reconciler tick

	shutdownOnce   sync.Once
	logPrefix      string // "gc start" or "gc supervisor"
	stdout, stderr io.Writer
}

// CityRuntimeParams holds the caller-provided parameters for creating a
// CityRuntime. Internal components (reconcileOps, crashTracker, etc.) are
// built by the constructor from these inputs.
type CityRuntimeParams struct {
	CityPath  string
	CityName  string
	TomlPath  string
	WatchDirs []string

	Cfg     *config.City
	SP      runtime.Provider
	BuildFn func(*config.City, runtime.Provider, beads.Store) map[string]TemplateParams
	Dops    drainOps

	Rec events.Recorder

	PoolSessions      map[string]time.Duration
	PoolDeathHandlers map[string]poolDeathInfo

	ConvergenceReqCh chan convergenceRequest // may be nil
	PokeCh           chan struct{}           // may be nil; triggers immediate tick

	LogPrefix      string // "gc start" or "gc supervisor"; defaults to "gc start"
	Stdout, Stderr io.Writer
}

// newCityRuntime creates a CityRuntime, building internal components
// (reconcileOps, crash tracker, idle tracker, wisp GC, automation
// dispatcher) from the provided parameters.
func newCityRuntime(p CityRuntimeParams) *CityRuntime {
	rops := newReconcileOps(p.SP)

	var ct crashTracker
	if maxR := p.Cfg.Daemon.MaxRestartsOrDefault(); maxR > 0 {
		ct = newCrashTracker(maxR, p.Cfg.Daemon.RestartWindowDuration())
	}

	it := buildIdleTracker(p.Cfg, p.CityName, p.CityPath, p.SP)

	var wg wispGC
	if p.Cfg.Daemon.WispGCEnabled() {
		wg = newWispGC(p.Cfg.Daemon.WispGCIntervalDuration(),
			p.Cfg.Daemon.WispTTLDuration(), beads.ExecCommandRunner())
	}

	ad := buildAutomationDispatcher(p.CityPath, p.Cfg, beads.ExecCommandRunner(), p.Rec, p.Stderr)

	suspendedNames := computeSuspendedNames(p.Cfg, p.CityName, p.CityPath)

	logPrefix := p.LogPrefix
	if logPrefix == "" {
		logPrefix = "gc start"
	}

	cr := &CityRuntime{
		cityPath:          p.CityPath,
		cityName:          p.CityName,
		tomlPath:          p.TomlPath,
		watchDirs:         p.WatchDirs,
		cfg:               p.Cfg,
		sp:                p.SP,
		buildFn:           p.BuildFn,
		rops:              rops,
		dops:              p.Dops,
		ct:                ct,
		it:                it,
		wg:                wg,
		ad:                ad,
		rec:               p.Rec,
		poolSessions:      p.PoolSessions,
		poolDeathHandlers: p.PoolDeathHandlers,
		suspendedNames:    suspendedNames,
		convergenceReqCh:  p.ConvergenceReqCh,
		pokeCh: func() chan struct{} {
			if p.PokeCh != nil {
				return p.PokeCh
			}
			return make(chan struct{}, 1)
		}(),
		logPrefix: logPrefix,
		stdout:    p.Stdout,
		stderr:    p.Stderr,
	}
	cr.svc = workspacesvc.NewManager(&serviceRuntime{cr: cr})
	if err := cr.svc.Reload(); err != nil {
		fmt.Fprintf(cr.stderr, "%s: service init: %v\n", cr.logPrefix, err) //nolint:errcheck // best-effort stderr
	}
	return cr
}

// setControllerState sets the API state for this city. The controller
// state is managed by the caller (who also owns the API server) and
// passed in after construction.
func (cr *CityRuntime) setControllerState(cs *controllerState) {
	cr.cs = cs
}

// crashTracker returns the crash tracker for API server wiring.
func (cr *CityRuntime) crashTrack() crashTracker {
	return cr.ct
}

// run executes the reconciliation loop until ctx is canceled. This is
// the per-city main loop — it watches config, reconciles agents, runs
// wisp GC, and dispatches automations.
func (cr *CityRuntime) run(ctx context.Context) {
	dirty := &atomic.Bool{}
	if cr.tomlPath != "" {
		dirs := cr.watchDirs
		if len(dirs) == 0 {
			dirs = []string{filepath.Dir(cr.tomlPath)}
		}
		cleanup := watchConfigDirs(dirs, dirty, cr.stderr)
		defer cleanup()
	}

	// Track effective provider name for hot-reload detection.
	lastProviderName := cr.cfg.Session.Provider
	if v := os.Getenv("GC_SESSION"); v != "" {
		lastProviderName = v
	}

	cityRoot := filepath.Dir(cr.tomlPath)

	// Enforce restrictive permissions on .gc/ and its subdirectories.
	enforceGCPermissions(cr.cityPath, cr.stderr)

	// Open standalone city bead store when API is disabled.
	// When API is enabled, controllerState manages the store.
	if cr.cs == nil {
		if store, err := openCityStoreAt(cityRoot); err != nil {
			fmt.Fprintf(cr.stderr, "%s: city bead store: %v (auto-suspend disabled)\n", cr.logPrefix, err) //nolint:errcheck // best-effort stderr
		} else {
			cr.standaloneCityStore = store
		}
	}

	// Record bead store health metric.
	telemetry.RecordBeadStoreHealth(context.Background(), cr.cityName, cr.cityBeadStore() != nil)

	// Upgrade to bead-driven reconcile ops when a bead store is available.
	cr.upgradeToBeadReconcileOps()

	// Initialize bead-driven drain tracker when explicitly enabled via config,
	// a bead store is available, and we have a real city path.
	if cr.cfg.Daemon.BeadReconciler && cr.cityBeadStore() != nil && cr.tomlPath != "" {
		cr.sessionDrains = newDrainTracker()
		fmt.Fprintf(cr.stdout, "%s: bead-driven reconciler enabled\n", cr.logPrefix) //nolint:errcheck
	}

	// Adoption barrier: ensure every running session has a bead.
	// Runs on every startup (rerunnable, crash-safe).
	if cr.cityBeadStore() != nil {
		result, passed := runAdoptionBarrier(cr.cityBeadStore(), cr.sp, cr.cfg, cr.cityName, clock.Real{}, cr.stderr, false)
		if result.Adopted > 0 {
			fmt.Fprintf(cr.stdout, "Adopted %d running session(s) into bead store.\n", result.Adopted) //nolint:errcheck
		}
		if !passed {
			// Sessions that fail adoption AND have no matching agent are
			// invisible to the bead reconciler (which only processes beaded
			// sessions). They will be cleaned up when they naturally exit.
			// Sessions with matching agents get beads via syncSessionBeads
			// on the next tick.
			fmt.Fprintf(cr.stderr, "%s: adoption barrier: %d session(s) failed bead creation\n", cr.logPrefix, result.Skipped) //nolint:errcheck
		}
	}

	// Initialize convergence handler (requires bead store).
	cr.initConvergenceHandler()

	// Session bead sync BEFORE reconciliation: ensures beads exist for
	// beadReconcileOps to read/write hashes. Bead "state" metadata reflects
	// pre-reconcile reality and may lag by one tick after reconciliation
	// starts/stops agents. This is acceptable — no external consumer reads
	// bead state within a tick, and it converges on the next sync.
	desiredState := cr.buildFn(cr.cfg, cr.sp, cr.cityBeadStore())
	cr.syncBeadsAndUpdateIndex(desiredState)

	// Convergence startup reconciliation: recover in-progress convergence
	// beads that were interrupted by a controller crash.
	cr.convergenceStartupReconcile(ctx)

	// Initial reconciliation.
	if cr.sessionDrains != nil {
		cr.beadReconcileTick(ctx, desiredState)
	} else {
		doReconcileAgents(desiredState, cr.sp, cr.rops, cr.dops, cr.ct, cr.it, cr.rec,
			cr.poolSessions, cr.suspendedNames,
			cr.cfg.Daemon.DriftDrainTimeoutDuration(), cr.cfg.Session.StartupTimeoutDuration(),
			cr.stdout, cr.stderr, ctx)
	}

	fmt.Fprintln(cr.stdout, "City started.") //nolint:errcheck // best-effort stdout

	interval := cr.cfg.Daemon.PatrolIntervalDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track pool instance liveness for death detection.
	var prevPoolRunning map[string]bool

	for {
		select {
		case <-ticker.C:
			cr.tick(ctx, dirty, &lastProviderName, cityRoot, &prevPoolRunning)
		case <-cr.pokeCh:
			// Event-driven wake path: sling or API assigned work to a sleeping
			// session. Trigger an immediate tick so the reconciler computes
			// workSet and wakes the target without waiting for the next patrol.
			cr.tick(ctx, dirty, &lastProviderName, cityRoot, &prevPoolRunning)
		case req := <-cr.convergenceReqCh:
			// Low-latency path: process convergence commands between ticks.
			// processConvergenceRequests() in tick() drains any that arrived
			// during tick processing. Both paths are safe — channel receives
			// are atomic, so each request is processed exactly once.
			// Note: ordering relative to convergenceTick is non-deterministic
			// via this path, but handlers are idempotent so interleaving is safe.
			reply := cr.safeHandleConvergenceRequest(ctx, req)
			req.replyCh <- reply
		case <-ctx.Done():
			return
		}
	}
}

// tick performs one reconciliation tick: pool death detection, config
// reload (if dirty), agent reconciliation, wisp GC, and automation
// dispatch.
func (cr *CityRuntime) tick(
	ctx context.Context,
	dirty *atomic.Bool,
	lastProviderName *string,
	cityRoot string,
	prevPoolRunning *map[string]bool,
) {
	// Detect pool instance deaths since last tick.
	if len(cr.poolDeathHandlers) > 0 {
		currentRunning, _ := cr.rops.listRunning("")
		currentSet := make(map[string]bool, len(currentRunning))
		for _, name := range currentRunning {
			currentSet[name] = true
		}
		if *prevPoolRunning != nil {
			for sn, info := range cr.poolDeathHandlers {
				if (*prevPoolRunning)[sn] && !currentSet[sn] {
					if _, err := shellScaleCheck(info.Command, info.Dir); err != nil {
						fmt.Fprintf(cr.stderr, "on_death %s: %v\n", sn, err) //nolint:errcheck // best-effort stderr
					}
				}
			}
		}
		*prevPoolRunning = make(map[string]bool)
		for sn := range cr.poolDeathHandlers {
			if currentSet[sn] {
				(*prevPoolRunning)[sn] = true
			}
		}
	}

	if dirty.Swap(false) {
		cr.reloadConfig(ctx, lastProviderName, cityRoot)
	}

	// Session bead sync BEFORE reconciliation (one-tick state lag; see run()).
	// Post-reconcile sync was intentionally removed: the daemon's next tick
	// corrects bead state, and the pre-reconcile sync is sufficient for
	// beadReconcileOps to read/write hashes during reconciliation.
	desiredState := cr.buildFn(cr.cfg, cr.sp, cr.cityBeadStore())
	cr.syncBeadsAndUpdateIndex(desiredState)

	// Use bead-driven reconciler when drain tracker is initialized
	// (requires bead store). Falls back to legacy reconciler otherwise.
	if cr.sessionDrains != nil {
		cr.beadReconcileTick(ctx, desiredState)
	} else {
		doReconcileAgents(desiredState, cr.sp, cr.rops, cr.dops, cr.ct, cr.it, cr.rec,
			cr.poolSessions, cr.suspendedNames,
			cr.cfg.Daemon.DriftDrainTimeoutDuration(), cr.cfg.Session.StartupTimeoutDuration(),
			cr.stdout, cr.stderr, ctx)
	}

	// Wisp GC: purge expired closed molecules.
	if cr.wg != nil && cr.wg.shouldRun(time.Now()) {
		purged, gcErr := cr.wg.runGC(cityRoot, time.Now())
		if gcErr != nil {
			fmt.Fprintf(cr.stderr, "%s: wisp gc: %v\n", cr.logPrefix, gcErr) //nolint:errcheck // best-effort stderr
		} else if purged > 0 {
			fmt.Fprintf(cr.stdout, "Bead GC: purged %d expired bead(s)\n", purged) //nolint:errcheck // best-effort stdout
		}
	}

	// Automation dispatch.
	if cr.ad != nil {
		cr.ad.dispatch(ctx, cityRoot, time.Now())
	}

	if cr.svc != nil {
		cr.svc.Tick(ctx, time.Now())
	}

	// Chat session auto-suspend: suspend detached idle sessions.
	if idleTimeout := cr.cfg.ChatSessions.IdleTimeoutDuration(); idleTimeout > 0 {
		autoSuspendChatSessions(cr.cityBeadStore(), cr.sp, idleTimeout, clock.Real{}, cr.stdout, cr.stderr)
	}

	// Drain queued convergence requests (CLI commands) BEFORE tick so
	// user commands (e.g. stop) take precedence over automated progression.
	cr.processConvergenceRequests(ctx)

	// Convergence tick: process active convergence loops.
	cr.convergenceTick(ctx)
}

// reloadConfig attempts to reload city.toml and update all internal
// components. On error, the old config is kept.
func (cr *CityRuntime) reloadConfig(
	ctx context.Context,
	lastProviderName *string,
	cityRoot string,
) {
	result, err := tryReloadConfig(cr.tomlPath, cr.cityName, cityRoot, cr.stderr)
	if err != nil {
		fmt.Fprintf(cr.stderr, "%s: config reload: %v (keeping old config)\n", cr.logPrefix, err) //nolint:errcheck // best-effort stderr
		telemetry.RecordConfigReload(ctx, "", err)
		return
	}

	oldAgentCount := len(cr.cfg.Agents)
	oldRigCount := len(cr.cfg.Rigs)
	cr.serviceStateMu.Lock()
	cr.cfg = result.Cfg
	cr.serviceStateMu.Unlock()

	// Detect session provider change.
	newProviderName := cr.cfg.Session.Provider
	if v := os.Getenv("GC_SESSION"); v != "" {
		newProviderName = v
	}
	if newProviderName != *lastProviderName {
		if running, lErr := cr.rops.listRunning(""); lErr == nil && len(running) > 0 {
			fmt.Fprintf(cr.stdout, "Provider changed (%s → %s), stopping %d agent(s)...\n", //nolint:errcheck
				displayProviderName(*lastProviderName), displayProviderName(newProviderName), len(running))
			gracefulStopAll(running, cr.sp, cr.cfg.Daemon.ShutdownTimeoutDuration(), cr.rec, cr.stdout, cr.stderr)
		}
		newSp, spErr := newSessionProviderByName(newProviderName, cr.cfg.Session, cr.cityName)
		if spErr != nil {
			fmt.Fprintf(cr.stderr, "%s: new session provider %q: %v (keeping old provider)\n", //nolint:errcheck
				cr.logPrefix, newProviderName, spErr)
		} else {
			cr.serviceStateMu.Lock()
			cr.sp = newSp
			cr.serviceStateMu.Unlock()
			cr.rops = newReconcileOps(cr.sp)
			cr.upgradeToBeadReconcileOps()
			cr.dops = newDrainOps(cr.sp)
			cr.rec.Record(events.Event{
				Type:    events.ProviderSwapped,
				Actor:   "gc",
				Message: fmt.Sprintf("%s → %s", displayProviderName(*lastProviderName), displayProviderName(newProviderName)),
			})
			fmt.Fprintf(cr.stdout, "Session provider swapped to %s.\n", displayProviderName(newProviderName)) //nolint:errcheck
			*lastProviderName = newProviderName
		}
	}

	// Re-materialize and prepend system formulas.
	sysDir, _ := MaterializeSystemFormulas(systemFormulasFS, "system_formulas", cityRoot)
	if sysDir != "" {
		cr.cfg.FormulaLayers.City = append([]string{sysDir}, cr.cfg.FormulaLayers.City...)
		for rigName, layers := range cr.cfg.FormulaLayers.Rigs {
			cr.cfg.FormulaLayers.Rigs[rigName] = append([]string{sysDir}, layers...)
		}
	}
	if err := config.ValidateRigs(cr.cfg.Rigs, cr.cityName); err != nil {
		fmt.Fprintf(cr.stderr, "%s: config reload: %v\n", cr.logPrefix, err) //nolint:errcheck
	}
	resolveRigPaths(cityRoot, cr.cfg.Rigs)
	if err := startBeadsLifecycle(cityRoot, cr.cityName, cr.cfg, cr.stderr); err != nil {
		fmt.Fprintf(cr.stderr, "%s: config reload: %v\n", cr.logPrefix, err) //nolint:errcheck
	}
	if len(cr.cfg.FormulaLayers.City) > 0 {
		if err := ResolveFormulas(cityRoot, cr.cfg.FormulaLayers.City); err != nil {
			fmt.Fprintf(cr.stderr, "%s: config reload: city formulas: %v\n", cr.logPrefix, err) //nolint:errcheck
		}
	}
	for _, r := range cr.cfg.Rigs {
		if layers, ok := cr.cfg.FormulaLayers.Rigs[r.Name]; ok && len(layers) > 0 {
			if err := ResolveFormulas(r.Path, layers); err != nil {
				fmt.Fprintf(cr.stderr, "%s: config reload: rig %q formulas: %v\n", cr.logPrefix, r.Name, err) //nolint:errcheck
			}
		}
	}

	cr.poolSessions = computePoolSessions(cr.cfg, cr.cityName, cr.cityPath, cr.sp)
	cr.poolDeathHandlers = computePoolDeathHandlers(cr.cfg, cr.cityName, cityRoot, cr.sp)
	cr.suspendedNames = computeSuspendedNames(cr.cfg, cr.cityName, cr.cityPath)

	// Rebuild crash tracker if config values changed, otherwise clear all
	// crash history so that a fixed config automatically unquarantines agents.
	newMaxR := cr.cfg.Daemon.MaxRestartsOrDefault()
	newWindow := cr.cfg.Daemon.RestartWindowDuration()
	switch {
	case newMaxR <= 0:
		cr.ct = nil
	case cr.ct == nil:
		cr.ct = newCrashTracker(newMaxR, newWindow)
	default:
		oldMaxR, oldWindow := cr.ct.limits()
		if newMaxR != oldMaxR || newWindow != oldWindow {
			cr.ct = newCrashTracker(newMaxR, newWindow)
		} else {
			cr.ct.clearAll()
		}
	}
	if cr.cs != nil {
		cr.cs.mu.Lock()
		cr.cs.ct = cr.ct
		cr.cs.mu.Unlock()
	}

	cr.it = buildIdleTracker(cr.cfg, cr.cityName, cr.cityPath, cr.sp)

	if cr.cfg.Daemon.WispGCEnabled() {
		cr.wg = newWispGC(cr.cfg.Daemon.WispGCIntervalDuration(),
			cr.cfg.Daemon.WispTTLDuration(), beads.ExecCommandRunner())
	} else {
		cr.wg = nil
	}

	cr.ad = buildAutomationDispatcher(cityRoot, cr.cfg, beads.ExecCommandRunner(), cr.rec, cr.stderr)

	if cr.svc != nil {
		if err := cr.svc.Reload(); err != nil {
			fmt.Fprintf(cr.stderr, "%s: service reload: %v\n", cr.logPrefix, err) //nolint:errcheck // best-effort stderr
		}
	}

	if cr.cs != nil {
		cr.cs.update(cr.cfg, cr.sp)
		// Upgrade rops if store recovered from nil → non-nil.
		cr.upgradeToBeadReconcileOps()
	} else {
		// Refresh standalone city store for auto-suspend.
		// Also recovers from nil → non-nil when bd becomes available after startup.
		if s, err := openCityStoreAt(cityRoot); err != nil {
			if cr.standaloneCityStore != nil {
				fmt.Fprintf(cr.stderr, "%s: city bead store reload: %v\n", cr.logPrefix, err) //nolint:errcheck
			}
		} else {
			cr.standaloneCityStore = s
		}
		// Upgrade rops if store recovered from nil → non-nil.
		cr.upgradeToBeadReconcileOps()
	}

	// Recompute bead-driven reconciler activation from current config.
	// Placed AFTER store recovery so that a nil→non-nil store transition
	// is visible to the activation check on this reload cycle.
	// Toggling bead_reconciler on creates the drain tracker; toggling it
	// off nils it so the next tick falls back to the legacy reconciler.
	// In-flight drains are discarded on toggle-off; the legacy path
	// re-reconciles those sessions within one tick.
	if cr.cfg.Daemon.BeadReconciler && cr.cityBeadStore() != nil && cr.tomlPath != "" {
		if cr.sessionDrains == nil {
			cr.sessionDrains = newDrainTracker()
			fmt.Fprintf(cr.stdout, "%s: bead-driven reconciler enabled (config reload)\n", cr.logPrefix) //nolint:errcheck
		}
	} else {
		if cr.sessionDrains != nil {
			fmt.Fprintf(cr.stdout, "%s: bead-driven reconciler disabled (config reload)\n", cr.logPrefix) //nolint:errcheck
		}
		cr.sessionDrains = nil
	}

	fmt.Fprintf(cr.stdout, "Config reloaded: %s (rev %s)\n", //nolint:errcheck
		configReloadSummary(oldAgentCount, oldRigCount, len(cr.cfg.Agents), len(cr.cfg.Rigs)),
		shortRev(result.Revision))
	telemetry.RecordConfigReload(ctx, result.Revision, nil)
}

// beadReconcileTick runs one reconciliation tick using the bead-driven
// reconciler. It loads session beads from the store, uses the provided
// desired state, and delegates to reconcileSessionBeads.
func (cr *CityRuntime) beadReconcileTick(ctx context.Context, desiredState map[string]TemplateParams) {
	store := cr.cityBeadStore()
	if store == nil {
		return
	}

	// Load open session beads (both legacy and new types, deduplicated).
	open, err := loadSessionBeads(store)
	if err != nil {
		fmt.Fprintf(cr.stderr, "%s: loading session beads: %v\n", cr.logPrefix, err) //nolint:errcheck
		return
	}

	// Compute pool desired counts from the desired state.
	poolDesired := derivePoolDesired(desiredState, cr.cfg)

	// Use cr.cityName consistently — it's the authoritative runtime name.
	cityName := cr.cityName

	cfgNames := configuredSessionNames(cr.cfg, cityName, store)

	// Compute work set via work_query commands for work-driven wake.
	workSet := computeWorkSet(cr.cfg, shellScaleCheck, cr.cityPath)

	reconcileSessionBeads(
		ctx, open, desiredState, cfgNames, cr.cfg, cr.sp, store,
		workSet, cr.sessionDrains, poolDesired, cityName,
		clock.Real{}, cr.rec, cr.cfg.Session.StartupTimeoutDuration(),
		cr.cfg.Daemon.DriftDrainTimeoutDuration(),
		cr.stdout, cr.stderr,
	)
}

// upgradeToBeadReconcileOps upgrades rops from providerReconcileOps to
// beadReconcileOps when a bead store is available. Called during run()
// after the bead store is opened, and again during reloadConfig() if the
// store recovers from nil → non-nil. No-op if no store is available or
// if rops is already a beadReconcileOps (double-wrap guard).
func (cr *CityRuntime) upgradeToBeadReconcileOps() {
	if cr.cityBeadStore() == nil || cr.rops == nil {
		return
	}
	// Guard against double-wrapping.
	if _, ok := cr.rops.(*beadReconcileOps); ok {
		return
	}
	cr.rops = newBeadReconcileOps(cr.rops, cr.cityBeadStore)
}

// syncBeadsAndUpdateIndex runs syncSessionBeads and, if rops is a
// beadReconcileOps, updates its session_name → bead_id index.
func (cr *CityRuntime) syncBeadsAndUpdateIndex(desiredState map[string]TemplateParams) {
	store := cr.cityBeadStore()
	cfgNames := configuredSessionNames(cr.cfg, cr.cityName, store)
	idx := syncSessionBeads(store, desiredState, cr.sp, cfgNames, cr.cfg, clock.Real{}, cr.stderr, cr.sessionDrains != nil)
	if bro, ok := cr.rops.(*beadReconcileOps); ok && idx != nil {
		bro.updateIndex(idx)
	}
}

// cityBeadStore returns the bead store for this city, preferring the
// controllerState store over the standalone store.
func (cr *CityRuntime) cityBeadStore() beads.Store {
	if cr.cs != nil {
		return cr.cs.CityBeadStore()
	}
	return cr.standaloneCityStore
}

// shutdown performs graceful two-pass agent shutdown for this city.
// Safe to call multiple times (e.g., from both panic recovery and
// normal shutdown) — only the first call takes effect.
func (cr *CityRuntime) shutdown() {
	cr.shutdownOnce.Do(func() {
		if cr.svc != nil {
			if err := cr.svc.Close(); err != nil {
				fmt.Fprintf(cr.stderr, "%s: service shutdown: %v\n", cr.logPrefix, err) //nolint:errcheck // best-effort stderr
			}
		}
		timeout := cr.cfg.Daemon.ShutdownTimeoutDuration()
		if cr.rops != nil {
			running, _ := cr.rops.listRunning("")
			gracefulStopAll(running, cr.sp, timeout, cr.rec, cr.stdout, cr.stderr)
		} else {
			ds := cr.buildFn(cr.cfg, cr.sp, cr.cityBeadStore())
			var names []string
			for sn := range ds {
				if cr.sp.IsRunning(sn) {
					names = append(names, sn)
				}
			}
			gracefulStopAll(names, cr.sp, timeout, cr.rec, cr.stdout, cr.stderr)
		}
	})
}
