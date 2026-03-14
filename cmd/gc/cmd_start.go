package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/hooks"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/telemetry"
	"github.com/gastownhall/gascity/internal/workspacesvc"
	"github.com/spf13/cobra"
)

// computeSuspendedNames builds a set of session names for agents marked
// suspended in the config or belonging to suspended rigs. Also includes
// all agents when the city itself is suspended (workspace.suspended).
// Used by the reconciler to distinguish suspended agents from true orphans
// during Phase 2 cleanup.
func computeSuspendedNames(cfg *config.City, cityName, cityPath string) map[string]bool {
	names := make(map[string]bool)
	st := cfg.Workspace.SessionTemplate

	// City-level suspend: all agents are suspended.
	if cfg.Workspace.Suspended {
		for _, a := range cfg.Agents {
			names[cliSessionName(cityPath, cityName, a.QualifiedName(), st)] = true
		}
		return names
	}

	// Individually suspended agents.
	for _, a := range cfg.Agents {
		if a.Suspended {
			qn := a.QualifiedName()
			names[cliSessionName(cityPath, cityName, qn, st)] = true
		}
	}
	// Agents in suspended rigs.
	suspendedRigPaths := make(map[string]bool)
	for _, r := range cfg.Rigs {
		if r.Suspended {
			suspendedRigPaths[filepath.Clean(r.Path)] = true
		}
	}
	if len(suspendedRigPaths) > 0 {
		for _, a := range cfg.Agents {
			if a.Suspended || a.Dir == "" {
				continue // Already counted or no rig scope.
			}
			workDir, err := resolveAgentDir(cityPath, a.Dir)
			if err != nil {
				continue
			}
			if suspendedRigPaths[filepath.Clean(workDir)] {
				names[cliSessionName(cityPath, cityName, a.QualifiedName(), st)] = true
			}
		}
	}
	return names
}

// computePoolSessions builds the set of ALL possible pool session names
// (1..max for bounded pools, currently running for unlimited) for every
// multi-instance pool agent in the config, mapped to the pool's drain
// timeout. Used to distinguish excess pool members (drain) from true orphans
// (kill) during reconciliation, and to enforce drain timeouts.
func computePoolSessions(cfg *config.City, cityName, cityPath string, sp runtime.Provider) map[string]time.Duration {
	ps := make(map[string]time.Duration)
	st := cfg.Workspace.SessionTemplate
	for _, a := range cfg.Agents {
		pool := a.EffectivePool()
		if !a.IsPool() || !pool.IsMultiInstance() {
			continue
		}
		timeout := pool.DrainTimeoutDuration()
		for _, qualifiedInstance := range discoverPoolInstances(a.Name, a.Dir, pool, cityName, st, sp) {
			ps[cliSessionName(cityPath, cityName, qualifiedInstance, st)] = timeout
		}
	}
	return ps
}

// poolDeathInfo holds the on_death command and working directory for a pool instance.
type poolDeathInfo struct {
	Command string // on_death shell command (pre-baked with instance QN)
	Dir     string // working directory for bd commands
}

// computePoolDeathHandlers builds a map from session name to death handler
// for every pool instance (static for bounded pools, currently running for
// unlimited). Used to detect and handle pool deaths.
func computePoolDeathHandlers(cfg *config.City, cityName, cityPath string, sp runtime.Provider) map[string]poolDeathInfo {
	handlers := make(map[string]poolDeathInfo)
	st := cfg.Workspace.SessionTemplate
	for _, a := range cfg.Agents {
		if !a.IsPool() {
			continue
		}
		pool := a.EffectivePool()
		if !pool.IsMultiInstance() {
			continue
		}
		for _, qualifiedInstance := range discoverPoolInstances(a.Name, a.Dir, pool, cityName, st, sp) {
			_, instanceName := config.ParseQualifiedName(qualifiedInstance)
			instance := config.Agent{Name: instanceName, Dir: a.Dir, Pool: a.Pool, PoolName: a.QualifiedName()}
			cmd := instance.EffectiveOnDeath()
			if cmd == "" {
				continue
			}
			dir := cityPath
			if a.Dir != "" {
				if d, err := resolveAgentDir(cityPath, a.Dir); err == nil {
					dir = d
				}
			}
			sn := cliSessionName(cityPath, cityName, qualifiedInstance, st)
			handlers[sn] = poolDeathInfo{Command: cmd, Dir: dir}
		}
	}
	return handlers
}

