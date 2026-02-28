package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/plugins"
)

// pluginDispatcher evaluates plugin gate conditions and dispatches due
// plugins as wisps. Follows the nil-guard tracker pattern: nil means
// no auto-dispatchable plugins exist.
type pluginDispatcher interface {
	dispatch(cityPath string, now time.Time) (int, error)
}

// memoryPluginDispatcher is the production implementation.
type memoryPluginDispatcher struct {
	pp     []plugins.Plugin
	store  beads.Store
	ep     events.Provider
	runner beads.CommandRunner
}

// buildPluginDispatcher scans formula layers for plugins and returns a
// dispatcher. Returns nil if no auto-dispatchable plugins are found.
// Scans both city-level and per-rig plugins. Rig plugins get their Rig
// field stamped so they use independent scoped labels.
func buildPluginDispatcher(cityPath string, cfg *config.City, runner beads.CommandRunner, rec events.Recorder, stderr io.Writer) pluginDispatcher {
	// Scan city-level plugins.
	cityLayers := cityFormulaLayers(cityPath, cfg)
	cityPP, err := plugins.Scan(fsys.OSFS{}, cityLayers, cfg.Plugins.Skip)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: plugin scan (city): %v\n", err) //nolint:errcheck // best-effort stderr
		return nil
	}

	// Scan per-rig plugins from rig-exclusive layers (skip city prefix).
	var rigPP []plugins.Plugin
	for rigName, layers := range cfg.FormulaLayers.Rigs {
		exclusive := rigExclusiveLayers(layers, cityLayers)
		if len(exclusive) == 0 {
			continue
		}
		rp, err := plugins.Scan(fsys.OSFS{}, exclusive, cfg.Plugins.Skip)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: plugin scan (rig %s): %v\n", rigName, err) //nolint:errcheck // best-effort stderr
			continue
		}
		for i := range rp {
			rp[i].Rig = rigName
		}
		rigPP = append(rigPP, rp...)
	}

	allPP := make([]plugins.Plugin, 0, len(cityPP)+len(rigPP))
	allPP = append(allPP, cityPP...)
	allPP = append(allPP, rigPP...)

	// Filter out manual-gate plugins — they are never auto-dispatched.
	var auto []plugins.Plugin
	for _, p := range allPP {
		if p.Gate != "manual" {
			auto = append(auto, p)
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

	return &memoryPluginDispatcher{
		pp:     auto,
		store:  store,
		ep:     ep,
		runner: runner,
	}
}

func (m *memoryPluginDispatcher) dispatch(cityPath string, now time.Time) (int, error) {
	lastRunFn := pluginLastRunFn(m.store)
	cursorFn := bdCursorFunc(m.store)

	dispatched := 0
	for _, p := range m.pp {
		result := plugins.CheckGate(p, now, lastRunFn, m.ep, cursorFn)
		if !result.Due {
			continue
		}

		// Capture event head before wisp creation for event gates.
		var headSeq uint64
		if p.Gate == "event" && m.ep != nil {
			headSeq, _ = m.ep.LatestSeq()
		}

		rootID, err := instantiateWisp(p.Formula, "", nil, m.store)
		if err != nil {
			continue // best-effort: skip failed cook, don't crash
		}

		// Label with plugin-run:<scopedName> for tracking.
		scoped := p.ScopedName()
		args := []string{"update", rootID, "--label=plugin-run:" + scoped}
		if p.Gate == "event" && m.ep != nil {
			args = append(args, fmt.Sprintf("--label=plugin:%s", scoped))
			args = append(args, fmt.Sprintf("--label=seq:%d", headSeq))
		}
		if p.Pool != "" {
			pool := qualifyPool(p.Pool, p.Rig)
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
// plugins.
func rigExclusiveLayers(rigLayers, cityLayers []string) []string {
	if len(rigLayers) <= len(cityLayers) {
		return nil
	}
	return rigLayers[len(cityLayers):]
}

// qualifyPool prefixes an unqualified pool name with the rig name for
// rig-scoped plugins. Already-qualified names (containing "/") are
// returned as-is. City plugins (empty rig) are unchanged.
func qualifyPool(pool, rig string) string {
	if rig == "" || strings.Contains(pool, "/") {
		return pool
	}
	return rig + "/" + pool
}
