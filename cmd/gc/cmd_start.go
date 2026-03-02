package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/hooks"
	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/telemetry"
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
			names[agent.SessionNameFor(cityName, a.QualifiedName(), st)] = true
		}
		return names
	}

	// Individually suspended agents.
	for _, a := range cfg.Agents {
		if a.Suspended {
			qn := a.QualifiedName()
			names[agent.SessionNameFor(cityName, qn, st)] = true
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
				names[agent.SessionNameFor(cityName, a.QualifiedName(), st)] = true
			}
		}
	}
	return names
}

// computePoolSessions builds the set of ALL possible pool session names
// (1..max) for every pool agent in the config, mapped to the pool's drain
// timeout. Used to distinguish excess pool members (drain) from true orphans
// (kill) during reconciliation, and to enforce drain timeouts.
func computePoolSessions(cfg *config.City, cityName string) map[string]time.Duration {
	ps := make(map[string]time.Duration)
	st := cfg.Workspace.SessionTemplate
	for _, a := range cfg.Agents {
		pool := a.EffectivePool()
		if !a.IsPool() || pool.Max <= 1 {
			continue
		}
		timeout := pool.DrainTimeoutDuration()
		for i := 1; i <= pool.Max; i++ {
			instanceName := fmt.Sprintf("%s-%d", a.Name, i)
			qualifiedInstance := instanceName
			if a.Dir != "" {
				qualifiedInstance = a.Dir + "/" + instanceName
			}
			ps[sessionName(cityName, qualifiedInstance, st)] = timeout
		}
	}
	return ps
}

// extraConfigFiles holds paths from -f flags for CLI-level file layering.
var extraConfigFiles []string

// strictMode promotes composition collision warnings to errors.
var strictMode bool

