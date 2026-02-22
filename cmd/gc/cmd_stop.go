package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

func newStopCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop all agent sessions in the current city",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdStop(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdStop stops the city by terminating all configured agent sessions.
func cmdStop(args []string, stdout, stderr io.Writer) int {
	_ = args
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	sp := sessiontmux.NewProvider()
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	return doStop(sp, cfg, cityName, stdout, stderr)
}

// doStop is the pure logic for "gc stop". It iterates configured agents,
// constructs session names, and stops any running sessions. Accepts an
// injected session provider for testability.
func doStop(sp session.Provider, cfg *config.City, cityName string, stdout, stderr io.Writer) int {
	for _, agent := range cfg.Agents {
		sn := sessionName(cityName, agent.Name)
		if sp.IsRunning(sn) {
			if err := sp.Stop(sn); err != nil {
				fmt.Fprintf(stderr, "gc stop: stopping %s: %v\n", agent.Name, err) //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stdout, "Stopped agent '%s' (session: %s)\n", agent.Name, sn) //nolint:errcheck // best-effort stdout
			}
		}
	}
	fmt.Fprintln(stdout, "City stopped.") //nolint:errcheck // best-effort stdout
	return 0
}
