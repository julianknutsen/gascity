package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

// bdPassthrough maps a gc bead subcommand to a bd CLI call, passing all
// args through. Runs bd in the city directory so it picks up .beads/ config.
// Returns exit code.
func bdPassthrough(bdCmd string, args []string, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead %s: %v\n", bdCmd, err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc bead %s: %v\n", bdCmd, err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cmdArgs := append([]string{bdCmd}, args...)
	cmd := exec.Command("bd", cmdArgs...)
	cmd.Dir = cityPath
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "gc bead %s: %v\n", bdCmd, err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// containsHelpFlag returns true if args contains --help or -h.
func containsHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// beadRunE returns a cobra RunE that delegates to bd when the provider is
// "bd", otherwise falls back to the Go implementation. When bdCmd is empty,
// no bd passthrough is attempted (gc-only commands like "hooked").
// Help flags (--help, -h) are intercepted and routed appropriately.
func beadRunE(bdCmd string, goImpl func(args []string, stdout, stderr io.Writer) int, stdout, stderr io.Writer) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// DisableFlagParsing means cobra doesn't intercept --help.
		// Handle it here: show bd help for bd provider, cobra help otherwise.
		if containsHelpFlag(args) {
			if bdCmd != "" {
				cwd, _ := os.Getwd()
				if cityPath, err := findCity(cwd); err == nil && beadsProvider(cityPath) == "bd" {
					bdPassthrough(bdCmd, []string{"--help"}, stdout, stderr)
					return nil
				}
			}
			return cmd.Help()
		}

		if bdCmd != "" {
			cwd, err := os.Getwd()
			if err == nil {
				if cityPath, err := findCity(cwd); err == nil {
					if beadsProvider(cityPath) == "bd" {
						if bdPassthrough(bdCmd, args, stdout, stderr) != 0 {
							return errExit
						}
						return nil
					}
				}
			}
		}
		// Fall back to Go implementation for file provider, gc-only commands,
		// or if city not found.
		if goImpl(args, stdout, stderr) != 0 {
			return errExit
		}
		return nil
	}
}

const beadPassthroughNote = `
When using the bd provider, all flags are passed through to bd.
Run 'bd %s --help' for full bd options (--format, --json, etc.).

With the file provider, supported flags:
  --format text|json|toon   Output format (default: text)
  --json                    Shorthand for --format json`

func newBeadCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bead",
		Short: "Manage beads (work units)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc bead: missing subcommand (close, create, hooked, list, ready, show)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc bead: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newBeadCloseCmd(stdout, stderr),
		newBeadCreateCmd(stdout, stderr),
		newBeadHookedCmd(stdout, stderr),
		newBeadListCmd(stdout, stderr),
		newBeadReadyCmd(stdout, stderr),
		newBeadShowCmd(stdout, stderr),
	)
	return cmd
}

func newBeadCloseCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "close <id>",
		Short:              "Close a bead by ID",
		Long:               "Close a bead by ID." + fmt.Sprintf(beadPassthroughNote, "close"),
		DisableFlagParsing: true,
		RunE:               beadRunE("close", cmdBeadClose, stdout, stderr),
	}
	return cmd
}

func newBeadCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create <title>",
		Short:              "Create a new bead",
		Long:               "Create a new bead with the given title." + fmt.Sprintf(beadPassthroughNote, "create"),
		DisableFlagParsing: true,
		RunE:               beadRunE("create", cmdBeadCreate, stdout, stderr),
	}
	return cmd
}

func newBeadHookedCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooked <agent-name>",
		Short: "Show the bead on an agent's hook",
		Long: `Show the bead currently hooked to the given agent.
This is a gc-specific command (no bd equivalent).

Supported flags:
  --format text|json|toon   Output format (default: text)
  --json                    Shorthand for --format json`,
		DisableFlagParsing: true,
		RunE:               beadRunE("", cmdBeadHooked, stdout, stderr), // no bd equivalent
	}
	return cmd
}

func newBeadListCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "list",
		Short:              "List all beads",
		Long:               "List all beads in a table." + fmt.Sprintf(beadPassthroughNote, "list"),
		DisableFlagParsing: true,
		RunE:               beadRunE("list", cmdBeadList, stdout, stderr),
	}
	return cmd
}

func newBeadReadyCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "ready",
		Short:              "List open (ready) beads",
		Long:               "List open (ready) beads." + fmt.Sprintf(beadPassthroughNote, "ready"),
		DisableFlagParsing: true,
		RunE:               beadRunE("ready", cmdBeadReady, stdout, stderr),
	}
	return cmd
}

func newBeadShowCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "show <id>",
		Short:              "Show details of a bead",
		Long:               "Show details of a bead by ID." + fmt.Sprintf(beadPassthroughNote, "show"),
		DisableFlagParsing: true,
		RunE:               beadRunE("show", cmdBeadShow, stdout, stderr),
	}
	return cmd
}

// cmdBeadClose is the CLI entry point for closing a bead. It opens a
// FileStore in the current city and delegates to doBeadClose.
func cmdBeadClose(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead close")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doBeadClose(store, rec, args, stdout, stderr)
}

// doBeadClose closes a bead by ID. Accepts an injected store and recorder
// for testability.
func doBeadClose(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	_, args = parseBeadFormat(args) // strip --format/--json; not used for mutations
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead close: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := store.Close(args[0]); err != nil {
		fmt.Fprintf(stderr, "gc bead close: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.BeadClosed,
		Actor:   eventActor(),
		Subject: args[0],
	})
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
	rec := openCityRecorder(stderr)
	return doBeadCreate(store, rec, args, stdout, stderr)
}

// doBeadCreate creates a bead with the given title. Accepts an injected
// store and recorder for testability.
func doBeadCreate(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	_, args = parseBeadFormat(args) // strip --format/--json; not used for mutations
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead create: missing title") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Create(beads.Bead{Title: args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "gc bead create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.BeadCreated,
		Actor:   eventActor(),
		Subject: b.ID,
		Message: b.Title,
	})
	fmt.Fprintf(stdout, "Created bead: %s  (status: %s)\n", b.ID, b.Status) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadHooked is the CLI entry point for showing the bead on an agent's hook.
// It opens the bead store in the current city and delegates to doBeadHooked.
func cmdBeadHooked(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead hooked")
	if store == nil {
		return code
	}
	return doBeadHooked(store, args, stdout, stderr)
}

// doBeadHooked shows the bead currently hooked to the given agent. Output
// format matches gc bead show. Accepts an injected store for testability.
func doBeadHooked(store beads.Store, args []string, stdout, stderr io.Writer) int {
	format, args := parseBeadFormat(args)
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead hooked: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Hooked(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc bead hooked: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	switch format {
	case "json":
		writeBeadJSON(b, stdout)
	case "toon":
		writeBeadTOON(b, stdout)
	default:
		w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
		w(fmt.Sprintf("ID:       %s", b.ID))
		w(fmt.Sprintf("Status:   %s", b.Status))
		w(fmt.Sprintf("Type:     %s", b.Type))
		w(fmt.Sprintf("Title:    %s", b.Title))
		w(fmt.Sprintf("Created:  %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
		w(fmt.Sprintf("Assignee: %s", b.Assignee))
	}
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
	format, _ := parseBeadFormat(args)
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	switch format {
	case "json":
		writeBeadsJSON(all, stdout)
	case "toon":
		writeBeadListTOON(all, "id,status,assignee,title", func(b beads.Bead) string {
			assignee := b.Assignee
			if assignee == "" {
				assignee = "\u2014"
			}
			return toonVal(b.ID) + "," + b.Status + "," + toonVal(assignee) + "," + toonVal(b.Title)
		}, stdout)
	default:
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
	}
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
	format, _ := parseBeadFormat(args)
	ready, err := store.Ready()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead ready: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	switch format {
	case "json":
		writeBeadsJSON(ready, stdout)
	case "toon":
		writeBeadListTOON(ready, "id,status,title", func(b beads.Bead) string {
			return toonVal(b.ID) + "," + b.Status + "," + toonVal(b.Title)
		}, stdout)
	default:
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSTATUS\tTITLE") //nolint:errcheck // best-effort stdout
		for _, b := range ready {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", b.ID, b.Status, b.Title) //nolint:errcheck // best-effort stdout
		}
		tw.Flush() //nolint:errcheck // best-effort stdout
	}
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
	format, args := parseBeadFormat(args)
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead show: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc bead show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	switch format {
	case "json":
		writeBeadJSON(b, stdout)
	case "toon":
		writeBeadTOON(b, stdout)
	default:
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
	}
	return 0
}
