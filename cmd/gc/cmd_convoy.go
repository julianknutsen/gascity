package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

func newConvoyCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convoy",
		Short: "Manage convoys (batch work tracking)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc convoy: missing subcommand (create, list, status, add, close, check, stranded)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc convoy: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newConvoyCreateCmd(stdout, stderr),
		newConvoyListCmd(stdout, stderr),
		newConvoyStatusCmd(stdout, stderr),
		newConvoyAddCmd(stdout, stderr),
		newConvoyCloseCmd(stdout, stderr),
		newConvoyCheckCmd(stdout, stderr),
		newConvoyStrandedCmd(stdout, stderr),
	)
	return cmd
}

func newConvoyCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name> [issue-ids...]",
		Short: "Create a convoy and optionally track issues",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdConvoyCreate(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyCreate is the CLI entry point for creating a convoy.
func cmdConvoyCreate(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy create")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doConvoyCreate(store, rec, args, stdout, stderr)
}

// doConvoyCreate creates a convoy bead and optionally adds issues to it.
func doConvoyCreate(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc convoy create: missing convoy name") //nolint:errcheck // best-effort stderr
		return 1
	}
	name := args[0]
	issueIDs := args[1:]

	convoy, err := store.Create(beads.Bead{Title: name, Type: "convoy"})
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	for _, id := range issueIDs {
		if _, err := store.Get(id); err != nil {
			fmt.Fprintf(stderr, "gc convoy create: issue %s: %v\n", id, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		parentID := convoy.ID
		if err := store.Update(id, beads.UpdateOpts{ParentID: &parentID}); err != nil {
			fmt.Fprintf(stderr, "gc convoy create: setting parent on %s: %v\n", id, err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	rec.Record(events.Event{
		Type:    events.ConvoyCreated,
		Actor:   eventActor(),
		Subject: convoy.ID,
		Message: name,
	})

	if len(issueIDs) > 0 {
		fmt.Fprintf(stdout, "Created convoy %s %q tracking %d issue(s)\n", convoy.ID, name, len(issueIDs)) //nolint:errcheck // best-effort stdout
	} else {
		fmt.Fprintf(stdout, "Created convoy %s %q\n", convoy.ID, name) //nolint:errcheck // best-effort stdout
	}
	return 0
}

func newConvoyListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List open convoys with progress",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdConvoyList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyList is the CLI entry point for listing convoys.
func cmdConvoyList(stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy list")
	if store == nil {
		return code
	}
	return doConvoyList(store, stdout, stderr)
}

// doConvoyList lists open convoys with progress counts.
func doConvoyList(store beads.Store, stdout, stderr io.Writer) int {
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	var convoys []beads.Bead
	for _, b := range all {
		if b.Type == "convoy" && b.Status != "closed" {
			convoys = append(convoys, b)
		}
	}

	if len(convoys) == 0 {
		fmt.Fprintln(stdout, "No open convoys") //nolint:errcheck // best-effort stdout
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tPROGRESS") //nolint:errcheck // best-effort stdout
	for _, c := range convoys {
		children, err := store.Children(c.ID)
		if err != nil {
			fmt.Fprintf(stderr, "gc convoy list: children of %s: %v\n", c.ID, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		closed := 0
		for _, ch := range children {
			if ch.Status == "closed" {
				closed++
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%d/%d closed\n", c.ID, c.Title, closed, len(children)) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

func newConvoyStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show detailed convoy status",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdConvoyStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyStatus is the CLI entry point for convoy status.
func cmdConvoyStatus(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy status")
	if store == nil {
		return code
	}
	return doConvoyStatus(store, args, stdout, stderr)
}

// doConvoyStatus shows detailed status of a convoy and its children.
func doConvoyStatus(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc convoy status: missing convoy ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	id := args[0]

	convoy, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if convoy.Type != "convoy" {
		fmt.Fprintf(stderr, "gc convoy status: bead %s is not a convoy\n", id) //nolint:errcheck // best-effort stderr
		return 1
	}

	children, err := store.Children(id)
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	closed := 0
	for _, ch := range children {
		if ch.Status == "closed" {
			closed++
		}
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("Convoy:   %s", convoy.ID))
	w(fmt.Sprintf("Title:    %s", convoy.Title))
	w(fmt.Sprintf("Status:   %s", convoy.Status))
	w(fmt.Sprintf("Progress: %d/%d closed", closed, len(children)))

	if len(children) > 0 {
		w("")
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTITLE\tSTATUS\tASSIGNEE") //nolint:errcheck // best-effort stdout
		for _, ch := range children {
			assignee := ch.Assignee
			if assignee == "" {
				assignee = "-"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ch.ID, ch.Title, ch.Status, assignee) //nolint:errcheck // best-effort stdout
		}
		tw.Flush() //nolint:errcheck // best-effort stdout
	}
	return 0
}

func newConvoyAddCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "add <convoy-id> <issue-id>",
		Short: "Add an issue to a convoy",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdConvoyAdd(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyAdd is the CLI entry point for adding an issue to a convoy.
func cmdConvoyAdd(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy add")
	if store == nil {
		return code
	}
	return doConvoyAdd(store, args, stdout, stderr)
}

// doConvoyAdd adds an issue to a convoy by setting the issue's ParentID.
func doConvoyAdd(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc convoy add: usage: gc convoy add <convoy-id> <issue-id>") //nolint:errcheck // best-effort stderr
		return 1
	}
	convoyID := args[0]
	issueID := args[1]

	convoy, err := store.Get(convoyID)
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if convoy.Type != "convoy" {
		fmt.Fprintf(stderr, "gc convoy add: bead %s is not a convoy\n", convoyID) //nolint:errcheck // best-effort stderr
		return 1
	}

	if _, err := store.Get(issueID); err != nil {
		fmt.Fprintf(stderr, "gc convoy add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if err := store.Update(issueID, beads.UpdateOpts{ParentID: &convoyID}); err != nil {
		fmt.Fprintf(stderr, "gc convoy add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Added %s to convoy %s\n", issueID, convoyID) //nolint:errcheck // best-effort stdout
	return 0
}

func newConvoyCloseCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "Close a convoy",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdConvoyClose(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyClose is the CLI entry point for closing a convoy.
func cmdConvoyClose(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy close")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doConvoyClose(store, rec, args, stdout, stderr)
}

// doConvoyClose closes a convoy bead.
func doConvoyClose(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc convoy close: missing convoy ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	id := args[0]

	convoy, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy close: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if convoy.Type != "convoy" {
		fmt.Fprintf(stderr, "gc convoy close: bead %s is not a convoy\n", id) //nolint:errcheck // best-effort stderr
		return 1
	}

	if err := store.Close(id); err != nil {
		fmt.Fprintf(stderr, "gc convoy close: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	rec.Record(events.Event{
		Type:    events.ConvoyClosed,
		Actor:   eventActor(),
		Subject: id,
	})

	fmt.Fprintf(stdout, "Closed convoy %s\n", id) //nolint:errcheck // best-effort stdout
	return 0
}

func newConvoyCheckCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Auto-close convoys where all issues are closed",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdConvoyCheck(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyCheck is the CLI entry point for auto-closing completed convoys.
func cmdConvoyCheck(stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy check")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doConvoyCheck(store, rec, stdout, stderr)
}

// doConvoyCheck auto-closes convoys where all children are closed.
func doConvoyCheck(store beads.Store, rec events.Recorder, stdout, stderr io.Writer) int {
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy check: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	closed := 0
	for _, b := range all {
		if b.Type != "convoy" || b.Status == "closed" {
			continue
		}
		children, err := store.Children(b.ID)
		if err != nil {
			fmt.Fprintf(stderr, "gc convoy check: children of %s: %v\n", b.ID, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if len(children) == 0 {
			continue
		}
		allClosed := true
		for _, ch := range children {
			if ch.Status != "closed" {
				allClosed = false
				break
			}
		}
		if allClosed {
			if err := store.Close(b.ID); err != nil {
				fmt.Fprintf(stderr, "gc convoy check: closing %s: %v\n", b.ID, err) //nolint:errcheck // best-effort stderr
				return 1
			}
			rec.Record(events.Event{
				Type:    events.ConvoyClosed,
				Actor:   eventActor(),
				Subject: b.ID,
			})
			fmt.Fprintf(stdout, "Auto-closed convoy %s %q\n", b.ID, b.Title) //nolint:errcheck // best-effort stdout
			closed++
		}
	}

	fmt.Fprintf(stdout, "%d convoy(s) auto-closed\n", closed) //nolint:errcheck // best-effort stdout
	return 0
}

func newConvoyStrandedCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stranded",
		Short: "Find convoys with ready work but no workers",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdConvoyStranded(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdConvoyStranded is the CLI entry point for finding stranded convoys.
func cmdConvoyStranded(stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc convoy stranded")
	if store == nil {
		return code
	}
	return doConvoyStranded(store, stdout, stderr)
}

// doConvoyStranded finds open convoys with open children that have no assignee.
func doConvoyStranded(store beads.Store, stdout, stderr io.Writer) int {
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc convoy stranded: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	type strandedItem struct {
		convoyID string
		issue    beads.Bead
	}
	var items []strandedItem

	for _, b := range all {
		if b.Type != "convoy" || b.Status == "closed" {
			continue
		}
		children, err := store.Children(b.ID)
		if err != nil {
			fmt.Fprintf(stderr, "gc convoy stranded: children of %s: %v\n", b.ID, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		for _, ch := range children {
			if ch.Status != "closed" && ch.Assignee == "" {
				items = append(items, strandedItem{convoyID: b.ID, issue: ch})
			}
		}
	}

	if len(items) == 0 {
		fmt.Fprintln(stdout, "No stranded work") //nolint:errcheck // best-effort stdout
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CONVOY\tISSUE\tTITLE") //nolint:errcheck // best-effort stdout
	for _, item := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", item.convoyID, item.issue.ID, item.issue.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}
