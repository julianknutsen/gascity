package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

// newRestartCmd creates the top-level "gc restart" command.
func newRestartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "restart [path]",
		Short: "Restart all agent sessions in the city",
		Long: `Restart the city by stopping all agents then starting them again.

Equivalent to running "gc stop" followed by "gc start". Performs a
full one-shot reconciliation after stopping, which re-reads city.toml
and starts all configured agents.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdRestart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdRestart stops all agents then starts them again via one-shot reconcile.
func cmdRestart(args []string, stdout, stderr io.Writer) int {
	if code := cmdStop(args, stdout, stderr); code != 0 {
		return code
	}
	return doStart(args, false /*controllerMode*/, stdout, stderr)
}

// newRigRestartCmd creates the "gc rig restart <name>" subcommand.
func newRigRestartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart all agents in a rig",
		Long: `Kill all agent sessions belonging to a rig.

The reconciler will restart the agents on its next tick. This is a
quick way to force-refresh all agents working on a particular project.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdRigRestart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdRigRestart kills all agent sessions in a rig. The reconciler restarts
// them on its next tick.
func cmdRigRestart(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc rig restart: missing rig name") //nolint:errcheck // best-effort stderr
		return 1
	}
	rigName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc rig restart: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc rig restart: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Verify rig exists.
	found := false
	for _, r := range cfg.Rigs {
		if r.Name == rigName {
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(stderr, "gc rig restart: rig %q not found in city.toml\n", rigName) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Collect agents belonging to this rig.
	var rigAgents []config.Agent
	for _, a := range cfg.Agents {
		if a.Dir == rigName {
			rigAgents = append(rigAgents, a)
		}
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	rec := openCityRecorder(stderr)
	return doRigRestart(sp, rec, rigAgents, rigName, cityName, cfg.Workspace.SessionTemplate, stdout, stderr)
}

// doRigRestart kills sessions for all agents in a rig. The reconciler will
// restart them. Returns 0 even if no agents were running.
func doRigRestart(
	sp session.Provider,
	rec events.Recorder,
	agents []config.Agent,
	rigName, cityName, sessionTemplate string,
	stdout, stderr io.Writer,
) int {
	killed := 0
	for _, a := range agents {
		pool := a.EffectivePool()
		if pool.Max <= 1 {
			// Single agent.
			sn := sessionName(cityName, a.QualifiedName(), sessionTemplate)
			if sp.IsRunning(sn) {
				if err := sp.Stop(sn); err != nil {
					fmt.Fprintf(stderr, "gc rig restart: stopping %s: %v\n", sn, err) //nolint:errcheck // best-effort stderr
					continue
				}
				rec.Record(events.Event{
					Type:    events.AgentStopped,
					Actor:   eventActor(),
					Subject: a.QualifiedName(),
				})
				killed++
			}
		} else {
			// Pool agent: iterate {name}-1 through {name}-{max}.
			for i := 1; i <= pool.Max; i++ {
				instanceName := fmt.Sprintf("%s-%d", a.Name, i)
				qualifiedInstance := instanceName
				if a.Dir != "" {
					qualifiedInstance = a.Dir + "/" + instanceName
				}
				sn := sessionName(cityName, qualifiedInstance, sessionTemplate)
				if sp.IsRunning(sn) {
					if err := sp.Stop(sn); err != nil {
						fmt.Fprintf(stderr, "gc rig restart: stopping %s: %v\n", sn, err) //nolint:errcheck // best-effort stderr
						continue
					}
					rec.Record(events.Event{
						Type:    events.AgentStopped,
						Actor:   eventActor(),
						Subject: qualifiedInstance,
					})
					killed++
				}
			}
		}
	}

	fmt.Fprintf(stdout, "Restarted %d agent(s) in rig '%s' (killed sessions; reconciler will restart)\n", killed, rigName) //nolint:errcheck // best-effort stdout
	return 0
}
