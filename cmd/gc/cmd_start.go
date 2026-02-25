package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

// computeSuspendedNames builds a set of session names for agents marked
// suspended in the config or belonging to suspended rigs. Used by the
// reconciler to distinguish suspended agents from true orphans during
// Phase 2 cleanup.
func computeSuspendedNames(cfg *config.City, cityName, cityPath string) map[string]bool {
	names := make(map[string]bool)
	st := cfg.Workspace.SessionTemplate
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

func newStartCmd(stdout, stderr io.Writer) *cobra.Command {
	var controllerMode bool
	cmd := &cobra.Command{
		Use:   "start [path]",
		Short: "Start the city (auto-initializes if needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doStart(args, controllerMode, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&controllerMode, "controller", false,
		"run as a persistent controller (reconcile loop)")
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
		initBeads(dir, dirName, stderr)
	}

	// Load config to find agents.
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	// Ensure dolt server is running if using bd provider.
	if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
		if err := dolt.EnsureRunning(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc start: dolt: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	// Validate rigs (prefix collisions, missing fields).
	if err := config.ValidateRigs(cfg.Rigs, cityName); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Initialize beads for all rigs and regenerate routes.
	if len(cfg.Rigs) > 0 {
		if code := initAllRigBeads(cityPath, cfg, stderr); code != 0 {
			return code
		}
	}

	// Validate agents.
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	sp := newSessionProvider()

	// buildAgents constructs the desired agent list from the given config.
	// Called once for one-shot, or on each tick for controller mode.
	// Pool check commands are re-evaluated each call. Accepts a *config.City
	// parameter so the controller loop can pass freshly-reloaded config.
	buildAgents := func(c *config.City) []agent.Agent {
		// Pre-compute suspended rig paths so we can skip agents in suspended rigs.
		suspendedRigPaths := make(map[string]bool)
		for _, r := range c.Rigs {
			if r.Suspended {
				suspendedRigPaths[filepath.Clean(r.Path)] = true
			}
		}

		var agents []agent.Agent
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
					fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].Name, err) //nolint:errcheck // best-effort stderr
					continue
				}
				workDir, err := resolveAgentDir(cityPath, c.Agents[i].Dir)
				if err != nil {
					fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].Name, err) //nolint:errcheck // best-effort stderr
					continue
				}
				if suspendedRigPaths[filepath.Clean(workDir)] {
					continue // Agent's rig is suspended — skip.
				}

				// Worktree isolation: create per-agent worktree from rig repo.
				var wtBranch, wtRig string
				if c.Agents[i].Isolation == "worktree" {
					rn, rp, found := findRigByDir(workDir, c.Rigs)
					if !found {
						fmt.Fprintf(stderr, "gc start: agent %q: isolation \"worktree\" but dir does not match a rig (skipping)\n", c.Agents[i].Name) //nolint:errcheck // best-effort stderr
						continue
					}
					wt, br, wtErr := createAgentWorktree(rp, cityPath, rn, c.Agents[i].Name)
					if wtErr != nil {
						fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].Name, wtErr) //nolint:errcheck // best-effort stderr
						continue
					}
					if rdErr := setupBeadsRedirect(wt, rp); rdErr != nil {
						fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", c.Agents[i].Name, rdErr) //nolint:errcheck // best-effort stderr
						continue
					}
					workDir = wt
					wtBranch = br
					wtRig = rn
				}

				command := resolved.CommandString()
				rigName := wtRig
				if rigName == "" {
					rigName = resolveRigForAgent(workDir, c.Rigs)
				}
				prompt := renderPrompt(fsys.OSFS{}, cityPath, cityName, c.Agents[i].PromptTemplate, PromptContext{
					CityRoot:     cityPath,
					AgentName:    c.Agents[i].QualifiedName(),
					TemplateName: c.Agents[i].Name,
					RigName:      rigName,
					WorkDir:      workDir,
					IssuePrefix:  findRigPrefix(rigName, c.Rigs),
					Branch:       wtBranch,
					Env:          c.Agents[i].Env,
				}, c.Workspace.SessionTemplate, stderr)
				agentEnv := map[string]string{
					"GC_AGENT": c.Agents[i].QualifiedName(),
					"GC_CITY":  cityPath,
					"GC_DIR":   workDir,
				}
				if rigName != "" {
					agentEnv["GC_RIG"] = rigName
				}
				if wtBranch != "" {
					agentEnv["GC_BRANCH"] = wtBranch
				}
				env := mergeEnv(passthroughEnv(), resolved.Env, agentEnv)
				hints := agent.StartupHints{
					ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
					ReadyDelayMs:           resolved.ReadyDelayMs,
					ProcessNames:           resolved.ProcessNames,
					EmitsPermissionWarning: resolved.EmitsPermissionWarning,
				}
				agents = append(agents, agent.New(c.Agents[i].QualifiedName(), cityName, command, prompt, env, hints, workDir, c.Workspace.SessionTemplate, sp))
				continue
			}

			// Pool agent (explicit [agents.pool] or implicit singleton with pool config).
			// Check rig suspension before evaluating pool to avoid wasted work.
			if c.Agents[i].Dir != "" {
				poolDir, pdErr := resolveAgentDir(cityPath, c.Agents[i].Dir)
				if pdErr == nil && suspendedRigPaths[filepath.Clean(poolDir)] {
					continue // Agent's rig is suspended — skip.
				}
			}
			desired, err := evaluatePool(c.Agents[i].Name, pool, shellScaleCheck)
			if err != nil {
				fmt.Fprintf(stderr, "gc start: %v (using min=%d)\n", err, pool.Min) //nolint:errcheck // best-effort stderr
			}
			pa, err := poolAgents(&c.Agents[i], desired, cityName, cityPath,
				&c.Workspace, c.Providers, exec.LookPath, fsys.OSFS{}, sp, c.Rigs, c.Workspace.SessionTemplate)
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

	tomlPath := filepath.Join(cityPath, "city.toml")
	if controllerMode {
		poolSessions := computePoolSessions(cfg, cityName)
		return runController(cityPath, tomlPath, cfg, buildAgents, sp,
			newDrainOps(sp), poolSessions, recorder, stdout, stderr)
	}

	// One-shot reconciliation (default): no drain (kill is fine).
	agents := buildAgents(cfg)
	cityPrefix := "gc-" + cityName + "-"
	rops := newReconcileOps(sp)
	code := doReconcileAgents(agents, sp, rops, nil, nil, recorder, cityPrefix, nil, nil, stdout, stderr)
	if code == 0 {
		fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	}
	return code
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
// and GC_BEADS/GC_DOLT so they use the same bead store as the parent.
func passthroughEnv() map[string]string {
	m := make(map[string]string)
	for _, key := range []string{"PATH", "GC_BEADS", "GC_DOLT"} {
		if v := os.Getenv(key); v != "" {
			m[key] = v
		}
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

// initAllRigBeads initializes beads for all configured rigs and regenerates
// routes.jsonl for cross-rig routing. Called during gc start when rigs are
// configured. Each rig gets its own .beads/ database with a unique prefix.
func initAllRigBeads(cityPath string, cfg *config.City, stderr io.Writer) int {
	isBd := beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip"

	// Init beads for each rig (idempotent).
	if isBd {
		for i := range cfg.Rigs {
			prefix := cfg.Rigs[i].EffectivePrefix()
			if err := dolt.InitRigBeads(cfg.Rigs[i].Path, prefix); err != nil {
				fmt.Fprintf(stderr, "gc start: init rig %q beads: %v\n", cfg.Rigs[i].Name, err) //nolint:errcheck // best-effort stderr
				return 1
			}
		}
	}

	// Regenerate routes for all rigs (HQ + configured rigs).
	allRigs := collectRigRoutes(cityPath, cfg)
	if err := writeAllRoutes(allRigs); err != nil {
		fmt.Fprintf(stderr, "gc start: writing routes: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	return 0
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
