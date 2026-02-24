package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

func newPoolCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage agent pools",
	}
	cmd.AddCommand(newPoolStatusCmd(stdout, stderr))
	return cmd
}

func newPoolStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show pool status and running agents",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(stderr, "gc pool status: %v\n", err) //nolint:errcheck // best-effort stderr
				return errExit
			}
			cityPath, err := findCity(cwd)
			if err != nil {
				fmt.Fprintf(stderr, "gc pool status: %v\n", err) //nolint:errcheck // best-effort stderr
				return errExit
			}
			cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
			if err != nil {
				fmt.Fprintf(stderr, "gc pool status: %v\n", err) //nolint:errcheck // best-effort stderr
				return errExit
			}
			cityName := cfg.Workspace.Name
			if cityName == "" {
				cityName = filepath.Base(cityPath)
			}
			sp := newSessionProvider()
			doPoolStatus(cfg.Pools, cityName, sp, stdout)
			return nil
		},
	}
}

// doPoolStatus is the testable pure logic for "gc pool status".
func doPoolStatus(pools []config.Pool, cityName string,
	sp session.Provider, stdout io.Writer,
) {
	if len(pools) == 0 {
		fmt.Fprintln(stdout, "No pools configured.") //nolint:errcheck // best-effort stdout
		return
	}
	for _, p := range pools {
		fmt.Fprintf(stdout, "Pool '%s': min=%d, max=%d, scale_check=%q\n", //nolint:errcheck // best-effort stdout
			p.Name, p.Min, p.Max, p.ScaleCheck)
		running := 0
		for i := 1; i <= p.Max; i++ {
			name := fmt.Sprintf("%s-%d", p.Name, i)
			sn := sessionName(cityName, name)
			if sp.IsRunning(sn) {
				fmt.Fprintf(stdout, "  %s: running (session: %s)\n", name, sn) //nolint:errcheck // best-effort stdout
				running++
			}
		}
		if running == 0 {
			fmt.Fprintln(stdout, "  (no agents running)") //nolint:errcheck // best-effort stdout
		} else {
			fmt.Fprintf(stdout, "  total: %d running\n", running) //nolint:errcheck // best-effort stdout
		}
	}
}
