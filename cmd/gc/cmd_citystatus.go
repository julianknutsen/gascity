package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

// newStatusCmd creates the "gc status [path]" command.
func newStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status [path]",
		Short: "Show city-wide status overview",
		Long: `Shows a city-wide overview: controller state, suspension,
all agents with running status, rigs, and a summary count.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdCityStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdCityStatus is the CLI entry point for the city status overview.
func cmdCityStatus(args []string, stdout, stderr io.Writer) int {
	var cityPath string
	var err error
	if len(args) > 0 {
		cityPath, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc status: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		cityPath, err = findCity(cityPath)
	} else {
		cityPath, err = resolveCity()
	}
	if err != nil {
		fmt.Fprintf(stderr, "gc status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	sp := newSessionProvider()
	dops := newDrainOps(sp)
	return doCityStatus(sp, dops, cfg, cityPath, stdout, stderr)
}

// doCityStatus prints the city-wide status overview. Accepts injected
// providers for testability.
func doCityStatus(
	sp session.Provider,
	dops drainOps,
	cfg *config.City,
	cityPath string,
	stdout, stderr io.Writer,
) int {
	_ = stderr // reserved for future error reporting

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	// Header: city name and path.
	fmt.Fprintf(stdout, "%s  %s\n", cityName, cityPath) //nolint:errcheck // best-effort stdout

	// Controller status.
	pid := readDaemonPID(cityPath)
	if pid != 0 && isDaemonAlive(pid) {
		fmt.Fprintf(stdout, "  Controller: running (PID %d)\n", pid) //nolint:errcheck // best-effort stdout
	} else {
		fmt.Fprintf(stdout, "  Controller: stopped\n") //nolint:errcheck // best-effort stdout
	}

	// Suspended status.
	if citySuspended(cfg) {
		fmt.Fprintf(stdout, "  Suspended:  yes\n") //nolint:errcheck // best-effort stdout
	} else {
		fmt.Fprintf(stdout, "  Suspended:  no\n") //nolint:errcheck // best-effort stdout
	}

	// Build set of suspended rig names.
	suspendedRigs := make(map[string]bool)
	for _, r := range cfg.Rigs {
		if r.Suspended {
			suspendedRigs[r.Name] = true
		}
	}

	// Agents section.
	if len(cfg.Agents) > 0 {
		fmt.Fprintln(stdout) //nolint:errcheck // best-effort stdout
		fmt.Fprintln(stdout, "Agents:")

		var totalAgents, runningAgents int

		for _, a := range cfg.Agents {
			// Effective suspended: agent-level or inherited from rig.
			suspended := a.Suspended || (a.Dir != "" && suspendedRigs[a.Dir])
			pool := a.EffectivePool()

			if pool.Max > 1 {
				// Pool agent — show pool header then instances.
				fmt.Fprintf(stdout, "  %-24spool (min=%d, max=%d)\n", a.QualifiedName(), pool.Min, pool.Max) //nolint:errcheck // best-effort stdout
				for i := 1; i <= pool.Max; i++ {
					instanceName := fmt.Sprintf("%s-%d", a.Name, i)
					qualifiedInstance := instanceName
					if a.Dir != "" {
						qualifiedInstance = a.Dir + "/" + instanceName
					}
					sn := sessionName(cityName, qualifiedInstance, cfg.Workspace.SessionTemplate)
					status := agentStatusLine(sp, dops, sn, suspended)
					fmt.Fprintf(stdout, "    %-22s%s\n", qualifiedInstance, status) //nolint:errcheck // best-effort stdout
					totalAgents++
					if sp.IsRunning(sn) {
						runningAgents++
					}
				}
			} else {
				// Singleton agent.
				sn := sessionName(cityName, a.QualifiedName(), cfg.Workspace.SessionTemplate)
				status := agentStatusLine(sp, dops, sn, suspended)
				fmt.Fprintf(stdout, "  %-24s%s\n", a.QualifiedName(), status) //nolint:errcheck // best-effort stdout
				totalAgents++
				if sp.IsRunning(sn) {
					runningAgents++
				}
			}
		}

		// Summary line.
		fmt.Fprintln(stdout)                                                      //nolint:errcheck // best-effort stdout
		fmt.Fprintf(stdout, "%d/%d agents running\n", runningAgents, totalAgents) //nolint:errcheck // best-effort stdout
	}

	// Rigs section.
	if len(cfg.Rigs) > 0 {
		fmt.Fprintln(stdout) //nolint:errcheck // best-effort stdout
		fmt.Fprintln(stdout, "Rigs:")
		for _, r := range cfg.Rigs {
			annotation := ""
			if r.Suspended {
				annotation = "  (suspended)"
			}
			fmt.Fprintf(stdout, "  %-24s%s%s\n", r.Name, r.Path, annotation) //nolint:errcheck // best-effort stdout
		}
	}

	return 0
}
