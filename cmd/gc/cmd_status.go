package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

// ---------------------------------------------------------------------------
// gc agent status <name>
// ---------------------------------------------------------------------------

// newAgentStatusCmd creates the "gc agent status <name>" subcommand.
func newAgentStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show agent status",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentStatus is the CLI entry point for showing agent status.
func cmdAgentStatus(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent status: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent status: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName = found.QualifiedName()

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	return doAgentStatus(sp, dops, found, agentName, sn, stdout, stderr)
}

// doAgentStatus prints detailed status for a single agent.
func doAgentStatus(
	sp session.Provider,
	dops drainOps,
	cfgAgent config.Agent,
	agentName, sn string,
	stdout, stderr io.Writer,
) int {
	_ = stderr // reserved for future error reporting
	running := sp.IsRunning(sn)
	draining, _ := dops.isDraining(sn)

	runStr := "no"
	if running {
		runStr = "yes"
	}
	suspStr := "no"
	if cfgAgent.Suspended {
		suspStr = "yes"
	}
	drainStr := "no"
	if draining {
		drainStr = "yes"
	}

	fmt.Fprintf(stdout, "%s:\n", agentName)             //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Session:    %s\n", sn)       //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Running:    %s\n", runStr)   //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Suspended:  %s\n", suspStr)  //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Draining:   %s\n", drainStr) //nolint:errcheck // best-effort stdout
	return 0
}

// ---------------------------------------------------------------------------
// gc rig status <name>
// ---------------------------------------------------------------------------

// newRigStatusCmd creates the "gc rig status <name>" subcommand.
func newRigStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show rig status and agent running state",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdRigStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdRigStatus is the CLI entry point for showing rig status.
func cmdRigStatus(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc rig status: missing rig name") //nolint:errcheck // best-effort stderr
		return 1
	}
	rigName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc rig status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc rig status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Find the rig.
	var rig config.Rig
	found := false
	for _, r := range cfg.Rigs {
		if r.Name == rigName {
			rig = r
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(stderr, "gc rig status: rig %q not found in city.toml\n", rigName) //nolint:errcheck // best-effort stderr
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
	dops := newDrainOps(sp)
	return doRigStatus(sp, dops, rig, rigAgents, cityName, cfg.Workspace.SessionTemplate, stdout, stderr)
}

// doRigStatus prints rig info and per-agent running state.
func doRigStatus(
	sp session.Provider,
	dops drainOps,
	rig config.Rig,
	agents []config.Agent,
	cityName, sessionTemplate string,
	stdout, stderr io.Writer,
) int {
	_ = stderr // reserved for future error reporting

	suspStr := "no"
	if rig.Suspended {
		suspStr = "yes"
	}

	fmt.Fprintf(stdout, "%s:\n", rig.Name)              //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Path:       %s\n", rig.Path) //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Suspended:  %s\n", suspStr)  //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "  Agents:\n")                  //nolint:errcheck // best-effort stdout

	for _, a := range agents {
		pool := a.EffectivePool()
		if pool.Max <= 1 {
			sn := sessionName(cityName, a.QualifiedName(), sessionTemplate)
			status := agentStatusLine(sp, dops, sn, a.Suspended)
			fmt.Fprintf(stdout, "    %-12s%s\n", a.Name, status) //nolint:errcheck // best-effort stdout
		} else {
			for i := 1; i <= pool.Max; i++ {
				instanceName := fmt.Sprintf("%s-%d", a.Name, i)
				qualifiedInstance := instanceName
				if a.Dir != "" {
					qualifiedInstance = a.Dir + "/" + instanceName
				}
				sn := sessionName(cityName, qualifiedInstance, sessionTemplate)
				status := agentStatusLine(sp, dops, sn, a.Suspended)
				fmt.Fprintf(stdout, "    %-12s%s\n", instanceName, status) //nolint:errcheck // best-effort stdout
			}
		}
	}
	return 0
}

// agentStatusLine returns a human-readable status string for an agent session.
func agentStatusLine(sp session.Provider, dops drainOps, sn string, suspended bool) string {
	running := sp.IsRunning(sn)
	draining, _ := dops.isDraining(sn)

	switch {
	case running && draining:
		return "running  (draining)"
	case running:
		return "running"
	case suspended:
		return "stopped  (suspended)"
	default:
		return "stopped"
	}
}
