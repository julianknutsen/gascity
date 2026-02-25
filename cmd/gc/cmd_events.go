package main

import (
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/events"
)

func newEventsCmd(stdout, stderr io.Writer) *cobra.Command {
	var typeFilter string
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show the event log",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdEvents(typeFilter, sinceFlag, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by event type (e.g. bead.created)")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "Show events since duration ago (e.g. 1h, 30m)")
	return cmd
}

// cmdEvents is the CLI entry point for viewing the event log.
func cmdEvents(typeFilter, sinceFlag string, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	path := filepath.Join(cityPath, ".gc", "events.jsonl")
	return doEvents(path, typeFilter, sinceFlag, stdout, stderr)
}

// doEvents reads and displays events from the log file. Accepts the path
// directly for testability.
func doEvents(path, typeFilter, sinceFlag string, stdout, stderr io.Writer) int {
	var filter events.Filter
	filter.Type = typeFilter

	if sinceFlag != "" {
		d, err := time.ParseDuration(sinceFlag)
		if err != nil {
			fmt.Fprintf(stderr, "gc events: invalid --since %q: %v\n", sinceFlag, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		filter.Since = time.Now().Add(-d)
	}

	var evts []events.Event
	var err error
	if filter.Type != "" || !filter.Since.IsZero() {
		evts, err = events.ReadFiltered(path, filter)
	} else {
		evts, err = events.ReadAll(path)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if len(evts) == 0 {
		fmt.Fprintln(stdout, "No events.") //nolint:errcheck // best-effort stdout
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEQ\tTYPE\tACTOR\tSUBJECT\tMESSAGE\tTIME") //nolint:errcheck // best-effort stdout
	for _, e := range evts {
		msg := e.Message
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", //nolint:errcheck // best-effort stdout
			e.Seq, e.Type, e.Actor, e.Subject, msg,
			e.Ts.Format("2006-01-02 15:04:05"),
		)
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}