// extraConfigFiles holds paths from -f flags for CLI-level file layering.
var extraConfigFiles []string

// strictMode promotes composition collision warnings to errors.
// Defaults to true; use --no-strict to disable.
var strictMode bool

// noStrictMode disables strict config checking (opt-out).
var noStrictMode bool

// dryRunMode previews what agents would start without actually starting them.
var dryRunMode bool

// buildIdleTracker creates an idleTracker from the config, populating
// timeouts for agents that have idle_timeout set. Returns nil if no
// agents use idle timeout (disabled).
func buildIdleTracker(cfg *config.City, cityName, cityPath string, sp runtime.Provider) idleTracker {
	var hasAny bool
	st := cfg.Workspace.SessionTemplate
	for _, a := range cfg.Agents {
		if a.IdleTimeoutDuration() > 0 {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}
	it := newIdleTracker()
	for _, a := range cfg.Agents {
		timeout := a.IdleTimeoutDuration()
		if timeout <= 0 {
			continue
		}
		pool := a.EffectivePool()
		if a.IsPool() && pool.IsMultiInstance() {
			// Register each pool instance (worker-1, worker-2, ...).
			for _, qualifiedInstance := range discoverPoolInstances(a.Name, a.Dir, pool, cityName, st, sp) {
				sn := cliSessionName(cityPath, cityName, qualifiedInstance, st)
				it.setTimeout(sn, timeout)
			}
		} else {
			sn := cliSessionName(cityPath, cityName, a.QualifiedName(), st)
			it.setTimeout(sn, timeout)
		}
	}
	return it
}

func newStartCmd(stdout, stderr io.Writer) *cobra.Command {
	var foregroundMode bool
	cmd := &cobra.Command{
		Use:   "start [path]",
		Short: "Start the city (auto-initializes if needed)",
		Long: `Start the city by launching all configured agent sessions.

Auto-initializes the city if no .gc/ directory exists. Fetches remote
packs, resolves providers, installs hooks, and starts agent sessions
via one-shot reconciliation. Use --foreground for a persistent controller
that continuously reconciles agent state.`,
		Example: `  gc start
  gc start ~/my-city
  gc start --foreground
  gc start -f overlay.toml --no-strict`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doStart(args, foregroundMode, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&foregroundMode, "foreground", false,
		"run as a persistent controller (reconcile loop)")
	// Hidden backward-compat alias for --foreground.
	cmd.Flags().BoolVar(&foregroundMode, "controller", false,
		"alias for --foreground")
	cmd.Flags().MarkHidden("controller") //nolint:errcheck // flag always exists
	cmd.Flags().StringArrayVarP(&extraConfigFiles, "file", "f", nil,
		"additional config files to layer (can be repeated)")
	cmd.Flags().BoolVar(&noStrictMode, "no-strict", false,
		"disable strict config collision checking (strict is on by default)")
	cmd.Flags().BoolVarP(&dryRunMode, "dry-run", "n", false,
		"preview what agents would start without starting them")
	return cmd
}

// doStart boots the city. If a path is given, operates there; otherwise uses
// cwd. If no city exists at the target, it auto-initializes one first via
// doInit, then starts all configured agent sessions. When controllerMode is
// true, enters a persistent reconciliation loop instead of one-shot start.
func doStart(args []string, controllerMode bool, stdout, stderr io.Writer) int {
	// Strict mode is on by default; --no-strict disables it.
	strictMode = !noStrictMode

	var dir string
	var err error
	switch {
	case len(args) > 0:
		dir, err = filepath.Abs(args[0])
	case cityFlag != "":
		dir, err = filepath.Abs(cityFlag)
	default:
		dir, err = os.Getwd()
	}
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if _, err := findCity(dir); err != nil {
		// No city found — auto-init at dir (non-interactive).
		// doInit is idempotent-safe: if another process initialized the city
		// concurrently (TOCTOU), it returns non-zero but findCity below will
		// succeed. Only fail if findCity still fails after the attempt.
		doInit(fsys.OSFS{}, dir, defaultWizardConfig(), stdout, stderr)
		dirName := filepath.Base(dir)
		prefix := config.DeriveBeadsPrefix(dirName)
		initDirIfReady(dir, dir, prefix) //nolint:errcheck // best-effort auto-init; gc start handles full lifecycle below
	}

	// Load config to find agents.
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := ensureCityScaffold(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: runtime scaffold: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	// Auto-fetch remote packs before full config load.
	if quickCfg, qErr := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml")); qErr == nil && len(quickCfg.Packs) > 0 {
		if fErr := config.FetchPacks(quickCfg.Packs, cityPath); fErr != nil {
			fmt.Fprintf(stderr, "gc start: fetching packs: %v\n", fErr) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	cfg, prov, err := config.LoadWithIncludes(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"), extraConfigFiles...)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	// Strict mode (default) promotes composition warnings to errors.
	if strictMode && len(prov.Warnings) > 0 {
		for _, w := range prov.Warnings {
			fmt.Fprintf(stderr, "gc start: strict: %s\n", w) //nolint:errcheck // best-effort stderr
		}
		fmt.Fprintln(stderr, "gc start: use --no-strict to disable strict checking") //nolint:errcheck // best-effort stderr
		return 1
	}
	for _, w := range prov.Warnings {
		fmt.Fprintf(stderr, "gc start: warning: %s\n", w) //nolint:errcheck // best-effort stderr
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	// Validate rigs (prefix collisions, missing fields).
	if err := config.ValidateRigs(cfg.Rigs, cityName); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := config.ValidateServices(cfg.Services); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := workspacesvc.ValidateRuntimeSupport(cfg.Services); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Materialize the gc-beads-bd script so the exec: provider can use it.
	if _, err := MaterializeBeadsBdScript(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: materializing gc-beads-bd: %v\n", err) //nolint:errcheck // best-effort stderr
		// Non-fatal: only needed if provider = "bd".
	}

	// Materialize builtin packs (bd + dolt) so doctor checks and commands are available.
	if err := MaterializeBuiltinPacks(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: materializing builtin packs: %v\n", err) //nolint:errcheck // best-effort stderr
		// Non-fatal: only needed if provider = "bd".
	}
	injectBuiltinPacks(cfg, cityPath)

	// Materialize builtin prompts and formulas to stay in sync with binary.
	if err := materializeBuiltinPrompts(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: builtin prompts: %v\n", err) //nolint:errcheck // best-effort stderr
	}
	if err := materializeBuiltinFormulas(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: builtin formulas: %v\n", err) //nolint:errcheck // best-effort stderr
	}

	// Resolve rig paths and run the full bead store lifecycle:
	// ensure-ready → init+hooks(city) → init+hooks(rigs) → routes.
	resolveRigPaths(cityPath, cfg.Rigs)
	if err := startBeadsLifecycle(cityPath, cityName, cfg, stderr); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Post-startup health check: baseline probe of the beads provider.
	// The gc-beads-bd script's health operation validates server liveness
	// (TCP + query probe). Recovery is attempted on failure.
	if err := healthBeadsProvider(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc start: beads health check: %v\n", err) //nolint:errcheck // best-effort stderr
		// Non-fatal warning — server may recover by the time agents need it.
	}

	// Materialize system formulas from binary.
	sysDir, sysErr := MaterializeSystemFormulas(systemFormulasFS, "system_formulas", cityPath)
	if sysErr != nil {
		fmt.Fprintf(stderr, "gc start: system formulas: %v\n", sysErr) //nolint:errcheck // best-effort stderr
	}
	if sysDir != "" {
		// Prepend as Layer 0 (lowest priority).
		cfg.FormulaLayers.City = append([]string{sysDir}, cfg.FormulaLayers.City...)
		for rigName, layers := range cfg.FormulaLayers.Rigs {
			cfg.FormulaLayers.Rigs[rigName] = append([]string{sysDir}, layers...)
		}
	}

	// Materialize formula symlinks before agent startup.
	if len(cfg.FormulaLayers.City) > 0 {
		if err := ResolveFormulas(cityPath, cfg.FormulaLayers.City); err != nil {
			fmt.Fprintf(stderr, "gc start: city formulas: %v\n", err) //nolint:errcheck // best-effort stderr
		}
	}
	for _, r := range cfg.Rigs {
		if layers, ok := cfg.FormulaLayers.Rigs[r.Name]; ok && len(layers) > 0 {
			if err := ResolveFormulas(r.Path, layers); err != nil {
				fmt.Fprintf(stderr, "gc start: rig %q formulas: %v\n", r.Name, err) //nolint:errcheck // best-effort stderr
			}
		}
	}

	// Materialize Claude skill stubs (after formulas, before agent startup).
	if cfg.Workspace.Provider == "claude" {
		dirs := []string{cityPath}
		for _, r := range cfg.Rigs {
			if r.Path != "" {
				dirs = append(dirs, r.Path)
			}
		}
		if err := materializeSkillStubs(dirs...); err != nil {
			fmt.Fprintf(stderr, "gc start: skill stubs: %v\n", err) //nolint:errcheck // best-effort stderr
			// Non-fatal.
		}
	}

	// Validate agents.
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate install_agent_hooks (workspace + all agents).
	if ih := cfg.Workspace.InstallAgentHooks; len(ih) > 0 {
		if err := hooks.Validate(ih); err != nil {
			fmt.Fprintf(stderr, "gc start: workspace: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	for _, a := range cfg.Agents {
		if len(a.InstallAgentHooks) > 0 {
			if err := hooks.Validate(a.InstallAgentHooks); err != nil {
				fmt.Fprintf(stderr, "gc start: agent %q: %v\n", a.QualifiedName(), err) //nolint:errcheck // best-effort stderr
				return 1
			}
		}
	}

	sp := newSessionProvider()

	// beaconTime is captured once so the beacon timestamp remains stable
	// across reconcile ticks. Without this, FormatBeacon(time.Now()) would
	// produce a different command string each tick, causing
	// ConfigFingerprint to detect spurious drift and restart all agents.
	beaconTime := time.Now()

	// buildAgents constructs the desired agent list from the given config.
	// Called once for one-shot, or on each tick for controller mode.
	// Pool check commands are re-evaluated each call. Accepts a *config.City
	// parameter so the controller loop can pass freshly-reloaded config.
	buildAgents := func(c *config.City, currentSP runtime.Provider, store beads.Store) map[string]TemplateParams {
		return buildDesiredState(cityName, cityPath, beaconTime, c, currentSP, store, stderr)
	}

	recorder := events.Discard
	var eventProv events.Provider // nil when events disabled or FileRecorder fails
	if fr, err := events.NewFileRecorder(
		filepath.Join(cityPath, ".gc", "events.jsonl"), stderr); err == nil {
		recorder = fr
		eventProv = fr
	}

	// Pre-check container images once (fail fast before N serial starts).
	if err := checkAgentImages(sp, cfg.Agents, stderr); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// --dry-run: build agents and print preview without starting.
	if dryRunMode {
		agents := buildAgents(cfg, sp, nil)
		printDryRunPreview(agents, cfg, cityName, stdout)
		return 0
	}

	tomlPath := filepath.Join(cityPath, "city.toml")
	if controllerMode {
		poolSessions := computePoolSessions(cfg, cityName, cityPath, sp)
		poolDeathHandlers := computePoolDeathHandlers(cfg, cityName, cityPath, sp)
		watchDirs := config.WatchDirs(prov, cfg, cityPath)
		return runController(cityPath, tomlPath, cfg, buildAgents, sp,
			newDrainOps(sp), poolSessions, poolDeathHandlers, watchDirs, recorder, eventProv, stdout, stderr)
	}

	// One-shot reconciliation (default): no drain (kill is fine).
	// Create a signal-aware context so Ctrl-C cancels in-flight starts.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Enforce restrictive permissions on .gc/ and its subdirectories.
	enforceGCPermissions(cityPath, stderr)

	runPoolOnBoot(cfg, cityPath, shellScaleCheck, stderr)
	rops := newReconcileOps(sp)
	// Upgrade to bead-driven rops so one-shot writes hashes to the same
	// store as the daemon, preventing false drift on next daemon start.
	var oneShotStore beads.Store
	if store, err := openCityStoreAt(cityPath); err == nil {
		oneShotStore = store

		// Run adoption barrier before sync.
		result, passed := runAdoptionBarrier(store, sp, cfg, cityName, clock.Real{}, stderr, false)
		if result.Adopted > 0 {
			fmt.Fprintf(stdout, "Adopted %d running session(s) into bead store.\n", result.Adopted) //nolint:errcheck
		}
		if !passed && result.Skipped > 0 {
			fmt.Fprintf(stderr, "adoption barrier: %d session(s) failed bead creation\n", result.Skipped) //nolint:errcheck
		}

		cfgNames := configuredSessionNames(cfg, cityName, store)
		ds := buildDesiredState(cityName, cityPath, beaconTime, cfg, sp, store, stderr)
		idx := syncSessionBeads(store, ds, sp, cfgNames, cfg, clock.Real{}, stderr, false)
		if idx != nil {
			bro := newBeadReconcileOps(rops, func() beads.Store { return store })
			bro.updateIndex(idx)
			rops = bro
		}
	} else {
		fmt.Fprintf(stderr, "gc start: bead store unavailable, using provider hashes: %v\n", err) //nolint:errcheck
	}
	agents := buildAgents(cfg, sp, oneShotStore)
	suspendedNames := computeSuspendedNames(cfg, cityName, cityPath)
	code := doReconcileAgents(agents, sp, rops, nil, nil, nil, recorder, nil, suspendedNames, 0, cfg.Session.StartupTimeoutDuration(), stdout, stderr, sigCtx)
	// Post-reconcile sync: update bead state to reflect post-start reality.
	if oneShotStore != nil {
		cfgNames := configuredSessionNames(cfg, cityName, oneShotStore)
		ds := buildDesiredState(cityName, cityPath, beaconTime, cfg, sp, oneShotStore, stderr)
		syncSessionBeads(oneShotStore, ds, sp, cfgNames, cfg, clock.Real{}, stderr, false)
	}
	if code == 0 {
		fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	}
	return code
}

// printDryRunPreview prints what agents would be started without starting them.
func printDryRunPreview(desiredState map[string]TemplateParams, cfg *config.City, cityName string, stdout io.Writer) {
	fmt.Fprintf(stdout, "Dry-run: %d agent(s) would start in city %q\n\n", len(desiredState), cityName) //nolint:errcheck // best-effort stdout

	if len(desiredState) == 0 {
		fmt.Fprintln(stdout, "  (no agents to start)") //nolint:errcheck // best-effort stdout
		return
	}

	sortedNames := make([]string, 0, len(desiredState))
	for sn := range desiredState {
		sortedNames = append(sortedNames, sn)
	}
	sort.Strings(sortedNames)
	for _, sn := range sortedNames {
		tp := desiredState[sn]
		fmt.Fprintf(stdout, "  %-30s  session=%s\n", tp.DisplayName(), sn) //nolint:errcheck // best-effort stdout
	}

	// Summary by suspension.
	var suspended int
	for _, a := range cfg.Agents {
		if a.Suspended {
			suspended++
		}
	}
	fmt.Fprintln(stdout) //nolint:errcheck // best-effort stdout
	if suspended > 0 {
		fmt.Fprintf(stdout, "  %d agent(s) suspended (not shown above)\n", suspended) //nolint:errcheck // best-effort stdout
	}
	fmt.Fprintln(stdout, "No side effects executed (--dry-run).") //nolint:errcheck // best-effort stdout
}

// settingsArgs returns "--settings <path>" to append to a Claude command
// if settings.json exists for this city. Uses a path relative to the session
// working directory so it works for both local and remote providers (the
// .gc directory is staged via CopyFiles).
// Returns empty string for non-Claude providers or if no settings file is present.
func settingsArgs(cityPath, providerName string) string {
	if providerName != "claude" {
		return ""
	}
	settingsPath := citylayout.ResolveReadPath(fsys.OSFS{}, cityPath, citylayout.ClaudeHookFile)
	if _, err := os.Stat(settingsPath); err != nil {
		return ""
	}
	return "--settings .gc/settings.json"
}

// stageHookFiles adds hook files installed by hooks.Install() to the
// copy_files list so container providers (K8s) can stage them into pods.
// Docker doesn't need this (bind-mount), but the extra entries are harmless.
// Avoids duplicating .gc/settings.json if settingsArgs already added it.
func stageHookFiles(copyFiles []runtime.CopyEntry, cityPath, workDir string) []runtime.CopyEntry {
	// workDir-based hooks: gemini, codex, opencode, copilot, pi, omp.
	for _, rel := range []string{
		filepath.Join(".gemini", "settings.json"),
		filepath.Join(".codex", "hooks.json"),
		filepath.Join(".opencode", "plugins", "gascity.js"),
		filepath.Join(".github", "hooks", "gascity.json"),
		filepath.Join(".github", "copilot-instructions.md"),
		filepath.Join(".pi", "extensions", "gc-hooks.js"),
		filepath.Join(".omp", "hooks", "gc-hook.ts"),
	} {
		abs := filepath.Join(workDir, rel)
		if _, err := os.Stat(abs); err == nil {
			copyFiles = append(copyFiles, runtime.CopyEntry{Src: abs, RelDst: rel})
		}
	}
	// Stage Claude skills directory (if materialized).
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
		copyFiles = append(copyFiles, runtime.CopyEntry{
			Src: skillsDir, RelDst: filepath.Join(".claude", "skills"),
		})
	}
	// cityDir-based hooks: claude (.gc/settings.json).
	// Skip if settingsArgs already added it.
	settingsRel := filepath.Join(".gc", "settings.json")
	settingsAbs := citylayout.ResolveReadPath(fsys.OSFS{}, cityPath, citylayout.ClaudeHookFile)
	if _, err := os.Stat(settingsAbs); err == nil {
		alreadyStaged := false
		for _, cf := range copyFiles {
			if cf.RelDst == settingsRel {
				alreadyStaged = true
				break
			}
		}
		if !alreadyStaged {
			copyFiles = append(copyFiles, runtime.CopyEntry{Src: settingsAbs, RelDst: settingsRel})
		}
	}
	return copyFiles
}

// resolveAgentDir returns the absolute working directory for an agent.
// Empty dir defaults to cityPath. Relative paths resolve against cityPath.
// Creates the directory if it doesn't exist.
func resolveAgentDir(cityPath, dir string) (string, error) {
	if dir == "" {
		return cityPath, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cityPath, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating agent dir %q: %w", dir, err)
	}
	return dir, nil
}

// passthroughEnv returns environment variables from the parent process that
// agent sessions should inherit. Agents need PATH to find tools (including gc),
// GC_BEADS/GC_DOLT so they use the same bead store as the parent, and
// GC_DOLT_HOST/PORT/USER/PASSWORD so agents can connect to remote Dolt servers.
func passthroughEnv() map[string]string {
	m := make(map[string]string)
	// Pass through PATH and all GC_* environment variables so provider
	// configs (Docker, K8s, beads, dolt, etc.) propagate to agents.
	if v := os.Getenv("PATH"); v != "" {
		m["PATH"] = v
	}
	for _, entry := range os.Environ() {
		if key, val, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(key, "GC_") && val != "" {
			m[key] = val
		}
	}
	// Propagate OTel env vars so agent subprocesses emit telemetry.
	for k, v := range telemetry.OTELEnvMap() {
		m[k] = v
	}
	// Strip Claude nesting-detection vars so agents don't refuse to start
	// when gc is run from inside a Claude Code session.
	for _, k := range []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"} {
		if os.Getenv(k) != "" {
			m[k] = ""
		}
	}
	return m
}

// expandEnvMap returns a copy of m with os.ExpandEnv applied to each value.
// This allows TOML-sourced env blocks to reference the controller's environment,
// e.g. DOLTHUB_TOKEN = "$DOLTHUB_TOKEN".
func expandEnvMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = os.ExpandEnv(v)
	}
	return out
}

// mergeEnv combines multiple env maps into one. Later maps override earlier
// ones for the same key. Returns nil if all inputs are empty.
func mergeEnv(maps ...map[string]string) map[string]string {
	size := 0
	for _, m := range maps {
		size += len(m)
	}
	if size == 0 {
		return nil
	}
	out := make(map[string]string, size)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// resolveRigForAgent returns the rig name for an agent based on its working
// directory. Returns empty string if the agent is not scoped to any rig.
// Paths are cleaned before comparison to handle trailing slashes and
// redundant separators.
func resolveRigForAgent(workDir string, rigs []config.Rig) string {
	cleanWork := filepath.Clean(workDir)
	for _, r := range rigs {
		if cleanWork == filepath.Clean(r.Path) {
			return r.Name
		}
	}
	return ""
}

// resolveOverlayDir resolves an overlay_dir path relative to cityPath.
// Returns the path unchanged if already absolute, or empty if not set.
func resolveOverlayDir(dir, cityPath string) string {
	if dir == "" || filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(cityPath, dir)
}

// imageChecker is implemented by session providers that support pre-checking
// container images (e.g., exec provider for Docker). Providers that don't
// support it simply don't implement this interface — checkAgentImages is a
// no-op for them.
type imageChecker interface {
	CheckImage(image string) error
}

// checkAgentImages verifies that all unique container images referenced by
// agents exist locally. Called once before the reconcile loop to fail fast
// instead of discovering a missing image after N serial start timeouts.
// Returns nil if the provider doesn't support image checking.
func checkAgentImages(sp runtime.Provider, agents []config.Agent, _ io.Writer) error {
	ic, ok := sp.(imageChecker)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for _, a := range agents {
		img := a.Env["GC_DOCKER_IMAGE"]
		if img == "" || seen[img] {
			continue
		}
		seen[img] = true
		if err := ic.CheckImage(img); err != nil {
			return fmt.Errorf("image pre-check: %w", err)
		}
	}
	return nil
}

// countRunningPoolInstances counts how many pool instances are currently
// running for a given pool agent. For bounded pools, checks static names
// (1..max). For unlimited pools, discovers via prefix matching.
//
// Uses ListRunning with the city prefix for a single batch call instead
// of N individual IsRunning calls. For exec providers (K8s), this reduces
// N subprocess spawns to 1.
func countRunningPoolInstances(agentName, agentDir string, pool config.PoolConfig, cityName, sessionTemplate string, sp runtime.Provider) int { //nolint:unparam // agentName varies in production use
	if pool.IsUnlimited() {
		// Unlimited: count by prefix matching.
		instances := discoverPoolInstances(agentName, agentDir, pool, cityName, sessionTemplate, sp)
		count := 0
		for _, qn := range instances {
			sn := sessionName(nil, cityName, qn, sessionTemplate)
			if sp.IsRunning(sn) {
				count++
			}
		}
		return count
	}

	// Bounded: build the set of expected pool instance session names.
	expected := make(map[string]bool, pool.Max)
	for i := 1; i <= pool.Max; i++ {
		instanceName := fmt.Sprintf("%s-%d", agentName, i)
		qualifiedInstance := instanceName
		if agentDir != "" {
			qualifiedInstance = agentDir + "/" + instanceName
		}
		expected[sessionName(nil, cityName, qualifiedInstance, sessionTemplate)] = true
	}

	// Single ListRunning call, then intersect with expected set.
	// Per-city socket isolation: all sessions belong to this city.
	running, err := sp.ListRunning("")
	if err != nil {
		// Fallback: individual IsRunning calls (original behavior).
		count := 0
		for sn := range expected {
			if sp.IsRunning(sn) {
				count++
			}
		}
		return count
	}

	count := 0
	for _, name := range running {
		if expected[name] {
			count++
		}
	}
	return count
}

// buildFingerprintExtra builds the fpExtra map for an agent's fingerprint
// from its config. Returns nil if no extra fields are present.
func buildFingerprintExtra(a *config.Agent) map[string]string {
	m := make(map[string]string)
	if a.Pool != nil {
		m["pool.min"] = strconv.Itoa(a.Pool.Min)
		m["pool.max"] = strconv.Itoa(a.Pool.Max)
		if a.Pool.Check != "" {
			m["pool.check"] = a.Pool.Check
		}
	}
	if len(a.DependsOn) > 0 {
		m["depends_on"] = strings.Join(a.DependsOn, ",")
	}
	if a.WakeMode != "" && a.WakeMode != "resume" {
		m["wake_mode"] = a.WakeMode
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
