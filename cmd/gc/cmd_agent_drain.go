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

// providerDrainOps implements drainOps using session.Provider metadata.
type providerDrainOps struct {
	sp session.Provider
}

func (o *providerDrainOps) setDrain(sessionName string) error {
	return o.sp.SetMeta(sessionName, "GC_DRAIN", strconv.FormatInt(time.Now().Unix(), 10))
}

func (o *providerDrainOps) clearDrain(sessionName string) error {
	_ = o.sp.RemoveMeta(sessionName, "GC_DRAIN_ACK")
	return o.sp.RemoveMeta(sessionName, "GC_DRAIN")
}

func (o *providerDrainOps) isDraining(sessionName string) (bool, error) {
	val, err := o.sp.GetMeta(sessionName, "GC_DRAIN")
	if err != nil {
		return false, nil // can't read = not draining
	}
	return val != "", nil
}

func (o *providerDrainOps) drainStartTime(sessionName string) (time.Time, error) {
	val, err := o.sp.GetMeta(sessionName, "GC_DRAIN")
	if err != nil {
		return time.Time{}, fmt.Errorf("reading GC_DRAIN: %w", err)
	}
	if val == "" {
		return time.Time{}, fmt.Errorf("GC_DRAIN not set")
	}
	unix, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing GC_DRAIN timestamp %q: %w", val, err)
	}
	return time.Unix(unix, 0), nil
}

func (o *providerDrainOps) setDrainAck(sessionName string) error {
	return o.sp.SetMeta(sessionName, "GC_DRAIN_ACK", "1")
}

func (o *providerDrainOps) isDrainAcked(sessionName string) (bool, error) {
	val, err := o.sp.GetMeta(sessionName, "GC_DRAIN_ACK")
	if err != nil {
		return false, nil
	}
	return val == "1", nil
}

// newDrainOps creates a drainOps from a session.Provider.
func newDrainOps(sp session.Provider) drainOps {
	return &providerDrainOps{sp: sp}
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

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent drain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent drain: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName = found.QualifiedName()

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
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

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent undrain: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent undrain: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName = found.QualifiedName()

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
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
		Use:   "drain-check [name]",
		Short: "Check if this agent is draining (exit 0 = draining)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentDrainCheck(args, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentDrainCheck(args []string, stderr io.Writer) int {
	var agentName, cityDir string
	if len(args) > 0 {
		// Explicit: resolve via city config (same as gc agent drain).
		agentName = args[0]
		var err error
		cityDir, err = resolveCity()
		if err != nil {
			return 1 // silent — same as current "not draining" behavior
		}
		cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityDir, "city.toml"))
		if err != nil {
			fmt.Fprintf(stderr, "gc agent drain-check: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
		if !ok {
			fmt.Fprintf(stderr, "gc agent drain-check: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
			return 1
		}
		agentName = found.QualifiedName()
		cityName := cfg.Workspace.Name
		if cityName == "" {
			cityName = filepath.Base(cityDir)
		}
		sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
		sp := newSessionProvider()
		dops := newDrainOps(sp)
		return doAgentDrainCheck(dops, sn)
	}

	// Implicit: env vars (backward compat for agent sessions).
	agentName = os.Getenv("GC_AGENT")
	cityDir = os.Getenv("GC_CITY")
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
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
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
		Use:   "drain-ack [name]",
		Short: "Acknowledge drain — signal the controller to stop this session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentDrainAck(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentDrainAck(args []string, stdout, stderr io.Writer) int {
	var agentName, cityDir string
	if len(args) > 0 {
		// Explicit: resolve via city config (same as gc agent drain).
		agentName = args[0]
		var err error
		cityDir, err = resolveCity()
		if err != nil {
			fmt.Fprintf(stderr, "gc agent drain-ack: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityDir, "city.toml"))
		if err != nil {
			fmt.Fprintf(stderr, "gc agent drain-ack: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
		if !ok {
			fmt.Fprintf(stderr, "gc agent drain-ack: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
			return 1
		}
		agentName = found.QualifiedName()
		cityName := cfg.Workspace.Name
		if cityName == "" {
			cityName = filepath.Base(cityDir)
		}
		sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
		sp := newSessionProvider()
		dops := newDrainOps(sp)
		return doAgentDrainAck(dops, sn, stdout, stderr)
	}

	// Implicit: env vars (backward compat for agent sessions).
	agentName = os.Getenv("GC_AGENT")
	cityDir = os.Getenv("GC_CITY")
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
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	dops := newDrainOps(sp)
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
