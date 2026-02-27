package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newHandoffCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "handoff <subject> [message]",
		Short: "Send handoff mail to self and request restart",
		Long: `Convenience command for context handoff. Equivalent to:

  gc mail send $GC_AGENT -s "HANDOFF: ..." -m "..."
  gc agent request-restart

Sends a message bead to the current agent (self-addressed), sets the
restart-requested flag, then blocks until the controller kills and
restarts the session.

Must be run from within an agent session (GC_AGENT and GC_CITY env vars).`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdHandoff(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdHandoff(args []string, stdout, stderr io.Writer) int {
	agentName := os.Getenv("GC_AGENT")
	cityDir := os.Getenv("GC_CITY")
	if agentName == "" || cityDir == "" {
		fmt.Fprintln(stderr, "gc handoff: not in agent context (GC_AGENT/GC_CITY not set)") //nolint:errcheck // best-effort stderr
		return 1
	}

	store, code := openCityStore(stderr, "gc handoff")
	if store == nil {
		return code
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityDir, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc handoff: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityDir)
	}
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	rec := openCityRecorder(stderr)

	code = doHandoff(store, rec, dops, agentName, sn, args, stdout, stderr)
	if code != 0 {
		return code
	}

	// Block forever. The controller will kill the entire process tree.
	select {}
}

// doHandoff sends a handoff mail to self and sets the restart-requested flag.
// Testable: does not block.
func doHandoff(store beads.Store, rec events.Recorder, dops drainOps,
	agentName, sn string, args []string, stdout, stderr io.Writer,
) int {
	subject := args[0]
	var message string
	if len(args) > 1 {
		message = args[1]
	}

	b, err := store.Create(beads.Bead{
		Title:       subject,
		Description: message,
		Type:        "message",
		Assignee:    agentName,
		From:        agentName,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gc handoff: creating mail: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.MailSent,
		Actor:   agentName,
		Subject: b.ID,
		Message: agentName,
	})

	if err := dops.setRestartRequested(sn); err != nil {
		fmt.Fprintf(stderr, "gc handoff: setting restart flag: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.AgentDraining,
		Actor:   agentName,
		Subject: agentName,
		Message: "handoff",
	})

	fmt.Fprintf(stdout, "Handoff: sent mail %s, requesting restart...\n", b.ID) //nolint:errcheck // best-effort stdout
	return 0
}
