package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/mail"
)

// nudgeFunc is an optional callback for nudging an agent after sending mail.
// When non-nil, it is called with the recipient name. Errors are non-fatal.
type nudgeFunc func(recipient string) error

func newMailCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Send and receive messages between agents and humans",
		Long: `Send and receive messages between agents and humans.

Mail is implemented as beads with type="message". Messages have a
sender, recipient, and body. Use "gc mail check --inject" in agent
hooks to deliver mail notifications into agent prompts.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc mail: missing subcommand (archive, check, inbox, read, send)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc mail: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newMailArchiveCmd(stdout, stderr),
		newMailCheckCmd(stdout, stderr),
		newMailSendCmd(stdout, stderr),
		newMailInboxCmd(stdout, stderr),
		newMailReadCmd(stdout, stderr),
	)
	return cmd
}

func newMailArchiveCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive a message without reading it",
		Long: `Close a message bead without displaying its contents.

Use this to dismiss a message without reading it. The message is marked
as closed and will no longer appear in mail check or inbox results.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailArchive(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdMailArchive is the CLI entry point for archiving a message.
func cmdMailArchive(args []string, stdout, stderr io.Writer) int {
	mp, code := openCityMailProvider(stderr, "gc mail archive")
	if mp == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doMailArchive(mp, rec, args, stdout, stderr)
}

// doMailArchive closes a message without displaying it. Accepts an
// injected provider and recorder for testability.
func doMailArchive(mp mail.Provider, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mail archive: missing message ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	id := args[0]

	if err := mp.Archive(id); err != nil {
		if errors.Is(err, mail.ErrAlreadyArchived) {
			fmt.Fprintf(stdout, "Already archived %s\n", id) //nolint:errcheck // best-effort stdout
			return 0
		}
		fmt.Fprintf(stderr, "gc mail archive: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.MailRead,
		Actor:   eventActor(),
		Subject: id,
	})
	fmt.Fprintf(stdout, "Archived message %s\n", id) //nolint:errcheck // best-effort stdout
	return 0
}

func newMailCheckCmd(stdout, stderr io.Writer) *cobra.Command {
	var inject bool
	cmd := &cobra.Command{
		Use:   "check [agent]",
		Short: "Check for unread mail (use --inject for hook output)",
		Long: `Check for unread mail addressed to an agent.

Without --inject: prints the count and exits 0 if mail exists, 1 if
empty. With --inject: outputs a <system-reminder> block suitable for
hook injection (always exits 0). The recipient defaults to $GC_AGENT
or "human".`,
		Example: `  gc mail check
  gc mail check --inject
  gc mail check mayor`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailCheck(args, inject, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&inject, "inject", false, "output <system-reminder> block for hook injection")
	return cmd
}

// cmdMailCheck is the CLI entry point for checking mail.
func cmdMailCheck(args []string, inject bool, stdout, stderr io.Writer) int {
	// Check city-level suspension before opening the store.
	if cityPath, err := resolveCity(); err == nil {
		if cfg, err := loadCityConfig(cityPath); err == nil {
			if citySuspended(cfg) {
				if inject {
					return 0
				}
				fmt.Fprintln(stderr, "gc mail check: city is suspended") //nolint:errcheck // best-effort stderr
				return 1
			}
		}
	}

	mp, code := openCityMailProvider(stderr, "gc mail check")
	if mp == nil {
		if inject {
			return 0 // --inject always exits 0
		}
		return code
	}

	recipient := os.Getenv("GC_AGENT")
	if recipient == "" {
		recipient = "human"
	}
	if len(args) > 0 {
		recipient = args[0]
	}

	return doMailCheck(mp, recipient, inject, stdout, stderr)
}

// doMailCheck checks for unread messages. Without --inject, prints the count
// and returns 0 if mail exists, 1 if empty. With --inject, outputs a
// <system-reminder> block for hook injection and always returns 0.
func doMailCheck(mp mail.Provider, recipient string, inject bool, stdout, stderr io.Writer) int {
	messages, err := mp.Check(recipient)
	if err != nil {
		if inject {
			fmt.Fprintf(stderr, "gc mail check: %v\n", err) //nolint:errcheck // best-effort stderr
			return 0                                        // --inject always exits 0
		}
		fmt.Fprintf(stderr, "gc mail check: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if inject {
		if len(messages) > 0 {
			fmt.Fprint(stdout, formatInjectOutput(messages)) //nolint:errcheck // best-effort stdout
		}
		return 0 // --inject always exits 0
	}

	// Non-inject mode: print count, return 0 if mail, 1 if empty.
	if len(messages) == 0 {
		return 1
	}
	fmt.Fprintf(stdout, "%d unread message(s) for %s\n", len(messages), recipient) //nolint:errcheck // best-effort stdout
	return 0
}

// formatInjectOutput formats messages as a <system-reminder> block for
// injection into an agent's prompt via a UserPromptSubmit hook.
func formatInjectOutput(messages []mail.Message) string {
	var sb strings.Builder
	sb.WriteString("<system-reminder>\n")
	sb.WriteString(fmt.Sprintf("You have %d unread message(s).\n\n", len(messages)))
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("- %s from %s: %s\n", m.ID, m.From, m.Body))
	}
	sb.WriteString("\nRun 'gc mail read <id>' for full details, or 'gc mail inbox' to see all.\n")
	sb.WriteString("</system-reminder>\n")
	return sb.String()
}

func newMailSendCmd(stdout, stderr io.Writer) *cobra.Command {
	var notify bool
	var all bool
	var from string
	cmd := &cobra.Command{
		Use:   "send <to> <body>",
		Short: "Send a message to an agent or human",
		Long: `Send a message to an agent or human.

Creates a message bead addressed to the recipient. The sender defaults
to $GC_AGENT (in agent sessions) or "human". Use --notify to nudge
the recipient after sending. Use --from to override the sender identity.
Use --all to broadcast to all agents (excluding sender and "human").`,
		Example: `  gc mail send mayor "Build is green"
  gc mail send human "Review needed for PR #42"
  gc mail send polecat "Priority task" --notify
  gc mail send --all "Status update: tests passing"`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailSend(args, notify, all, from, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&notify, "notify", false, "nudge the recipient after sending")
	cmd.Flags().BoolVar(&all, "all", false, "broadcast to all agents (excludes sender and human)")
	cmd.Flags().StringVar(&from, "from", "", "sender identity (default: $GC_AGENT or \"human\")")
	return cmd
}

func newMailInboxCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "inbox [agent]",
		Short: "List unread messages (defaults to your inbox)",
		Long: `List all unread messages for an agent or human.

Shows message ID, sender, and body in a table. The recipient defaults
to $GC_AGENT or "human". Pass an agent name to view another agent's inbox.`,
		Args: cobra.ArbitraryArgs,
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
		Long: `Display a message and mark it as read.

Shows the full message details (ID, sender, recipient, date, body) and
closes the message bead. Closed messages no longer appear in mail check
or inbox results.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMailRead(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdMailSend is the CLI entry point for sending mail. It opens the provider,
// loads config for recipient validation, and delegates to doMailSend.
func cmdMailSend(args []string, notify bool, all bool, from string, stdout, stderr io.Writer) int {
	mp, code := openCityMailProvider(stderr, "gc mail send")
	if mp == nil {
		return code
	}

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	validRecipients := make(map[string]bool)
	validRecipients["human"] = true
	for _, a := range cfg.Agents {
		validRecipients[a.QualifiedName()] = true
	}

	sender := from
	if sender == "" {
		sender = os.Getenv("GC_AGENT")
	}
	if sender == "" {
		sender = "human"
	}

	var nf nudgeFunc
	if notify {
		cityName := cfg.Workspace.Name
		if cityName == "" {
			cityName = filepath.Base(cityPath)
		}
		nf = func(recipient string) error {
			found, ok := resolveAgentIdentity(cfg, recipient, currentRigContext(cfg))
			if !ok {
				return fmt.Errorf("agent %q not found", recipient)
			}
			sp := newSessionProvider()
			a := agent.New(found.QualifiedName(), cityName, "", "", nil, agent.StartupHints{}, "", cfg.Workspace.SessionTemplate, nil, sp)
			return a.Nudge(fmt.Sprintf("You have mail from %s", sender))
		}
	}

	if all {
		rec := openCityRecorder(stderr)
		return doMailSendAll(mp, rec, validRecipients, sender, args, nf, stdout, stderr)
	}

	rec := openCityRecorder(stderr)
	return doMailSend(mp, rec, validRecipients, sender, args, nf, stdout, stderr)
}

// doMailSend creates a message addressed to a recipient. The sender is
// determined by the caller (GC_AGENT env var or "human"). When nudgeFn is
// non-nil, the recipient is nudged after message creation (skipped for
// "human"). Nudge errors are non-fatal. Accepts an injected provider,
// recorder, recipient set, and nudge callback for testability.
func doMailSend(mp mail.Provider, rec events.Recorder, validRecipients map[string]bool, sender string, args []string, nudgeFn nudgeFunc, stdout, stderr io.Writer) int {
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

	m, err := mp.Send(sender, to, body)
	if err != nil {
		fmt.Fprintf(stderr, "gc mail send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.MailSent,
		Actor:   sender,
		Subject: m.ID,
		Message: to,
	})
	fmt.Fprintf(stdout, "Sent message %s to %s\n", m.ID, to) //nolint:errcheck // best-effort stdout

	// Nudge recipient if requested and recipient is not human.
	if nudgeFn != nil && to != "human" {
		if err := nudgeFn(to); err != nil {
			fmt.Fprintf(stderr, "gc mail send: nudge failed: %v\n", err) //nolint:errcheck // best-effort stderr
		}
	}
	return 0
}

// doMailSendAll broadcasts a message to all configured agents (excluding the
// sender and "human"). With --all, args is just [body] (no recipient).
func doMailSendAll(mp mail.Provider, rec events.Recorder, validRecipients map[string]bool, sender string, args []string, nudgeFn nudgeFunc, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mail send --all: usage: gc mail send --all <body>") //nolint:errcheck // best-effort stderr
		return 1
	}
	body := args[0]

	// Collect recipients in sorted order for deterministic output.
	var recipients []string
	for r := range validRecipients {
		if r == sender || r == "human" {
			continue
		}
		recipients = append(recipients, r)
	}
	sort.Strings(recipients)

	if len(recipients) == 0 {
		fmt.Fprintln(stderr, "gc mail send --all: no recipients (all agents excluded)") //nolint:errcheck // best-effort stderr
		return 1
	}

	for _, to := range recipients {
		m, err := mp.Send(sender, to, body)
		if err != nil {
			fmt.Fprintf(stderr, "gc mail send --all: sending to %s: %v\n", to, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		rec.Record(events.Event{
			Type:    events.MailSent,
			Actor:   sender,
			Subject: m.ID,
			Message: to,
		})
		fmt.Fprintf(stdout, "Sent message %s to %s\n", m.ID, to) //nolint:errcheck // best-effort stdout

		if nudgeFn != nil {
			if err := nudgeFn(to); err != nil {
				fmt.Fprintf(stderr, "gc mail send --all: nudge %s failed: %v\n", to, err) //nolint:errcheck // best-effort stderr
			}
		}
	}
	return 0
}

// cmdMailInbox is the CLI entry point for checking the inbox.
func cmdMailInbox(args []string, stdout, stderr io.Writer) int {
	mp, code := openCityMailProvider(stderr, "gc mail inbox")
	if mp == nil {
		return code
	}

	recipient := os.Getenv("GC_AGENT")
	if recipient == "" {
		recipient = "human"
	}
	if len(args) > 0 {
		recipient = args[0]
	}

	return doMailInbox(mp, recipient, stdout, stderr)
}

// doMailInbox lists unread messages for a recipient.
func doMailInbox(mp mail.Provider, recipient string, stdout, stderr io.Writer) int {
	messages, err := mp.Inbox(recipient)
	if err != nil {
		fmt.Fprintf(stderr, "gc mail inbox: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if len(messages) == 0 {
		fmt.Fprintf(stdout, "No unread messages for %s\n", recipient) //nolint:errcheck // best-effort stdout
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tFROM\tBODY") //nolint:errcheck // best-effort stdout
	for _, m := range messages {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", m.ID, m.From, m.Body) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdMailRead is the CLI entry point for reading a message.
func cmdMailRead(args []string, stdout, stderr io.Writer) int {
	mp, code := openCityMailProvider(stderr, "gc mail read")
	if mp == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doMailRead(mp, rec, args, stdout, stderr)
}

// doMailRead displays a message and marks it as read. Accepts an injected
// provider and recorder for testability.
func doMailRead(mp mail.Provider, rec events.Recorder, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mail read: missing message ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	id := args[0]

	m, err := mp.Read(id)
	if err != nil {
		fmt.Fprintf(stderr, "gc mail read: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("ID:     %s", m.ID))
	w(fmt.Sprintf("From:   %s", m.From))
	w(fmt.Sprintf("To:     %s", m.To))
	w(fmt.Sprintf("Sent:   %s", m.CreatedAt.Format("2006-01-02 15:04:05")))
	w(fmt.Sprintf("Body:   %s", m.Body))

	rec.Record(events.Event{
		Type:    events.MailRead,
		Actor:   eventActor(),
		Subject: id,
	})
	return 0
}
