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
		// No city found — auto-init at dir.
		if code := doInit(fsys.OSFS{}, dir, stdout, stderr); code != 0 {
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

	// Resolve provider command for each agent. Agents whose provider can't
	// be resolved are skipped with a warning (the city still starts).
	sp := newSessionProvider()
	var agents []agent.Agent
	for i := range cfg.Agents {
		command, err := resolveAgentCommand(&cfg.Agents[i], exec.LookPath)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", cfg.Agents[i].Name, err) //nolint:errcheck // best-effort stderr
			continue
		}
		sn := sessionName(cityName, cfg.Agents[i].Name)
		agents = append(agents, agent.New(cfg.Agents[i].Name, sn, command, sp))
	}

	return doStartAgents(agents, stdout, stderr)
}

// doStartAgents is the pure logic for starting agent sessions. It iterates
// agents and starts any that aren't already running. Accepts pre-built
// agents for testability.
func doStartAgents(agents []agent.Agent, stdout, stderr io.Writer) int {
	for _, a := range agents {
		if a.IsRunning() {
			continue
		}
		if err := a.Start(); err != nil {
			fmt.Fprintf(stderr, "gc start: starting %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
		} else {
			fmt.Fprintf(stdout, "Started agent '%s' (session: %s)\n", a.Name(), a.SessionName()) //nolint:errcheck // best-effort stdout
		}
	}
	fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	return 0
}