// buildIdleTracker creates an idleTracker from the config, populating
// timeouts for agents that have idle_timeout set. Returns nil if no
// agents use idle timeout (disabled).
func buildIdleTracker(cfg *config.City, cityName string, sp session.Provider) idleTracker {
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
	it := newIdleTracker(sp)
	for _, a := range cfg.Agents {
		timeout := a.IdleTimeoutDuration()
		if timeout > 0 {
			sn := agent.SessionNameFor(cityName, a.QualifiedName(), st)
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
topologies, resolves providers, installs hooks, and starts agent sessions
via one-shot reconciliation. Use --foreground for a persistent controller
that continuously reconciles agent state.`,
		Example: `  gc start
  gc start ~/my-city
  gc start --foreground
  gc start -f overlay.toml --strict`,
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
	cmd.Flags().BoolVar(&strictMode, "strict", false,
		"promote config collision warnings to errors")
	return cmd
}

// doStart boots the city. If a path is given, operates there; otherwise uses
// cwd. If no city exists at the target, it auto-initializes one first via
// doInit, then starts all configured agent sessions. When controllerMode is
// true, enters a persistent reconciliation loop instead of one-shot start.
func doStart(args []string, controllerMode bool, stdout, stderr io.Writer) int {
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
	// Auto-fetch remote topologies before full config load.
	if quickCfg, qErr := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml")); qErr == nil && len(quickCfg.Topologies) > 0 {
		if fErr := config.FetchTopologies(quickCfg.Topologies, cityPath); fErr != nil {
			fmt.Fprintf(stderr, "gc start: fetching topologies: %v\n", fErr) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	cfg, prov, err := config.LoadWithIncludes(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"), extraConfigFiles...)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	// --strict promotes composition warnings to errors.
	if strictMode && len(prov.Warnings) > 0 {
		for _, w := range prov.Warnings {
			fmt.Fprintf(stderr, "gc start: strict: %s\n", w) //nolint:errcheck // best-effort stderr
		}
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

	// Resolve rig paths and run the full bead store lifecycle:
	// ensure-ready → init+hooks(city) → init+hooks(rigs) → routes.
	resolveRigPaths(cityPath, cfg.Rigs)
	if err := startBeadsLifecycle(cityPath, cityName, cfg, stderr); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Verify that the Dolt server is actually serving the expected databases.
	// EnsureRunning only checks TCP reachability; this catches stale servers
	// or databases that failed to load.
	if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
		_, missing, verifyErr := dolt.VerifyDatabasesCityWithRetry(cityPath, 3)
		if verifyErr != nil {
			fmt.Fprintf(stderr, "gc start: database verification failed: %v\n", verifyErr) //nolint:errcheck // best-effort stderr
			return 1
		}
		if len(missing) > 0 {
			fmt.Fprintf(stderr, "gc start: Dolt server is not serving expected databases: %s\n", strings.Join(missing, ", ")) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	// Post-startup health check: baseline probe of the beads provider.
	// VerifyDatabasesWithRetry validates database content (stale servers,
	// missing DBs); this validates server liveness (TCP + query + write).
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
	buildAgents := func(c *config.City) []agent.Agent {
		// City-level suspension: no agents should start.
		if c.Workspace.Suspended {
			return nil
		}

		// Pre-compute suspended rig paths so we can skip agents in suspended rigs.
		suspendedRigPaths := make(map[string]bool)
		for _, r := range c.Rigs {
			if r.Suspended {
				suspendedRigPaths[filepath.Clean(r.Path)] = true
			}
		}

		// poolEvalWork collects pool agents for parallel scale_check evaluation.
		type poolEvalWork struct {
			agentIdx int
			pool     config.PoolConfig
			poolDir  string
		}

		var agents []agent.Agent
		var pendingPools []poolEvalWork
		for i := range c.Agents {
			if c.Agents[i].Suspended {
				continue // Suspended agent — skip until resumed.
			}

			pool := c.Agents[i].EffectivePool()

			if pool.Max == 0 {
				continue // Disabled agent.
			}

			if pool.Max == 1 && !c.Agents[i].IsPool() {
				// Fixed agent (no explicit pool config): resolve and build single agent.
				resolved, err := config.ResolveProvider(&c.Agents[i], &c.Workspace, c.Providers, exec.LookPath)
				if err != nil {
					fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].QualifiedName(), err) //nolint:errcheck // best-effort stderr
					continue
				}
				// Expand dir templates (e.g. ".gc/worktrees/{{.Rig}}/{{.Agent}}").
				expandedDir := expandDirTemplate(c.Agents[i].Dir, SessionSetupContext{
					Agent:    c.Agents[i].QualifiedName(),
					Rig:      c.Agents[i].Dir,
					CityRoot: cityPath,
					CityName: cityName,
				})
				workDir, err := resolveAgentDir(cityPath, expandedDir)
				if err != nil {
					fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].QualifiedName(), err) //nolint:errcheck // best-effort stderr
					continue
				}
				if suspendedRigPaths[filepath.Clean(workDir)] {
					continue // Agent's rig is suspended — skip.
				}

				// Install provider hooks if configured.
				if ih := config.ResolveInstallHooks(&c.Agents[i], &c.Workspace); len(ih) > 0 {
					if hErr := hooks.Install(fsys.OSFS{}, cityPath, workDir, ih); hErr != nil {
						fmt.Fprintf(stderr, "gc start: agent %q: hooks: %v\n", c.Agents[i].QualifiedName(), hErr) //nolint:errcheck // best-effort stderr
					}
				}

				// Resolve overlay directory path (provider handles the copy).
				overlayDir := resolveOverlayDir(c.Agents[i].OverlayDir, cityPath)

				// Stage settings.json when referenced so the agent can find it.
				var copyFiles []session.CopyEntry
				command := resolved.CommandString()
				if sa := settingsArgs(cityPath, resolved.Name); sa != "" {
					command = command + " " + sa
					settingsFile := filepath.Join(cityPath, ".gc", "settings.json")
					copyFiles = append(copyFiles, session.CopyEntry{Src: settingsFile, RelDst: filepath.Join(".gc", "settings.json")})
				}
				// Stage .gc/scripts/ if present so agent scripts resolve inside
				// container providers (K8s, Docker) that don't bind-mount host paths.
				scriptsDir := filepath.Join(cityPath, ".gc", "scripts")
				if info, err := os.Stat(scriptsDir); err == nil && info.IsDir() {
					copyFiles = append(copyFiles, session.CopyEntry{Src: scriptsDir, RelDst: filepath.Join(".gc", "scripts")})
				}
				// Stage hook files for container providers (K8s uses emptyDir,
				// not bind-mounts). hooks.Install() writes to host filesystem;
				// container pods need explicit staging via copy_files.
				copyFiles = stageHookFiles(copyFiles, cityPath, workDir)
				rigName := resolveRigForAgent(workDir, c.Rigs)
				var prompt string
				if resolved.PromptMode != "none" {
					fragments := mergeFragmentLists(c.Workspace.GlobalFragments, c.Agents[i].InjectFragments)
					prompt = renderPrompt(fsys.OSFS{}, cityPath, cityName, c.Agents[i].PromptTemplate, PromptContext{
						CityRoot:      cityPath,
						AgentName:     c.Agents[i].QualifiedName(),
						TemplateName:  c.Agents[i].Name,
						RigName:       rigName,
						WorkDir:       workDir,
						IssuePrefix:   findRigPrefix(rigName, c.Rigs),
						DefaultBranch: defaultBranchFor(workDir),
						WorkQuery:     c.Agents[i].EffectiveWorkQuery(),
						SlingQuery:    c.Agents[i].EffectiveSlingQuery(),
						Env:           c.Agents[i].Env,
					}, c.Workspace.SessionTemplate, stderr,
						c.TopologySharedDirs, fragments)
					hasHooks := config.AgentHasHooks(&c.Agents[i], &c.Workspace, resolved.Name)
					beacon := session.FormatBeaconAt(cityName, c.Agents[i].QualifiedName(), !hasHooks, beaconTime)
					if prompt != "" {
						prompt = beacon + "\n\n" + prompt
					} else {
						prompt = beacon
					}
				}
				agentEnv := map[string]string{
					"GC_AGENT": c.Agents[i].QualifiedName(),
					"GC_CITY":  cityPath,
					"GC_DIR":   workDir,
				}
				if rigName != "" {
					agentEnv["GC_RIG"] = rigName
				}
				env := mergeEnv(passthroughEnv(), resolved.Env, c.Agents[i].Env, agentEnv)
				// Expand session_setup templates with session context.
				sessName := sessionName(cityName, c.Agents[i].QualifiedName(), c.Workspace.SessionTemplate)
				configDir := cityPath
				if c.Agents[i].SourceDir != "" {
					configDir = c.Agents[i].SourceDir
				}
				// Expand Go templates in start_command (e.g. {{.ConfigDir}}).
				if strings.Contains(command, "{{") {
					expanded := expandSessionSetup([]string{command}, SessionSetupContext{
						Session:   sessName,
						Agent:     c.Agents[i].QualifiedName(),
						Rig:       rigName,
						CityRoot:  cityPath,
						CityName:  cityName,
						WorkDir:   workDir,
						ConfigDir: configDir,
					})
					command = expanded[0]
				}
				expandedSetup := expandSessionSetup(c.Agents[i].SessionSetup, SessionSetupContext{
					Session:   sessName,
					Agent:     c.Agents[i].QualifiedName(),
					Rig:       rigName,
					CityRoot:  cityPath,
					CityName:  cityName,
					WorkDir:   workDir,
					ConfigDir: configDir,
				})
				resolvedScript := resolveSetupScript(c.Agents[i].SessionSetupScript, cityPath)
				expandedPreStart := expandSessionSetup(c.Agents[i].PreStart, SessionSetupContext{
					Session:   sessName,
					Agent:     c.Agents[i].QualifiedName(),
					Rig:       rigName,
					CityRoot:  cityPath,
					CityName:  cityName,
					WorkDir:   workDir,
					ConfigDir: configDir,
				})
				hints := agent.StartupHints{
					ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
					ReadyDelayMs:           resolved.ReadyDelayMs,
					ProcessNames:           resolved.ProcessNames,
					EmitsPermissionWarning: resolved.EmitsPermissionWarning,
					Nudge:                  c.Agents[i].Nudge,
					PreStart:               expandedPreStart,
					SessionSetup:           expandedSetup,
					SessionSetupScript:     resolvedScript,
					OverlayDir:             overlayDir,
					CopyFiles:              copyFiles,
				}
				fpExtra := buildFingerprintExtra(&c.Agents[i])
				agents = append(agents, agent.New(c.Agents[i].QualifiedName(), cityName, command, prompt, env, hints, workDir, c.Workspace.SessionTemplate, fpExtra, sp))
				continue
			}

			// Pool agent (explicit [agents.pool] or implicit singleton with pool config).
			// Collect for parallel scale_check evaluation below.
			if c.Agents[i].Dir != "" {
				poolDir, pdErr := resolveAgentDir(cityPath, c.Agents[i].Dir)
				if pdErr == nil && suspendedRigPaths[filepath.Clean(poolDir)] {
					continue // Agent's rig is suspended — skip.
				}
			}
			// Resolve pool working directory for scale_check context.
			poolDir := cityPath
			if c.Agents[i].Dir != "" {
				if pd, pdErr := resolveAgentDir(cityPath, c.Agents[i].Dir); pdErr == nil {
					poolDir = pd
				}
			}
			pendingPools = append(pendingPools, poolEvalWork{agentIdx: i, pool: pool, poolDir: poolDir})
		}

		// Run pool scale_check commands in parallel. Each check is an
		// independent shell command; running them concurrently reduces
		// wall-clock time from sum(check_durations) to max(check_duration).
		type poolEvalResult struct {
			desired int
			err     error
		}
		evalResults := make([]poolEvalResult, len(pendingPools))
		var wg sync.WaitGroup
		for j, pw := range pendingPools {
			wg.Add(1)
			go func(idx int, name string, pool config.PoolConfig, dir string) {
				defer wg.Done()
				desired, err := evaluatePool(name, pool, dir, shellScaleCheck)
				evalResults[idx] = poolEvalResult{desired: desired, err: err}
			}(j, c.Agents[pw.agentIdx].Name, pw.pool, pw.poolDir)
		}
		wg.Wait()

		// Process results sequentially (logging, counting, agent building).
		for j, pw := range pendingPools {
			pr := evalResults[j]
			if pr.err != nil {
				fmt.Fprintf(stderr, "gc start: %v (using min=%d)\n", pr.err, pw.pool.Min) //nolint:errcheck // best-effort stderr
			}
			running := countRunningPoolInstances(c.Agents[pw.agentIdx].Name, c.Agents[pw.agentIdx].Dir, pw.pool.Max, cityName, c.Workspace.SessionTemplate, sp)
			if pr.desired != running {
				fmt.Fprintf(stderr, "Pool '%s': check returned %d, %d running → scaling %s\n", //nolint:errcheck // best-effort stderr
					c.Agents[pw.agentIdx].Name, pr.desired, running, scaleDirection(running, pr.desired))
			}
			pa, err := poolAgents(&c.Agents[pw.agentIdx], pr.desired, cityName, cityPath,
				&c.Workspace, c.Providers, exec.LookPath, fsys.OSFS{}, sp, c.Rigs, c.Workspace.SessionTemplate, c.FormulaLayers, beaconTime,
				c.TopologySharedDirs, c.Workspace.GlobalFragments)
			if err != nil {
				fmt.Fprintf(stderr, "gc start: %v (skipping pool)\n", err) //nolint:errcheck // best-effort stderr
				continue
			}
			agents = append(agents, pa...)
		}
		return agents
	}

	recorder := events.Discard
	if fr, err := events.NewFileRecorder(
		filepath.Join(cityPath, ".gc", "events.jsonl"), stderr); err == nil {
		recorder = fr
	}

	// Pre-check container images once (fail fast before N serial starts).
	if err := checkAgentImages(sp, cfg.Agents, stderr); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	tomlPath := filepath.Join(cityPath, "city.toml")
	if controllerMode {
		poolSessions := computePoolSessions(cfg, cityName)
		watchDirs := config.WatchDirs(prov, cfg.Rigs, cityPath)
		return runController(cityPath, tomlPath, cfg, buildAgents, sp,
			newDrainOps(sp), poolSessions, watchDirs, recorder, stdout, stderr)
	}

	// One-shot reconciliation (default): no drain (kill is fine).
	agents := buildAgents(cfg)
	cityPrefix := "gc-" + cityName + "-"
	rops := newReconcileOps(sp)
	suspendedNames := computeSuspendedNames(cfg, cityName, cityPath)
	code := doReconcileAgents(agents, sp, rops, nil, nil, nil, recorder, cityPrefix, nil, suspendedNames, stdout, stderr)
	if code == 0 {
		fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	}
	return code
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
	settingsPath := filepath.Join(cityPath, ".gc", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		return ""
	}
	return "--settings .gc/settings.json"
}

// stageHookFiles adds hook files installed by hooks.Install() to the
// copy_files list so container providers (K8s) can stage them into pods.
// Docker doesn't need this (bind-mount), but the extra entries are harmless.
// Avoids duplicating .gc/settings.json if settingsArgs already added it.
func stageHookFiles(copyFiles []session.CopyEntry, cityPath, workDir string) []session.CopyEntry {
	// workDir-based hooks: gemini, opencode, copilot.
	for _, rel := range []string{
		filepath.Join(".gemini", "settings.json"),
		filepath.Join(".opencode", "plugins", "gascity.js"),
		filepath.Join(".github", "copilot-instructions.md"),
	} {
		abs := filepath.Join(workDir, rel)
		if _, err := os.Stat(abs); err == nil {
			copyFiles = append(copyFiles, session.CopyEntry{Src: abs, RelDst: rel})
		}
	}
	// cityDir-based hooks: claude (.gc/settings.json).
	// Skip if settingsArgs already added it.
	settingsRel := filepath.Join(".gc", "settings.json")
	settingsAbs := filepath.Join(cityPath, settingsRel)
	if _, err := os.Stat(settingsAbs); err == nil {
		alreadyStaged := false
		for _, cf := range copyFiles {
			if cf.RelDst == settingsRel {
				alreadyStaged = true
				break
			}
		}
		if !alreadyStaged {
			copyFiles = append(copyFiles, session.CopyEntry{Src: settingsAbs, RelDst: settingsRel})
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
	return m
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
func checkAgentImages(sp session.Provider, agents []config.Agent, _ io.Writer) error {
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

// countRunningPoolInstances counts how many pool instances (1..max) are
// currently running for a given pool agent. Used to log scaling decisions.
//
// Uses ListRunning with the city prefix for a single batch call instead
// of N individual IsRunning calls. For exec providers (K8s), this reduces
// N subprocess spawns to 1.
func countRunningPoolInstances(agentName, agentDir string, poolMax int, cityName, sessionTemplate string, sp session.Provider) int {
	// Build the set of expected pool instance session names.
	expected := make(map[string]bool, poolMax)
	for i := 1; i <= poolMax; i++ {
		instanceName := fmt.Sprintf("%s-%d", agentName, i)
		qualifiedInstance := instanceName
		if agentDir != "" {
			qualifiedInstance = agentDir + "/" + instanceName
		}
		expected[sessionName(cityName, qualifiedInstance, sessionTemplate)] = true
	}

	// Single ListRunning call with city prefix, then intersect with expected set.
	cityPrefix := "gc-" + cityName + "-"
	running, err := sp.ListRunning(cityPrefix)
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

// scaleDirection returns "up" or "down" based on current vs desired count.
func scaleDirection(running, desired int) string {
	if desired > running {
		return "up"
	}
	return "down"
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
	if len(m) == 0 {
		return nil
	}
	return m
}
