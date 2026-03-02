package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/dolt"
)

func newBeadsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "beads",
		Short: "Manage the beads provider",
		Long: `Manage the beads provider (backing store for issue tracking).

Subcommands for health checking and diagnostics.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc beads: missing subcommand (health)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc beads: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newBeadsHealthCmd(stdout, stderr),
	)
	return cmd
}

func newBeadsHealthCmd(stdout, stderr io.Writer) *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check beads provider health",
		Long: `Check beads provider health and attempt recovery on failure.

For the bd (dolt) provider, runs a three-layer health check:
  1. TCP reachability on the configured port
  2. Query probe (SELECT 1)
  3. Write probe (create/write/drop temp table)

If unhealthy, attempts automatic recovery (stop + restart).
For exec providers, delegates to the provider's "health" operation.
For the file provider, always succeeds (no-op).

Also used by the beads-health system automation for periodic monitoring.`,
		Example: `  gc beads health
  gc beads health --quiet`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doBeadsHealth(quiet, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false,
		"silent on success, stderr on failure")
	return cmd
}

// doBeadsHealth runs the beads provider health check.
// Returns 0 if healthy, 1 if unhealthy/recovery-failed.
func doBeadsHealth(quiet bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc beads health: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	provider := beadsProvider(cityPath)

	// For non-bd providers, delegate to healthBeadsProvider directly.
	if provider != "bd" || os.Getenv("GC_DOLT") == "skip" {
		if err := healthBeadsProvider(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc beads health: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if !quiet {
			fmt.Fprintln(stdout, "Beads provider: healthy") //nolint:errcheck // best-effort stdout
		}
		return 0
	}

	// bd provider: run per-tier checks with verbose output.
	config := dolt.GasCityConfig(cityPath)
	allOK := true

	// Tier 1: TCP reachability.
	tcpOK := false
	conn, tcpErr := net.DialTimeout("tcp", config.HostPort(), 2*time.Second)
	if tcpErr == nil {
		_ = conn.Close()
		tcpOK = true
	}
	if !quiet {
		if tcpOK {
			fmt.Fprintf(stdout, "TCP (%s): ok\n", config.HostPort()) //nolint:errcheck // best-effort stdout
		} else {
			fmt.Fprintf(stdout, "TCP (%s): FAIL (%v)\n", config.HostPort(), tcpErr) //nolint:errcheck // best-effort stdout
		}
	}
	if !tcpOK {
		allOK = false
	}

	// Tier 2: Query probe.
	queryOK := false
	if tcpOK {
		queryErr := dolt.HealthCheckQuery(cityPath)
		queryOK = queryErr == nil
		if !quiet {
			if queryOK {
				fmt.Fprintln(stdout, "Query (SELECT 1): ok") //nolint:errcheck // best-effort stdout
			} else {
				fmt.Fprintf(stdout, "Query (SELECT 1): FAIL (%v)\n", queryErr) //nolint:errcheck // best-effort stdout
			}
		}
		if !queryOK {
			allOK = false
		}
	}

	// Tier 3: Write probe.
	if queryOK {
		writeErr := dolt.HealthCheckWrite(cityPath)
		writeOK := writeErr == nil
		if !quiet {
			if writeOK {
				fmt.Fprintln(stdout, "Write: ok") //nolint:errcheck // best-effort stdout
			} else {
				fmt.Fprintf(stdout, "Write: FAIL (%v)\n", writeErr) //nolint:errcheck // best-effort stdout
			}
		}
		if !writeOK {
			allOK = false
		}
	}

	// Report DOLT_UNHEALTHY signal status.
	unhealthy, reason := dolt.IsUnhealthy(cityPath)
	if !quiet && unhealthy {
		fmt.Fprintf(stdout, "DOLT_UNHEALTHY: %s\n", reason) //nolint:errcheck // best-effort stdout
	}

	if !allOK {
		// Attempt recovery.
		if !quiet {
			fmt.Fprintln(stdout, "Attempting recovery...") //nolint:errcheck // best-effort stdout
		}
		dolt.SetUnhealthy(cityPath, "health check failed")
		if recErr := dolt.RecoverDolt(cityPath); recErr != nil {
			fmt.Fprintf(stderr, "gc beads health: recovery failed: %v\n", recErr) //nolint:errcheck // best-effort stderr
			return 1
		}
		dolt.ClearUnhealthy(cityPath)
		if !quiet {
			fmt.Fprintln(stdout, "Recovery successful.") //nolint:errcheck // best-effort stdout
		}
	}

	return 0
}
