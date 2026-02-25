package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/events"
)

func newEventCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Event operations",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc event: missing subcommand (emit)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc event: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(newEventEmitCmd(stdout, stderr))
	return cmd
}

func newEventEmitCmd(_, stderr io.Writer) *cobra.Command {
	var subject, message, actor string

	cmd := &cobra.Command{
		Use:   "emit <type>",
		Short: "Emit an event to the city event log",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdEventEmit(args[0], subject, message, actor, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&subject, "subject", "", "Event subject (e.g. bead ID)")
	cmd.Flags().StringVar(&message, "message", "", "Event message")
	cmd.Flags().StringVar(&actor, "actor", "", "Actor name (default: GC_AGENT or \"human\")")
	return cmd
}

// cmdEventEmit records a single event to the city event log. Best-effort:
// errors go to stderr but exit code is always 0 so bd hooks never fail.
func cmdEventEmit(eventType, subject, message, actor string, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc event emit: %v\n", err) //nolint:errcheck // best-effort stderr
		return 0                                        // best-effort — never fail
	}
	return doEventEmit(filepath.Join(cityPath, ".gc", "events.jsonl"),
		eventType, subject, message, actor, stderr)
}

// doEventEmit is the pure logic for "gc event emit". Accepts the event log
// path directly for testability.
func doEventEmit(eventsPath, eventType, subject, message, actor string, stderr io.Writer) int {
	if actor == "" {
		actor = eventActor()
	}

	rec, err := events.NewFileRecorder(eventsPath, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "gc event emit: %v\n", err) //nolint:errcheck // best-effort stderr
		return 0                                        // best-effort — never fail
	}
	defer rec.Close() //nolint:errcheck // best-effort

	rec.Record(events.Event{
		Type:    eventType,
		Actor:   actor,
		Subject: subject,
		Message: message,
	})
	return 0
}
