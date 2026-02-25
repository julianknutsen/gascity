package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newMailCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Send and receive messages between agents and humans",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc mail: missing subcommand (inbox, read, send)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc mail: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newMailSendCmd(stdout, stderr),
		newMailInboxCmd(stdout, stderr),
		newMailReadCmd(stdout, stderr),
	)
	return cmd
}

func newMailSendCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "send <to> <body>",
		Short: "Send a message to an agent or human",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailSend(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newMailInboxCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "inbox [agent]",
		Short: "List unread messages (defaults to your inbox)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailInbox(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newMailReadCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "read <id>",
		Short: "Read a message and mark it as read",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailRead(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdMailSend is the CLI entry point for sending mail. It opens the store,
// loads config for recipient validation, and delegates to doMailSend.
func cmdMailSend(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc mail send")
	if store == nil {
		return code
	}

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	validRecipients := make(map[string]bool)
	validRecipients["human"] = true
	for _, a := range cfg.Agents {
		validRecipients[a.Name] = true
	}

	sender := os.Getenv("GC_AGENT")
	if sender == "" {
		sender = "human"
	}

	rec := openCityRecorder(stderr)
	return doMailSend(store, rec, validRecipients, sender, args, stdout, stderr)
}

// doMailSend creates a message bead addressed to a recipient. The sender is
// determined by the caller (GC_AGENT env var or "human"). Accepts an injected
// store, recorder, and recipient set for testability.
func doMailSend(store beads.Store, rec events.Recorder, validRecipients map[string]bool, sender string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc mail send: usage: gc mail send <to> <body>") //nolint:errcheck // best-effort stderr
		return 1
	}
	to := args[0]
	body := args[1]

	if !validRecipients[to] {
		fmt.Fprintf(stderr, "gc mail send: unknown recipient %q\n", to) //nolint:errcheck // best-effort stderr
		return 1
	}

	b, err := store.Create(beads.Bead{
		Title:    body,
		Type:     "message",
		Assignee: to,
		From:     sender,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.MailSent,
		Actor:   sender,
		Subject: b.ID,
		Message: to,
	})
	fmt.Fprintf(stdout, "Sent message %s to %s\n", b.ID, to) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdMailInbox is the CLI entry point for checking the inbox.
func cmdMailInbox(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc mail inbox")
	if store == nil {
		return code
	}

	recipient := os.Getenv("GC_AGENT")
	if recipient == "" {
		recipient = "human"
	}
	if len(args) > 0 {
		recipient = args[0]
	}

	return doMailInbox(store, recipient, stdout, stderr)
}

// doMailInbox lists unread messages for a recipient. Messages are beads with
// Type="message", Status="open", and Assignee matching the recipient.
func doMailInbox(store beads.Store, recipient string, stdout, stderr io.Writer) int {
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc mail inbox: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	var messages []beads.Bead
	for _, b := range all {
		if b.Type == "message" && b.Status == "open" && b.Assignee == recipient {
			messages = append(messages, b)
		}
	}

	if len(messages) == 0 {
		fmt.Fprintf(stdout, "No unread messages for %s\n", recipient) //nolint:errcheck // best-effort stdout
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tFROM\tBODY") //nolint:errcheck // best-effort stdout
	for _, m := range messages {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", m.ID, m.From, m.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdMailRead is the CLI entry point for reading a message.
func cmdMailRead(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc mail read")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doMailRead(store, rec, args, stdout, stderr)
}

// doMailRead displays a message and marks it as read (closes the bead).
// Accepts an injected store and recorder for testability.
func doMailRead(store beads.Store, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mail read: missing message ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	id := args[0]

	b, err := store.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "gc mail read: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("ID:     %s", b.ID))
	w(fmt.Sprintf("From:   %s", b.From))
	w(fmt.Sprintf("To:     %s", b.Assignee))
	w(fmt.Sprintf("Sent:   %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
	w(fmt.Sprintf("Body:   %s", b.Title))

	if b.Status != "closed" {
		if err := store.Close(id); err != nil {
			fmt.Fprintf(stderr, "gc mail read: marking as read: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		rec.Record(events.Event{
			Type:    events.MailRead,
			Actor:   eventActor(),
			Subject: id,
		})
	}
	return 0
}
