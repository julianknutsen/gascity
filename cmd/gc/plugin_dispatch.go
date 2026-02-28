package main

import (
	"fmt"
	"io"
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
func buildPluginDispatcher(cityPath string, cfg *config.City, runner beads.CommandRunner, rec events.Recorder, stderr io.Writer) pluginDispatcher {
	layers := pluginFormulaLayers(cityPath, cfg)
	pp, err := plugins.Scan(fsys.OSFS{}, layers, cfg.Plugins.Skip)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: plugin scan: %v\n", err) //nolint:errcheck // best-effort stderr
		return nil
	}

	// Filter out manual-gate plugins — they are never auto-dispatched.
	var auto []plugins.Plugin
	for _, p := range pp {
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

		// Label with plugin-run:<name> for tracking.
		args := []string{"update", rootID, "--label=plugin-run:" + p.Name}
		if p.Gate == "event" && m.ep != nil {
			args = append(args, fmt.Sprintf("--label=plugin:%s", p.Name))
			args = append(args, fmt.Sprintf("--label=seq:%d", headSeq))
		}
		if p.Pool != "" {
			args = append(args, fmt.Sprintf("--label=pool:%s", p.Pool))
		}
		if _, err := m.runner(cityPath, "bd", args...); err != nil {
			continue // best-effort: skip label failure
		}

		dispatched++
	}
	return dispatched, nil
}
