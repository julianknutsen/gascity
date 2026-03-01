package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/automations"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

// automationDispatcher evaluates automation gate conditions and dispatches due
// automations as wisps or exec scripts. Follows the nil-guard tracker pattern:
// nil means no auto-dispatchable automations exist.
//
// dispatch is fire-and-forget: gate evaluation is synchronous, but each due
// automation's dispatch action runs in its own goroutine. The tracking bead
// is created before the goroutine launches to prevent re-fire on the next tick.
type automationDispatcher interface {
	dispatch(ctx context.Context, cityPath string, now time.Time)
}

// ExecRunner runs a shell command with context, working directory, and
// environment variables. Returns combined stdout or an error.
type ExecRunner func(ctx context.Context, command, dir string, env []string) ([]byte, error)

// shellExecRunner is the production ExecRunner using os/exec.
func shellExecRunner(ctx context.Context, command, dir string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), env...)
	return cmd.CombinedOutput()
}

// memoryAutomationDispatcher is the production implementation.
type memoryAutomationDispatcher struct {
	aa         []automations.Automation
	store      beads.Store
	ep         events.Provider
	runner     beads.CommandRunner
	execRun    ExecRunner
	rec        events.Recorder
	stderr     io.Writer
	maxTimeout time.Duration
}

// buildAutomationDispatcher scans formula layers for automations and returns a
// dispatcher. Returns nil if no auto-dispatchable automations are found.
// Scans both city-level and per-rig automations. Rig automations get their Rig
// field stamped so they use independent scoped labels.
func buildAutomationDispatcher(cityPath string, cfg *config.City, runner beads.CommandRunner, rec events.Recorder, stderr io.Writer) automationDispatcher {
	// Scan city-level automations.
	cityLayers := cityFormulaLayers(cityPath, cfg)
	cityAA, err := automations.Scan(fsys.OSFS{}, cityLayers, cfg.Automations.Skip)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: automation scan (city): %v\n", err) //nolint:errcheck // best-effort stderr
		return nil
	}

	// Scan per-rig automations from rig-exclusive layers (skip city prefix).
	var rigAA []automations.Automation
	for rigName, layers := range cfg.FormulaLayers.Rigs {
		exclusive := rigExclusiveLayers(layers, cityLayers)
		if len(exclusive) == 0 {
			continue
		}
		ra, err := automations.Scan(fsys.OSFS{}, exclusive, cfg.Automations.Skip)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: automation scan (rig %s): %v\n", rigName, err) //nolint:errcheck // best-effort stderr
			continue
		}
		for i := range ra {
			ra[i].Rig = rigName
		}
		rigAA = append(rigAA, ra...)
	}

	allAA := make([]automations.Automation, 0, len(cityAA)+len(rigAA))
	allAA = append(allAA, cityAA...)
	allAA = append(allAA, rigAA...)

	// Apply automation overrides from city config.
	if len(cfg.Automations.Overrides) > 0 {
		if err := automations.ApplyOverrides(allAA, convertOverrides(cfg.Automations.Overrides)); err != nil {
			fmt.Fprintf(stderr, "gc start: automation overrides: %v\n", err) //nolint:errcheck // best-effort stderr
		}
	}

	// Filter out manual-gate automations — they are never auto-dispatched.
	var auto []automations.Automation
	for _, a := range allAA {
		if a.Gate != "manual" {
			auto = append(auto, a)
		}
	}
	if len(auto) == 0 {
		return nil
	}

	store := beads.NewBdStore(cityPath, runner)

	// Extract events.Provider from recorder if available.
	// FileRecorder implements Provider; Discard does not.
	var ep events.Provider
	if p, ok := rec.(events.Provider); ok {
		ep = p
	}

	return &memoryAutomationDispatcher{
		aa:         auto,
		store:      store,
		ep:         ep,
		runner:     runner,
		execRun:    shellExecRunner,
		rec:        rec,
		stderr:     stderr,
		maxTimeout: cfg.Automations.MaxTimeoutDuration(),
	}
}

func (m *memoryAutomationDispatcher) dispatch(ctx context.Context, cityPath string, now time.Time) {
	lastRunFn := automationLastRunFn(m.store)
	cursorFn := bdCursorFunc(m.store)

	for _, a := range m.aa {
		result := automations.CheckGate(a, now, lastRunFn, m.ep, cursorFn)
		if !result.Due {
			continue
		}

		// Create tracking bead synchronously BEFORE dispatch goroutine.
		// This prevents the cooldown gate from re-firing on the next tick.
		scoped := a.ScopedName()
		trackingBead, err := m.store.Create(beads.Bead{
			Title:  "automation:" + scoped,
			Labels: []string{"automation-run:" + scoped},
		})
		if err != nil {
			fmt.Fprintf(m.stderr, "gc: automation dispatch: creating tracking bead for %s: %v\n", scoped, err) //nolint:errcheck
			continue
		}

		// Fire and forget with timeout.
		a := a // capture loop variable
		go m.dispatchOne(ctx, a, cityPath, trackingBead.ID)
	}
}

