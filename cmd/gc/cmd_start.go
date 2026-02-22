package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"

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

// startAgent holds the resolved name and command for an agent to start.
type startAgent struct {
	Name    string
	Command string
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
	cfg, err := config.Load(filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve provider command for each agent. Agents whose provider can't
	// be resolved are skipped with a warning (the city still starts).
	var agents []startAgent
	for i := range cfg.Agents {
		command, err := resolveAgentCommand(&cfg.Agents[i], exec.LookPath)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: agent %q: %v (skipping)\n", cfg.Agents[i].Name, err) //nolint:errcheck // best-effort stderr
			continue
		}
		agents = append(agents, startAgent{Name: cfg.Agents[i].Name, Command: command})
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	sp := sessiontmux.NewProvider()
	return doStartAgents(sp, agents, cityName, stdout, stderr)
}

// doStartAgents is the pure logic for starting agent sessions. It iterates
// agents, constructs session names, and starts any that aren't already
// running. Accepts an injected session provider for testability.
func doStartAgents(sp session.Provider, agents []startAgent, cityName string, stdout, stderr io.Writer) int {
	for _, agent := range agents {
		sn := sessionName(cityName, agent.Name)
		if sp.IsRunning(sn) {
			continue
		}
		if err := sp.Start(sn, session.Config{Command: agent.Command}); err != nil {
			fmt.Fprintf(stderr, "gc start: starting %s: %v\n", agent.Name, err) //nolint:errcheck // best-effort stderr
		} else {
			fmt.Fprintf(stdout, "Started agent '%s' (session: %s)\n", agent.Name, sn) //nolint:errcheck // best-effort stdout
		}
	}
	fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	return 0
}
