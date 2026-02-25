package main

import (
	"encoding/json"
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
	var watchFlag bool
	var timeoutFlag string
	var afterFlag uint64

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show the event log",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if watchFlag {
				if cmdEventsWatch(typeFilter, afterFlag, timeoutFlag, stdout, stderr) != 0 {
					return errExit
				}
				return nil
			}
			if cmdEvents(typeFilter, sinceFlag, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by event type (e.g. bead.created)")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "Show events since duration ago (e.g. 1h, 30m)")
	cmd.Flags().BoolVar(&watchFlag, "watch", false, "Block until matching events arrive")
	cmd.Flags().StringVar(&timeoutFlag, "timeout", "30s", "Max wait duration for --watch (e.g. 30s, 5m)")
	cmd.Flags().Uint64Var(&afterFlag, "after", 0, "Resume watching from this sequence number (0 = current head)")
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

// cmdEventsWatch is the CLI entry point for watch mode.
func cmdEventsWatch(typeFilter string, afterSeq uint64, timeoutFlag string, stdout, stderr io.Writer) int {
	timeout, err := time.ParseDuration(timeoutFlag)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: invalid --timeout %q: %v\n", timeoutFlag, err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	path := filepath.Join(cityPath, ".gc", "events.jsonl")
	return doEventsWatch(path, typeFilter, afterSeq, timeout, 250*time.Millisecond, stdout, stderr)
}

// doEventsWatch polls the event log for new events matching the filter.
// It blocks until matching events arrive or the timeout expires. Outputs
// matching events as JSON lines (one per line). Returns 0 always — empty
// stdout means timeout, non-empty means events found.
func doEventsWatch(path, typeFilter string, afterSeq uint64, timeout, pollInterval time.Duration, stdout, stderr io.Writer) int {
	explicitAfterSeq := afterSeq > 0

	// Determine starting point.
	if afterSeq == 0 {
		seq, err := events.ReadLatestSeq(path)
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		afterSeq = seq
	}

	// When afterSeq was explicitly provided, check existing events first.
	// Some may already be past the requested sequence number.
	if explicitAfterSeq {
		all, err := events.ReadAll(path)
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if matches := filterEvents(all, afterSeq, typeFilter); len(matches) > 0 {
			return printEventsJSON(matches, stdout, stderr)
		}
	}

	// Get starting byte offset (current end of file).
	_, offset, err := events.ReadFrom(path, 0)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	deadline := time.Now().Add(timeout)

	for {
		evts, newOffset, err := events.ReadFrom(path, offset)
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		offset = newOffset

		if matches := filterEvents(evts, afterSeq, typeFilter); len(matches) > 0 {
			return printEventsJSON(matches, stdout, stderr)
		}

		if time.Now().After(deadline) {
			return 0
		}

		time.Sleep(pollInterval)
	}
}

// filterEvents returns events with Seq > afterSeq that match typeFilter.
func filterEvents(evts []events.Event, afterSeq uint64, typeFilter string) []events.Event {
	var matches []events.Event
	for _, e := range evts {
		if e.Seq <= afterSeq {
			continue
		}
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		matches = append(matches, e)
	}
	return matches
}

// printEventsJSON writes events as JSON lines to stdout. Returns 0.
func printEventsJSON(evts []events.Event, stdout, stderr io.Writer) int {
	for _, e := range evts {
		data, err := json.Marshal(e)
		if err != nil {
			fmt.Fprintf(stderr, "gc events: marshal: %v\n", err) //nolint:errcheck // best-effort stderr
			continue
		}
		fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
	}
	return 0
}
