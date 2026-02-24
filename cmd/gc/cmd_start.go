package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newStartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "start [path]",
		Short: "Start the city (auto-initializes if needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doStart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doStart boots the city. If a path is given, operates there; otherwise uses
// cwd. If no city exists at the target, it auto-initializes one first via
// doInit, then starts all configured agent sessions.
func doStart(args []string, stdout, stderr io.Writer) int {
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
			// Non-fatal: agents may still work if server started externally.
		}
	}

	// Resolve provider command for each agent. Agents whose provider can't
	// be resolved are skipped with a warning (the city still starts).
	sp := newSessionProvider()
	var agents []agent.Agent
	for i := range cfg.Agents {
		resolved, err := config.ResolveProvider(&cfg.Agents[i], &cfg.Workspace, cfg.Providers, exec.LookPath)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", cfg.Agents[i].Name, err) //nolint:errcheck // best-effort stderr
			continue
		}
		command := resolved.CommandString()
		sn := sessionName(cityName, cfg.Agents[i].Name)
		prompt := readPromptFile(fsys.OSFS{}, cityPath, cfg.Agents[i].PromptTemplate)
		env := mergeEnv(passthroughEnv(), resolved.Env, map[string]string{
			"GC_AGENT": cfg.Agents[i].Name,
			"GC_CITY":  cityPath,
		})
		hints := agent.StartupHints{
			ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
			ReadyDelayMs:           resolved.ReadyDelayMs,
			ProcessNames:           resolved.ProcessNames,
			EmitsPermissionWarning: resolved.EmitsPermissionWarning,
		}
		agents = append(agents, agent.New(cfg.Agents[i].Name, sn, command, prompt, env, hints, sp))
	}

	// Process pools: evaluate scale_check and generate pool agents.
	if err := config.ValidatePools(cfg.Pools); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	for i := range cfg.Pools {
		desired, err := evaluatePool(&cfg.Pools[i], shellScaleCheck)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v (using min=%d)\n", err, cfg.Pools[i].Min) //nolint:errcheck // best-effort stderr
		}
		pa, err := poolAgents(&cfg.Pools[i], desired, cityName, cityPath,
			&cfg.Workspace, cfg.Providers, exec.LookPath, fsys.OSFS{}, sp)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v (skipping pool)\n", err) //nolint:errcheck // best-effort stderr
			continue
		}
		if len(pa) > 0 {
			fmt.Fprintf(stdout, "Pool '%s': starting %d agent(s)\n", cfg.Pools[i].Name, len(pa)) //nolint:errcheck // best-effort stdout
			agents = append(agents, pa...)
		}
	}

	cityPrefix := "gc-" + cityName + "-"
	rops := newReconcileOps(sp)

	suspended := make(map[string]bool)
	for i := range cfg.Agents {
		if cfg.Agents[i].IsSuspended() {
			sn := sessionName(cityName, cfg.Agents[i].Name)
			suspended[sn] = true
		}
	}

	recorder := events.Discard
	if fr, err := events.NewFileRecorder(
		filepath.Join(cityPath, ".gc", "events.jsonl"), stderr); err == nil {
		recorder = fr
	}

	return doReconcileAgents(agents, suspended, sp, rops, recorder, cityPrefix, stdout, stderr)
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
