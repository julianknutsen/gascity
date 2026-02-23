package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newStopCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stop [path]",
		Short: "Stop all agent sessions in the city",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdStop(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdStop stops the city by terminating all configured agent sessions.
// If a path is given, operates there; otherwise uses cwd.
func cmdStop(args []string, stdout, stderr io.Writer) int {
	var dir string
	if len(args) > 0 {
		var err error
		dir, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	sp := newSessionProvider()
	var agents []agent.Agent
	for _, a := range cfg.Agents {
		sn := sessionName(cityName, a.Name)
		agents = append(agents, agent.New(a, sn, "", "", nil, sp))
	}
	code := doStop(agents, stdout, stderr)

	// Stop dolt server after agents.
	if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
		if err := dolt.StopCity(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc stop: dolt: %v\n", err) //nolint:errcheck // best-effort stderr
			// Non-fatal warning.
		}
	}

	return code
}

// doStop is the pure logic for "gc stop". It iterates agents and stops any
// running sessions. Accepts pre-built agents for testability.
func doStop(agents []agent.Agent, stdout, stderr io.Writer) int {
	for _, a := range agents {
		if a.IsRunning() {
			if err := a.Stop(); err != nil {
				fmt.Fprintf(stderr, "gc stop: stopping %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stdout, "Stopped agent '%s' (session: %s)\n", a.Name(), a.SessionName()) //nolint:errcheck // best-effort stdout
			}
		}
	}
	fmt.Fprintln(stdout, "City stopped.") //nolint:errcheck // best-effort stdout
	return 0
}
