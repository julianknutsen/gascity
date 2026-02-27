package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/events"
)

func newEventsCmd(stdout, stderr io.Writer) *cobra.Command {
	var typeFilter string
	var sinceFlag string
	var watchFlag bool
	var seqFlag bool
	var timeoutFlag string
	var afterFlag uint64
	var payloadMatch []string

	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show the event log",
		Long: `Show the city event log with optional filtering.

Events are recorded to .gc/events.jsonl by the controller, agent
lifecycle operations, and bead mutations. Use --type and --since to
filter. Use --watch to block until matching events arrive (useful for
scripting and automation).`,
		Example: `  gc events
  gc events --type bead.created --since 1h
  gc events --watch --type convoy.closed --timeout 5m
  gc events --seq`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if seqFlag {
				if cmdEventsSeq(stdout, stderr) != 0 {
					return errExit
				}
				return nil
			}
			if watchFlag {
				if cmdEventsWatch(typeFilter, payloadMatch, afterFlag, timeoutFlag, stdout, stderr) != 0 {
					return errExit
				}
				return nil
			}
			if cmdEvents(typeFilter, sinceFlag, payloadMatch, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by event type (e.g. bead.created)")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "Show events since duration ago (e.g. 1h, 30m)")
	cmd.Flags().BoolVar(&watchFlag, "watch", false, "Block until matching events arrive")
	cmd.Flags().BoolVar(&seqFlag, "seq", false, "Print the current head sequence number and exit")
	cmd.Flags().StringVar(&timeoutFlag, "timeout", "30s", "Max wait duration for --watch (e.g. 30s, 5m)")
	cmd.Flags().Uint64Var(&afterFlag, "after", 0, "Resume watching from this sequence number (0 = current head)")
	cmd.Flags().StringArrayVar(&payloadMatch, "payload-match", nil, "Filter by payload field (key=value, repeatable)")
	return cmd
}

// cmdEvents is the CLI entry point for viewing the event log.
func cmdEvents(typeFilter, sinceFlag string, payloadMatchArgs []string, stdout, stderr io.Writer) int {
	pm, err := parsePayloadMatch(payloadMatchArgs)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	ep, code := openCityEventsProvider(stderr, "gc events")
	if ep == nil {
		return code
	}
	defer ep.Close() //nolint:errcheck // best-effort
	return doEvents(ep, typeFilter, sinceFlag, pm, stdout, stderr)
}

// cmdEventsSeq prints the current head sequence number.
func cmdEventsSeq(stdout, stderr io.Writer) int {
	ep, code := openCityEventsProvider(stderr, "gc events")
	if ep == nil {
		return code
	}
	defer ep.Close() //nolint:errcheck // best-effort
	return doEventsSeq(ep, stdout, stderr)
}

// doEventsSeq prints the current head sequence number. Returns 0 on
// success. Prints "0" if the event log is missing or empty.
func doEventsSeq(ep events.Provider, stdout, stderr io.Writer) int {
	seq, err := ep.LatestSeq()
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintln(stdout, seq) //nolint:errcheck // best-effort stdout
	return 0
}

// doEvents reads and displays events from the provider.
func doEvents(ep events.Provider, typeFilter, sinceFlag string, payloadMatch map[string][]string, stdout, stderr io.Writer) int {
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

	evts, err := ep.List(filter)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Apply payload-match filter if specified.
	if len(payloadMatch) > 0 {
		evts = filterEventsByPayload(evts, payloadMatch)
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

// filterEventsByPayload returns events that match all payload criteria.
func filterEventsByPayload(evts []events.Event, pm map[string][]string) []events.Event {
	var out []events.Event
	for _, e := range evts {
		if matchPayload(e.Payload, pm) {
			out = append(out, e)
		}
	}
	return out
}

// cmdEventsWatch is the CLI entry point for watch mode.
func cmdEventsWatch(typeFilter string, payloadMatch []string, afterSeq uint64, timeoutFlag string, stdout, stderr io.Writer) int {
	timeout, err := time.ParseDuration(timeoutFlag)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: invalid --timeout %q: %v\n", timeoutFlag, err) //nolint:errcheck // best-effort stderr
		return 1
	}

	pm, err := parsePayloadMatch(payloadMatch)
	if err != nil {
		fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	ep, code := openCityEventsProvider(stderr, "gc events")
	if ep == nil {
		return code
	}
	defer ep.Close() //nolint:errcheck // best-effort
	return doEventsWatch(ep, typeFilter, pm, afterSeq, timeout, 250*time.Millisecond, stdout, stderr)
}

// doEventsWatch polls the event provider for new events matching the filter.
// It blocks until matching events arrive or the timeout expires. Outputs
// matching events as JSON lines (one per line). Returns 0 always — empty
// stdout means timeout, non-empty means events found.
func doEventsWatch(ep events.Provider, typeFilter string, payloadMatch map[string][]string, afterSeq uint64, timeout, pollInterval time.Duration, stdout, stderr io.Writer) int {
	explicitAfterSeq := afterSeq > 0

	// Determine starting point.
	if afterSeq == 0 {
		seq, err := ep.LatestSeq()
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		afterSeq = seq
	}

	// When afterSeq was explicitly provided, check existing events first.
	// Some may already be past the requested sequence number.
	if explicitAfterSeq {
		evts, err := ep.List(events.Filter{AfterSeq: afterSeq})
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if matches := filterEvents(evts, afterSeq, typeFilter, payloadMatch); len(matches) > 0 {
			return printEventsJSON(matches, stdout, stderr)
		}
	}

	deadline := time.Now().Add(timeout)
	lastSeq := afterSeq

	for {
		evts, err := ep.List(events.Filter{AfterSeq: lastSeq})
		if err != nil {
			fmt.Fprintf(stderr, "gc events: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}

		// Track the highest seq we've seen.
		for _, e := range evts {
			if e.Seq > lastSeq {
				lastSeq = e.Seq
			}
		}

		if matches := filterEvents(evts, afterSeq, typeFilter, payloadMatch); len(matches) > 0 {
			return printEventsJSON(matches, stdout, stderr)
		}

		if time.Now().After(deadline) {
			return 0
		}

		time.Sleep(pollInterval)
	}
}

// filterEvents returns events with Seq > afterSeq that match typeFilter
// and all payloadMatch criteria.
func filterEvents(evts []events.Event, afterSeq uint64, typeFilter string, payloadMatch map[string][]string) []events.Event {
	var matches []events.Event
	for _, e := range evts {
		if e.Seq <= afterSeq {
			continue
		}
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		if !matchPayload(e.Payload, payloadMatch) {
			continue
		}
		matches = append(matches, e)
	}
	return matches
}

// matchPayload checks whether the event payload satisfies all key constraints.
// Same key with multiple values = OR (any value matches).
// Different keys = AND (all keys must match).
// Returns true if payloadMatch is empty.
func matchPayload(payload json.RawMessage, payloadMatch map[string][]string) bool {
	if len(payloadMatch) == 0 {
		return true
	}
	if len(payload) == 0 {
		return false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(payload, &obj) != nil {
		return false
	}
	for k, wants := range payloadMatch {
		raw, ok := obj[k]
		if !ok {
			return false
		}
		// Resolve the actual value (try string unquote, fall back to raw).
		var got string
		if json.Unmarshal(raw, &got) != nil {
			got = string(raw)
		}
		// OR: any value in the list matches.
		matched := false
		for _, w := range wants {
			if got == w {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// parsePayloadMatch parses "key=value" strings into a multi-map.
// Same key repeated = OR (match any value for that key).
// Different keys = AND (all keys must match).
func parsePayloadMatch(args []string) (map[string][]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	m := make(map[string][]string, len(args))
	for _, arg := range args {
		i := strings.IndexByte(arg, '=')
		if i < 1 {
			return nil, fmt.Errorf("invalid --payload-match %q: expected key=value", arg)
		}
		k, v := arg[:i], arg[i+1:]
		m[k] = append(m[k], v)
	}
	return m, nil
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
