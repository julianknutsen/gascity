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

// computePoolSessions builds the set of ALL possible pool session names
// (1..max) for every pool agent in the config, mapped to the pool's drain
// timeout. Used to distinguish excess pool members (drain) from true orphans
// (kill) during reconciliation, and to enforce drain timeouts.
func computePoolSessions(cfg *config.City, cityName string) map[string]time.Duration {
	ps := make(map[string]time.Duration)
	for _, a := range cfg.Agents {
		pool := a.EffectivePool()
		if !a.IsPool() || pool.Max <= 1 {
			continue
		}
		timeout := pool.DrainTimeoutDuration()
		for i := 1; i <= pool.Max; i++ {
			name := fmt.Sprintf("%s-%d", a.Name, i)
			ps[sessionName(cityName, name)] = timeout
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
	if len(args) > 0 {
		var err error
		dir, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	if _, err := findCity(dir); err != nil {
		// No city found — auto-init at dir (non-interactive).
		if code := doInit(fsys.OSFS{}, dir, defaultWizardConfig(), stdout, stderr); code != 0 {
			return code
		}
		dirName := filepath.Base(dir)
		if code := initBeads(dir, dirName, stderr); code != 0 {
			return code
		}
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
		var agents []agent.Agent
		for i := range c.Agents {
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
				command := resolved.CommandString()
				sn := sessionName(cityName, c.Agents[i].Name)
				prompt := readPromptFile(fsys.OSFS{}, cityPath, c.Agents[i].PromptTemplate)
				agentEnv := map[string]string{
					"GC_AGENT": c.Agents[i].Name,
					"GC_CITY":  cityPath,
					"GC_DIR":   workDir,
				}
				if rigName := resolveRigForAgent(workDir, c.Rigs); rigName != "" {
					agentEnv["GC_RIG"] = rigName
				}
				env := mergeEnv(passthroughEnv(), resolved.Env, agentEnv)
				hints := agent.StartupHints{
					ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
					ReadyDelayMs:           resolved.ReadyDelayMs,
					ProcessNames:           resolved.ProcessNames,
					EmitsPermissionWarning: resolved.EmitsPermissionWarning,
				}
				agents = append(agents, agent.New(c.Agents[i].Name, sn, command, prompt, env, hints, workDir, sp))
				continue
			}

			// Pool agent (explicit [agents.pool] or implicit singleton with pool config).
			desired, err := evaluatePool(c.Agents[i].Name, pool, shellScaleCheck)
			if err != nil {
				fmt.Fprintf(stderr, "gc start: %v (using min=%d)\n", err, pool.Min) //nolint:errcheck // best-effort stderr
			}
			pa, err := poolAgents(&c.Agents[i], desired, cityName, cityPath,
				&c.Workspace, c.Providers, exec.LookPath, fsys.OSFS{}, sp)
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
	code := doReconcileAgents(agents, sp, rops, nil, recorder, cityPrefix, nil, stdout, stderr)
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
func resolveRigForAgent(workDir string, rigs []config.Rig) string {
	for _, r := range rigs {
		if workDir == r.Path {
			return r.Name
		}
	}
	return ""
}

// readPromptFile reads a prompt template file relative to cityPath.
// Returns empty string if templatePath is empty or the file doesn't exist
// (agent starts without a prompt — not an error).
func readPromptFile(fs fsys.FS, cityPath, templatePath string) string {
	if templatePath == "" {
		return ""
	}
	data, err := fs.ReadFile(filepath.Join(cityPath, templatePath))
	if err != nil {
		return ""
	}
	return string(data)
}
