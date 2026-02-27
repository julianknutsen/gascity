package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/dolt"
)

func newDoltCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dolt",
		Short: "Manage the Dolt SQL server",
		Long: `Manage the Dolt SQL server used for bead storage.

Dolt provides the persistent database backing for the beads system.
These commands help inspect, recover, and sync the database.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc dolt: missing subcommand (logs, sql, list, recover, sync)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc dolt: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newDoltLogsCmd(stdout, stderr),
		newDoltSQLCmd(stdout, stderr),
		newDoltListCmd(stdout, stderr),
		newDoltRecoverCmd(stdout, stderr),
		newDoltSyncCmd(stdout, stderr),
	)
	return cmd
}

// --- gc dolt logs ---

func newDoltLogsCmd(stdout, stderr io.Writer) *cobra.Command {
	var lines int
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail the Dolt server log file",
		Long: `Tail the Dolt server log file.

Shows recent log output with optional follow mode.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoltLogs(lines, follow, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "number of lines to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the log in real time")
	return cmd
}

// doDoltLogs tails the dolt server log file.
func doDoltLogs(lines int, follow bool, _ io.Writer, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	config := dolt.GasCityConfig(cityPath)
	logFile := config.LogFile

	if _, err := os.Stat(logFile); err != nil {
		fmt.Fprintf(stderr, "gc dolt logs: log file not found: %s\n", logFile) //nolint:errcheck // best-effort stderr
		return 1
	}

	args := []string{fmt.Sprintf("-n%d", lines)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, logFile)

	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "gc dolt logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// --- gc dolt sql ---

func newDoltSQLCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "sql",
		Short: "Open an interactive Dolt SQL shell",
		Long: `Open an interactive Dolt SQL shell.

Connects to the running Dolt server if available, otherwise opens
in embedded mode using the first database directory found.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoltSQL(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDoltSQL opens an interactive dolt sql shell.
func doDoltSQL(_ io.Writer, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt sql: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	config := dolt.GasCityConfig(cityPath)

	// If server is running, connect to it; otherwise use embedded mode.
	running, _, err := dolt.IsRunningCity(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt sql: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	var args []string
	if running {
		// Connect to the running server.
		sqlArgs := config.SQLArgs()
		args = append(args, sqlArgs...)
		args = append(args, "sql")
	} else {
		// Embedded mode — find first database directory.
		databases, err := dolt.ListDatabases(cityPath)
		if err != nil || len(databases) == 0 {
			fmt.Fprintln(stderr, "gc dolt sql: no dolt server running and no databases found") //nolint:errcheck // best-effort stderr
			return 1
		}
		dbDir := dolt.RigDatabaseDir(cityPath, databases[0])
		args = append(args, "--data-dir", filepath.Dir(dbDir), "sql")
	}

	cmd := exec.Command("dolt", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "gc dolt sql: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// --- gc dolt list ---

func newDoltListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Dolt databases",
		Long: `List all Dolt databases with their filesystem paths.

Shows databases for the HQ (city) and all configured rigs.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoltList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDoltList lists all dolt databases with their paths.
func doDoltList(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	databases, err := dolt.ListDatabases(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if len(databases) == 0 {
		fmt.Fprintln(stdout, "No databases found.") //nolint:errcheck // best-effort stdout
		return 0
	}

	for _, db := range databases {
		dir := dolt.RigDatabaseDir(cityPath, db)
		fmt.Fprintf(stdout, "%s\t%s\n", db, dir) //nolint:errcheck // best-effort stdout
	}
	return 0
}

// --- gc dolt recover ---

func newDoltRecoverCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "recover",
		Short: "Recover Dolt from read-only state",
		Long: `Check for and recover from Dolt read-only state.

Dolt can enter read-only mode after certain failures. This command
detects the condition and attempts automatic recovery by restarting
the server.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoltRecover(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDoltRecover checks for read-only state and attempts recovery.
func doDoltRecover(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt recover: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	config := dolt.GasCityConfig(cityPath)
	if config.IsRemote() {
		fmt.Fprintln(stderr, "gc dolt recover: not supported for remote dolt servers") //nolint:errcheck // best-effort stderr
		return 1
	}

	readOnly, err := dolt.CheckReadOnly(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt recover: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if !readOnly {
		fmt.Fprintln(stdout, "Dolt server is not in read-only state.") //nolint:errcheck // best-effort stdout
		return 0
	}

	fmt.Fprintln(stdout, "Dolt server is in read-only state. Attempting recovery...") //nolint:errcheck // best-effort stdout
	if err := dolt.RecoverReadOnly(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc dolt recover: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintln(stdout, "Recovery successful.") //nolint:errcheck // best-effort stdout
	return 0
}

// --- gc dolt sync ---

func newDoltSyncCmd(stdout, stderr io.Writer) *cobra.Command {
	var dryRun, force, gc bool
	var dbFilter string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Push databases to configured remotes",
		Long: `Push Dolt databases to their configured remotes.

Stops the server for a clean push, syncs each database, then restarts.
Use --gc to purge closed ephemeral beads before syncing to reduce
transfer size. Use --dry-run to preview without pushing.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoltSync(dryRun, force, gc, dbFilter, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be pushed without pushing")
	cmd.Flags().BoolVar(&force, "force", false, "force-push to remotes")
	cmd.Flags().StringVar(&dbFilter, "db", "", "sync only the named database")
	cmd.Flags().BoolVar(&gc, "gc", false, "purge closed ephemeral beads before sync")
	return cmd
}

// doDoltSync pushes databases to their configured remotes.
func doDoltSync(dryRun, force, gc bool, dbFilter string, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc dolt sync: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Optional GC phase: purge closed ephemerals before sync.
	// Must run while server is still up (bd purge needs SQL access).
	if gc {
		databases, err := dolt.ListDatabases(cityPath)
		if err != nil {
			fmt.Fprintf(stderr, "gc dolt sync: listing databases for gc: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		for _, db := range databases {
			if dbFilter != "" && db != dbFilter {
				continue
			}
			purged, err := dolt.PurgeClosedEphemerals(cityPath, db, dryRun)
			if err != nil {
				fmt.Fprintf(stderr, "gc dolt sync: purge %s: %v\n", db, err) //nolint:errcheck // best-effort stderr
				// Non-fatal — continue with sync.
			} else if purged > 0 {
				fmt.Fprintf(stdout, "Purged %d ephemeral bead(s) from %s\n", purged, db) //nolint:errcheck // best-effort stdout
			}
		}
	}

	// Stop server for clean push (avoid write conflicts during push).
	wasRunning, _, _ := dolt.IsRunningCity(cityPath)
	if wasRunning && !dryRun {
		if err := dolt.StopCity(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc dolt sync: stopping server: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	// Sync databases.
	opts := dolt.SyncOptions{
		DryRun: dryRun,
		Force:  force,
		Filter: dbFilter,
	}
	results := dolt.SyncDatabases(cityPath, opts)

	// Print results.
	exitCode := 0
	for _, r := range results {
		switch {
		case r.Error != nil:
			fmt.Fprintf(stderr, "  %s: ERROR: %v\n", r.Database, r.Error) //nolint:errcheck // best-effort stderr
			exitCode = 1
		case r.Skipped:
			fmt.Fprintf(stdout, "  %s: skipped (no remote)\n", r.Database) //nolint:errcheck // best-effort stdout
		case r.DryRun:
			fmt.Fprintf(stdout, "  %s: would push to %s\n", r.Database, r.Remote) //nolint:errcheck // best-effort stdout
		case r.Pushed:
			fmt.Fprintf(stdout, "  %s: pushed to %s\n", r.Database, r.Remote) //nolint:errcheck // best-effort stdout
		}
	}

	// Restart server if it was running before.
	if wasRunning && !dryRun {
		if err := dolt.EnsureRunning(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc dolt sync: restarting server: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	return exitCode
}

// doltSummary returns a human-readable one-liner of sync results.
func doltSummary(results []dolt.SyncResult) string {
	var pushed, skipped, errors int
	for _, r := range results {
		switch {
		case r.Error != nil:
			errors++
		case r.Skipped:
			skipped++
		case r.Pushed || r.DryRun:
			pushed++
		}
	}
	parts := []string{}
	if pushed > 0 {
		parts = append(parts, fmt.Sprintf("%d pushed", pushed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errors))
	}
	if len(parts) == 0 {
		return "no databases"
	}
	return strings.Join(parts, ", ")
}
