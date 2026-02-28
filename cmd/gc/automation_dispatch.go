package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/automations"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

// automationDispatcher evaluates automation gate conditions and dispatches due
// automations as wisps. Follows the nil-guard tracker pattern: nil means
// no auto-dispatchable automations exist.
type automationDispatcher interface {
	dispatch(cityPath string, now time.Time) (int, error)
}

// memoryAutomationDispatcher is the production implementation.
type memoryAutomationDispatcher struct {
	aa     []automations.Automation
	store  beads.Store
	ep     events.Provider
	runner beads.CommandRunner
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
		aa:     auto,
		store:  store,
		ep:     ep,
		runner: runner,
	}
}

func (m *memoryAutomationDispatcher) dispatch(cityPath string, now time.Time) (int, error) {
	lastRunFn := automationLastRunFn(m.store)
	cursorFn := bdCursorFunc(m.store)

	dispatched := 0
	for _, a := range m.aa {
		result := automations.CheckGate(a, now, lastRunFn, m.ep, cursorFn)
		if !result.Due {
			continue
		}

		// Capture event head before wisp creation for event gates.
		var headSeq uint64
		if a.Gate == "event" && m.ep != nil {
			headSeq, _ = m.ep.LatestSeq()
		}

		rootID, err := instantiateWisp(a.Formula, "", nil, m.store)
		if err != nil {
			continue // best-effort: skip failed cook, don't crash
		}

		// Label with automation-run:<scopedName> for tracking.
		scoped := a.ScopedName()
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
			continue // best-effort: skip label failure
		}

		dispatched++
	}
	return dispatched, nil
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
