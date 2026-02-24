package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

// drainOps abstracts drain signal operations for testability.
type drainOps interface {
	setDrain(sessionName string) error
	clearDrain(sessionName string) error
	isDraining(sessionName string) (bool, error)
	drainStartTime(sessionName string) (time.Time, error)
	setDrainAck(sessionName string) error
	isDrainAcked(sessionName string) (bool, error)
}

// tmuxDrainOps implements drainOps using tmux session environment.
type tmuxDrainOps struct {
	tm *sessiontmux.Tmux
}

func (o *tmuxDrainOps) setDrain(sessionName string) error {
	return o.tm.SetEnvironment(sessionName, "GC_DRAIN", strconv.FormatInt(time.Now().Unix(), 10))
}

func (o *tmuxDrainOps) clearDrain(sessionName string) error {
	return o.tm.RemoveEnvironment(sessionName, "GC_DRAIN")
}

func (o *tmuxDrainOps) isDraining(sessionName string) (bool, error) {
	val, err := o.tm.GetEnvironment(sessionName, "GC_DRAIN")
	if err != nil {
		return false, nil // no GC_DRAIN set = not draining
	}
	return val != "", nil
}

func (o *tmuxDrainOps) drainStartTime(sessionName string) (time.Time, error) {
	val, err := o.tm.GetEnvironment(sessionName, "GC_DRAIN")
	if err != nil {
		return time.Time{}, fmt.Errorf("reading GC_DRAIN: %w", err)
	}
	unix, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing GC_DRAIN timestamp %q: %w", val, err)
	}
	return time.Unix(unix, 0), nil
}

func (o *tmuxDrainOps) setDrainAck(sessionName string) error {
	return o.tm.SetEnvironment(sessionName, "GC_DRAIN_ACK", "1")
}

func (o *tmuxDrainOps) isDrainAcked(sessionName string) (bool, error) {
	val, err := o.tm.GetEnvironment(sessionName, "GC_DRAIN_ACK")
	if err != nil {
		return false, nil
	}
	return val == "1", nil
}

// newDrainOps creates a drainOps from a session.Provider.
// Returns nil if the provider doesn't support drain ops (e.g., test fakes).
func newDrainOps(sp session.Provider) drainOps {
	if tp, ok := sp.(*sessiontmux.Provider); ok {
		return &tmuxDrainOps{tm: tp.Tmux()}
	}
	return nil
}

// ---------------------------------------------------------------------------
// gc agent drain <name>
// ---------------------------------------------------------------------------

func newAgentDrainCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "drain <name>",
		Short: "Signal an agent to drain (wind down gracefully)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentDrain(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentDrain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent drain: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent drain: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	if dops == nil {
		fmt.Fprintln(stderr, "gc agent drain: drain requires tmux session provider") //nolint:errcheck // best-effort stderr
		return 1
	}
	rec := openCityRecorder(stderr)
	return doAgentDrain(dops, sp, rec, agentName, sn, stdout, stderr)
}

// doAgentDrain sets the drain signal on an agent's session.
func doAgentDrain(dops drainOps, sp session.Provider, rec events.Recorder,
	agentName, sn string, stdout, stderr io.Writer,
) int {
	if !sp.IsRunning(sn) {
		fmt.Fprintf(stderr, "gc agent drain: agent %q is not running\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := dops.setDrain(sn); err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.AgentDraining,
		Actor:   eventActor(),
		Subject: agentName,
	})
	fmt.Fprintf(stdout, "Draining agent '%s'\n", agentName) //nolint:errcheck // best-effort stdout
	return 0
}

// ---------------------------------------------------------------------------
// gc agent undrain <name>
// ---------------------------------------------------------------------------

func newAgentUndrainCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "undrain <name>",
		Short: "Cancel drain on an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentUndrain(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentUndrain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent undrain: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent undrain: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	if dops == nil {
		fmt.Fprintln(stderr, "gc agent undrain: drain requires tmux session provider") //nolint:errcheck // best-effort stderr
		return 1
	}
	rec := openCityRecorder(stderr)
	return doAgentUndrain(dops, sp, rec, agentName, sn, stdout, stderr)
}

// doAgentUndrain clears the drain signal on an agent's session.
func doAgentUndrain(dops drainOps, sp session.Provider, rec events.Recorder,
	agentName, sn string, stdout, stderr io.Writer,
) int {
	if !sp.IsRunning(sn) {
		fmt.Fprintf(stderr, "gc agent undrain: agent %q is not running\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := dops.clearDrain(sn); err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.AgentUndrained,
		Actor:   eventActor(),
		Subject: agentName,
	})
	fmt.Fprintf(stdout, "Undrained agent '%s'\n", agentName) //nolint:errcheck // best-effort stdout
	return 0
}

// ---------------------------------------------------------------------------
// gc agent drain-check
// ---------------------------------------------------------------------------

func newAgentDrainCheckCmd(stdout, stderr io.Writer) *cobra.Command {
	_ = stdout // drain-check is silent on stdout
	return &cobra.Command{
		Use:   "drain-check",
		Short: "Check if this agent is draining (exit 0 = draining)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentDrainCheck(stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentDrainCheck(stderr io.Writer) int {
	agentName := os.Getenv("GC_AGENT")
	cityDir := os.Getenv("GC_CITY")
	if agentName == "" || cityDir == "" {
		return 1 // not in agent context → not draining
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityDir, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain-check: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityDir)
	}
	sn := sessionName(cityName, agentName)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	if dops == nil {
		return 1 // no tmux → can't be draining
	}
	return doAgentDrainCheck(dops, sn)
}

// doAgentDrainCheck returns 0 if the agent is draining, 1 otherwise.
// Silent on stdout — designed for `if gc agent drain-check; then ...`.
func doAgentDrainCheck(dops drainOps, sn string) int {
	draining, err := dops.isDraining(sn)
	if err != nil || !draining {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// gc agent drain-ack
// ---------------------------------------------------------------------------

func newAgentDrainAckCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "drain-ack",
		Short: "Acknowledge drain — signal the controller to stop this session",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentDrainAck(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentDrainAck(stdout, stderr io.Writer) int {
	agentName := os.Getenv("GC_AGENT")
	cityDir := os.Getenv("GC_CITY")
	if agentName == "" || cityDir == "" {
		fmt.Fprintln(stderr, "gc agent drain-ack: not in agent context (GC_AGENT/GC_CITY not set)") //nolint:errcheck // best-effort stderr
		return 1
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityDir, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain-ack: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityDir)
	}
	sn := sessionName(cityName, agentName)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
	if dops == nil {
		fmt.Fprintln(stderr, "gc agent drain-ack: requires tmux session provider") //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentDrainAck(dops, sn, stdout, stderr)
}

// doAgentDrainAck sets the drain-ack flag on the session. The controller
// will stop the session on the next tick.
func doAgentDrainAck(dops drainOps, sn string, stdout, stderr io.Writer) int {
	if err := dops.setDrainAck(sn); err != nil {
		fmt.Fprintf(stderr, "gc agent drain-ack: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintln(stdout, "Drain acknowledged. Controller will stop this session.") //nolint:errcheck // best-effort stdout
	return 0
}