// dispatchOne runs a single automation dispatch in its own goroutine.
// For exec automations, runs the script directly. For formula automations,
// instantiates a wisp. Emits events and updates the tracking bead.
func (m *memoryAutomationDispatcher) dispatchOne(ctx context.Context, a automations.Automation, cityPath, trackingID string) {
	timeout := effectiveTimeout(a, m.maxTimeout)
	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scoped := a.ScopedName()
	m.rec.Record(events.Event{
		Type:    events.AutomationFired,
		Actor:   "controller",
		Subject: scoped,
	})

	if a.IsExec() {
		m.dispatchExec(childCtx, a, cityPath, trackingID)
	} else {
		m.dispatchWisp(childCtx, a, cityPath, trackingID)
	}
}

// dispatchExec runs an exec automation's shell command.
func (m *memoryAutomationDispatcher) dispatchExec(ctx context.Context, a automations.Automation, cityPath, trackingID string) {
	scoped := a.ScopedName()

	// Build env with AUTOMATION_DIR.
	var env []string
	if a.Source != "" {
		env = append(env, "AUTOMATION_DIR="+filepath.Dir(a.Source))
	}

	output, err := m.execRun(ctx, a.Exec, cityPath, env)

	// Update tracking bead with outcome labels.
	labels := []string{"exec"}
	if err != nil {
		labels = append(labels, "exec-failed")
		fmt.Fprintf(m.stderr, "gc: automation exec %s failed: %v\n", scoped, err) //nolint:errcheck
		if len(output) > 0 {
			fmt.Fprintf(m.stderr, "gc: automation exec %s output: %s\n", scoped, output) //nolint:errcheck
		}
		m.rec.Record(events.Event{
			Type:    events.AutomationFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
	} else {
		m.rec.Record(events.Event{
			Type:    events.AutomationCompleted,
			Actor:   "controller",
			Subject: scoped,
		})
	}

	// Label tracking bead with outcome via store (not CLI).
	m.store.Update(trackingID, beads.UpdateOpts{Labels: labels}) //nolint:errcheck // best-effort
}

// dispatchWisp instantiates a wisp from the automation's formula.
func (m *memoryAutomationDispatcher) dispatchWisp(_ context.Context, a automations.Automation, cityPath, _ string) {
	scoped := a.ScopedName()

	// Capture event head before wisp creation for event gates.
	var headSeq uint64
	if a.Gate == "event" && m.ep != nil {
		headSeq, _ = m.ep.LatestSeq()
	}

	rootID, err := m.store.MolCook(a.Formula, "", nil)
	if err != nil {
		m.rec.Record(events.Event{
			Type:    events.AutomationFailed,
			Actor:   "controller",
			Subject: scoped,
			Message: err.Error(),
		})
		return // best-effort: skip failed cook, don't crash
	}

	// Label wisp with automation-run:<scopedName> for tracking.
	args := []string{"update", rootID, "--label=automation-run:" + scoped}
	if a.Gate == "event" && m.ep != nil {
		args = append(args, fmt.Sprintf("--label=automation:%s", scoped))
		args = append(args, fmt.Sprintf("--label=seq:%d", headSeq))
	}
	if a.Pool != "" {
		pool := qualifyPool(a.Pool, a.Rig)
		args = append(args, fmt.Sprintf("--label=pool:%s", pool))
	}
	if _, err := m.runner(cityPath, "bd", args...); err != nil {
		return // best-effort: skip label failure
	}

	m.rec.Record(events.Event{
		Type:    events.AutomationCompleted,
		Actor:   "controller",
		Subject: scoped,
	})
}

// effectiveTimeout returns the timeout to use for an automation dispatch.
// Uses the automation's configured timeout (or default), capped by maxTimeout.
func effectiveTimeout(a automations.Automation, maxTimeout time.Duration) time.Duration {
	t := a.TimeoutOrDefault()
	if maxTimeout > 0 && t > maxTimeout {
		return maxTimeout
	}
	return t
}

// rigExclusiveLayers returns the suffix of rigLayers that is not in
// cityLayers. Since rig layers are built as [cityLayers..., rigTopoLayers...,
// rigLocalLayer], we strip the city prefix to avoid double-scanning city
// automations.
func rigExclusiveLayers(rigLayers, cityLayers []string) []string {
	if len(rigLayers) <= len(cityLayers) {
		return nil
	}
	return rigLayers[len(cityLayers):]
}

// qualifyPool prefixes an unqualified pool name with the rig name for
// rig-scoped automations. Already-qualified names (containing "/") are
// returned as-is. City automations (empty rig) are unchanged.
func qualifyPool(pool, rig string) string {
	if rig == "" || strings.Contains(pool, "/") {
		return pool
	}
	return rig + "/" + pool
}

// convertOverrides converts config.AutomationOverride to automations.Override.
func convertOverrides(cfgOvs []config.AutomationOverride) []automations.Override {
	out := make([]automations.Override, len(cfgOvs))
	for i, c := range cfgOvs {
		out[i] = automations.Override{
			Name:     c.Name,
			Rig:      c.Rig,
			Enabled:  c.Enabled,
			Gate:     c.Gate,
			Interval: c.Interval,
			Schedule: c.Schedule,
			Check:    c.Check,
			On:       c.On,
			Pool:     c.Pool,
			Timeout:  c.Timeout,
		}
	}
	return out
}
