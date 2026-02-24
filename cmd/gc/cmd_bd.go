package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

// newBdCmd creates the "gc bd" command — a transparent proxy to the bd CLI
// with automatic event recording. All arguments are forwarded to bd.
func newBdCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bd [command] [args...]",
		Short: "Run bd (beads) commands with event recording",
		Long: `Transparent proxy to the bd CLI with automatic event recording.

All arguments are forwarded to bd. Mutation commands (create, close,
update, label) emit events to the city event log.

When using the file provider (GC_BEADS=file), a subset of bd commands
is implemented natively: create, close, list, show, ready.`,
		DisableFlagParsing: true,
	}
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
			return cmd.Help()
		}
		if doBd(args, stdout, stderr) != 0 {
			return errExit
		}
		return nil
	}
	return cmd
}

// doBd dispatches bd commands. For the bd provider, all args are passed
// through to the bd binary with event emission. For the file provider,
// known subcommands are handled by Go implementations.
func doBd(args []string, stdout, stderr io.Writer) int {
	subcmd := args[0]
	rest := args[1:]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc bd: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc bd: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	rec := openCityRecorder(stderr)
	provider := beadsProvider(cityPath)

	if provider == "bd" {
		code := bdPassthrough(cityPath, subcmd, rest, stdout, stderr)
		if code == 0 {
			emitBdEvent(rec, subcmd, rest)
		}
		return code
	}

	// File provider: dispatch to Go implementations.
	store, code := openCityStore(stderr, "gc bd")
	if store == nil {
		return code
	}

	switch subcmd {
	case "create":
		return doBeadCreate(store, rec, rest, stdout, stderr)
	case "close":
		return doBeadClose(store, rec, rest, stdout, stderr)
	case "list":
		return doBeadList(store, rest, stdout, stderr)
	case "show":
		return doBeadShow(store, rest, stdout, stderr)
	case "ready":
		return doBeadReady(store, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc bd %s: not supported with file provider (install bd or set GC_BEADS=bd)\n", subcmd) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// bdPassthrough runs a bd subcommand in the given city directory.
// Returns the exit code.
func bdPassthrough(cityPath, subcmd string, args []string, stdout, stderr io.Writer) int {
	cmdArgs := append([]string{subcmd}, args...)
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
		fmt.Fprintf(stderr, "gc bd %s: %v\n", subcmd, err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// emitBdEvent records an event for a bd mutation. Called after successful
// bd passthrough execution. Maps known subcommands to standard event types;
// other mutations get a generic bd.<subcmd> event.
func emitBdEvent(rec events.Recorder, subcmd string, args []string) {
	switch subcmd {
	case "create":
		rec.Record(events.Event{
			Type:    events.BeadCreated,
			Actor:   eventActor(),
			Message: strings.Join(args, " "),
		})
	case "close":
		rec.Record(events.Event{
			Type:    events.BeadClosed,
			Actor:   eventActor(),
			Subject: firstPositionalArg(args),
		})
	case "update", "label", "sync":
		rec.Record(events.Event{
			Type:    "bd." + subcmd,
			Actor:   eventActor(),
			Subject: firstPositionalArg(args),
			Message: strings.Join(args, " "),
		})
	}
}

// firstPositionalArg returns the first arg that doesn't look like a flag.
func firstPositionalArg(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			// Skip --flag=value (single token).
			if strings.Contains(a, "=") {
				continue
			}
			// Skip --flag value (two tokens) if next arg exists.
			if i+1 < len(args) {
				i++
			}
			continue
		}
		return a
	}
	return ""
}

// --- File provider implementations (used when GC_BEADS=file) ---

// doBeadClose closes a bead by ID. Accepts an injected store and recorder
// for testability.
func doBeadClose(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	_, args = parseBeadFormat(args) // strip --format/--json; not used for mutations
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bd close: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := store.Close(args[0]); err != nil {
		fmt.Fprintf(stderr, "gc bd close: %v\n", err) //nolint:errcheck // best-effort stderr
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

// doBeadCreate creates a bead with the given title. Accepts an injected
// store and recorder for testability.
func doBeadCreate(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	_, args = parseBeadFormat(args) // strip --format/--json; not used for mutations
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bd create: missing title") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Create(beads.Bead{Title: args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "gc bd create: %v\n", err) //nolint:errcheck // best-effort stderr
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

// doBeadList lists all beads in a tab-aligned table. Accepts an injected
// store for testability.
func doBeadList(store beads.Store, args []string, stdout, stderr io.Writer) int {
	format, _ := parseBeadFormat(args)
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc bd list: %v\n", err) //nolint:errcheck // best-effort stderr
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

// doBeadReady lists all open beads in a tab-aligned table. Accepts an
// injected store for testability.
func doBeadReady(store beads.Store, args []string, stdout, stderr io.Writer) int {
	format, _ := parseBeadFormat(args)
	ready, err := store.Ready()
	if err != nil {
		fmt.Fprintf(stderr, "gc bd ready: %v\n", err) //nolint:errcheck // best-effort stderr
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

// doBeadShow displays a bead's details. Accepts an injected store for
// testability.
func doBeadShow(store beads.Store, args []string, stdout, stderr io.Writer) int {
	format, args := parseBeadFormat(args)
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bd show: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc bd show: %v\n", err) //nolint:errcheck // best-effort stderr
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
