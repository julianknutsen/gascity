package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
)

func newBeadCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bead",
		Short: "Manage beads (work units)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc bead: missing subcommand (close, create, list, ready, show)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc bead: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newBeadCloseCmd(stdout, stderr),
		newBeadCreateCmd(stdout, stderr),
		newBeadListCmd(stdout, stderr),
		newBeadReadyCmd(stdout, stderr),
		newBeadShowCmd(stdout, stderr),
	)
	return cmd
}

func newBeadCloseCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "Close a bead by ID",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdBeadClose(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newBeadCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new bead",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdBeadCreate(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newBeadListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all beads",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdBeadList(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newBeadReadyCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "List open (ready) beads",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdBeadReady(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newBeadShowCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show details of a bead",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdBeadShow(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdBeadClose is the CLI entry point for closing a bead. It opens a
// FileStore in the current city and delegates to doBeadClose.
func cmdBeadClose(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead close")
	if store == nil {
		return code
	}
	return doBeadClose(store, args, stdout, stderr)
}

// doBeadClose closes a bead by ID. Accepts an injected store for testability.
func doBeadClose(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead close: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := store.Close(args[0]); err != nil {
		fmt.Fprintf(stderr, "gc bead close: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Closed bead: %s\n", args[0]) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadCreate is the CLI entry point for bead creation. It opens a
// FileStore in the current city and delegates to doBeadCreate.
func cmdBeadCreate(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead create")
	if store == nil {
		return code
	}
	return doBeadCreate(store, args, stdout, stderr)
}

// doBeadCreate creates a bead with the given title. Accepts an injected
// store for testability.
func doBeadCreate(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead create: missing title") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Create(beads.Bead{Title: args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "gc bead create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Created bead: %s  (status: %s)\n", b.ID, b.Status) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadList is the CLI entry point for listing all beads. It opens a
// FileStore in the current city and delegates to doBeadList.
func cmdBeadList(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead list")
	if store == nil {
		return code
	}
	return doBeadList(store, args, stdout, stderr)
}

// doBeadList lists all beads in a tab-aligned table. Accepts an injected
// store for testability.
func doBeadList(store beads.Store, args []string, stdout, stderr io.Writer) int {
	_ = args // no arguments used yet
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tASSIGNEE\tTITLE") //nolint:errcheck // best-effort stdout
	for _, b := range all {
		assignee := b.Assignee
		if assignee == "" {
			assignee = "\u2014"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", b.ID, b.Status, assignee, b.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadReady is the CLI entry point for listing ready beads. It opens a
// FileStore in the current city and delegates to doBeadReady.
func cmdBeadReady(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead ready")
	if store == nil {
		return code
	}
	return doBeadReady(store, args, stdout, stderr)
}

// doBeadReady lists all open beads in a tab-aligned table. Accepts an
// injected store for testability.
func doBeadReady(store beads.Store, args []string, stdout, stderr io.Writer) int {
	_ = args // no arguments used yet
	ready, err := store.Ready()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead ready: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tTITLE") //nolint:errcheck // best-effort stdout
	for _, b := range ready {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", b.ID, b.Status, b.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadShow is the CLI entry point for showing a bead. It opens a
// FileStore in the current city and delegates to doBeadShow.
func cmdBeadShow(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead show")
	if store == nil {
		return code
	}
	return doBeadShow(store, args, stdout, stderr)
}

// doBeadShow displays a bead's details. Accepts an injected store for
// testability.
func doBeadShow(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead show: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc bead show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("ID:       %s", b.ID))
	w(fmt.Sprintf("Status:   %s", b.Status))
	w(fmt.Sprintf("Type:     %s", b.Type))
	w(fmt.Sprintf("Title:    %s", b.Title))
	w(fmt.Sprintf("Created:  %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
	assignee := b.Assignee
	if assignee == "" {
		assignee = "\u2014"
	}
	w(fmt.Sprintf("Assignee: %s", assignee))
	return 0
}
