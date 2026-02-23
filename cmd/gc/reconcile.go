package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

// reconcileOps provides session-level operations needed by declarative
// reconciliation. Separate from session.Provider to avoid bloating the
// core 4-method interface with operations only reconciliation needs.
type reconcileOps interface {
	// listRunning returns all session names that match the given prefix.
	listRunning(prefix string) ([]string, error)

	// storeConfigHash persists a config hash in the session's environment
	// so future reconciliation can detect drift.
	storeConfigHash(name, hash string) error

	// configHash retrieves the previously stored config hash for a session.
	// Returns ("", nil) if no hash was stored (e.g., session predates hashing).
	configHash(name string) (string, error)
}

// tmuxReconcileOps implements reconcileOps using the tmux session environment.
type tmuxReconcileOps struct {
	tm *sessiontmux.Tmux
}

func (o *tmuxReconcileOps) listRunning(prefix string) ([]string, error) {
	all, err := o.tm.ListSessions()
	if err != nil {
		return nil, err
	}
	var matched []string
	for _, name := range all {
		if strings.HasPrefix(name, prefix) {
			matched = append(matched, name)
		}
	}
	return matched, nil
}

func (o *tmuxReconcileOps) storeConfigHash(name, hash string) error {
	return o.tm.SetEnvironment(name, "GC_CONFIG_HASH", hash)
}

func (o *tmuxReconcileOps) configHash(name string) (string, error) {
	val, err := o.tm.GetEnvironment(name, "GC_CONFIG_HASH")
	if err != nil {
		// No hash stored yet — not an error for reconciliation.
		return "", nil
	}
	return val, nil
}

// newReconcileOps creates a reconcileOps from a session.Provider.
// Returns nil if the provider doesn't support reconciliation ops
// (e.g., test fakes).
func newReconcileOps(sp session.Provider) reconcileOps {
	if tp, ok := sp.(*sessiontmux.Provider); ok {
		return &tmuxReconcileOps{tm: tp.Tmux()}
	}
	return nil
}

// doReconcileAgents performs declarative reconciliation: make reality match
// the desired agent list. It handles six rows:
//
//  1. Suspended + running → Stop (desired state = not running)
//  2. Suspended + not running → Skip (already in desired state)
//  3. Not running + in config → Start
//  4. Running + healthy (same hash) → Skip
//  5. Running + NOT in config → Stop (orphan cleanup)
//  6. Running + wrong config (hash differs) → Stop + Start
//
// The suspended map keys are session names of agents marked suspended in
// config. A nil map means no agents are suspended.
//
// If rops is nil, reconciliation degrades gracefully to the simpler
// start-if-not-running behavior (no drift detection, no orphan cleanup).
func doReconcileAgents(agents []agent.Agent, suspended map[string]bool,
	sp session.Provider, rops reconcileOps, rec events.Recorder, cityPrefix string,
	stdout, stderr io.Writer,
) int {
	// Build desired session name set for orphan detection.
	desired := make(map[string]bool, len(agents))

	// Phase 1: Start / drift detection for each desired agent.
	for _, a := range agents {
		if suspended[a.SessionName()] {
			// Desired state is "not running". Stop if running.
			if a.IsRunning() {
				if err := a.Stop(); err != nil {
					fmt.Fprintf(stderr, "gc start: stopping suspended %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
				} else {
					fmt.Fprintf(stdout, "Stopped suspended agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
				}
			}
			continue // Don't add to desired set — intentionally stopped.
		}

		desired[a.SessionName()] = true

		if !a.IsRunning() {
			// Row 1: not running → start.
			if err := a.Start(); err != nil {
				fmt.Fprintf(stderr, "gc start: starting %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
				continue
			}
			fmt.Fprintf(stdout, "Started agent '%s' (session: %s)\n", a.Name(), a.SessionName()) //nolint:errcheck // best-effort stdout
			rec.Record(events.Event{
				Type:    events.AgentStarted,
				Actor:   "gc",
				Subject: a.Name(),
				Message: a.SessionName(),
			})
			// Store config hash after successful start.
			if rops != nil {
				hash := session.ConfigFingerprint(a.SessionConfig())
				_ = rops.storeConfigHash(a.SessionName(), hash) // best-effort
			}
			continue
		}

		// Running — check for drift if reconcile ops available.
		if rops == nil {
			continue // Row 2: no reconcile ops, skip.
		}

		stored, err := rops.configHash(a.SessionName())
		if err != nil || stored == "" {
			// No stored hash — graceful upgrade, don't restart.
			continue
		}

		current := session.ConfigFingerprint(a.SessionConfig())
		if stored == current {
			continue // Row 2: hash matches, healthy.
		}

		// Row 5: drift detected → stop + start.
		fmt.Fprintf(stdout, "Config changed for '%s', restarting...\n", a.Name()) //nolint:errcheck // best-effort stdout
		if err := a.Stop(); err != nil {
			fmt.Fprintf(stderr, "gc start: stopping %s for restart: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
			continue
		}
		if err := a.Start(); err != nil {
			fmt.Fprintf(stderr, "gc start: restarting %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
			continue
		}
		fmt.Fprintf(stdout, "Restarted agent '%s' (session: %s)\n", a.Name(), a.SessionName()) //nolint:errcheck // best-effort stdout
		rec.Record(events.Event{
			Type:    events.AgentStarted,
			Actor:   "gc",
			Subject: a.Name(),
			Message: a.SessionName(),
		})
		hash := session.ConfigFingerprint(a.SessionConfig())
		_ = rops.storeConfigHash(a.SessionName(), hash) // best-effort
	}

	// Phase 2: Orphan cleanup — stop sessions with the city prefix that
	// are not in the desired set.
	if rops != nil {
		running, err := rops.listRunning(cityPrefix)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: listing sessions: %v\n", err) //nolint:errcheck // best-effort stderr
		} else {
			for _, name := range running {
				if desired[name] {
					continue
				}
				if err := sp.Stop(name); err != nil {
					fmt.Fprintf(stderr, "gc start: stopping orphan %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
				} else {
					fmt.Fprintf(stdout, "Stopped orphan session '%s'\n", name) //nolint:errcheck // best-effort stdout
				}
			}
		}
	}

	fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	return 0
}

// doStopOrphans stops sessions with the city prefix that are not in the
// desired set. Used by gc stop to clean up orphans after stopping config agents.
func doStopOrphans(sp session.Provider, rops reconcileOps, desired map[string]bool,
	cityPrefix string, stdout, stderr io.Writer,
) {
	if rops == nil {
		return
	}
	running, err := rops.listRunning(cityPrefix)
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: listing sessions: %v\n", err) //nolint:errcheck // best-effort stderr
		return
	}
	for _, name := range running {
		if desired[name] {
			continue
		}
		if err := sp.Stop(name); err != nil {
			fmt.Fprintf(stderr, "gc stop: stopping orphan %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
		} else {
			fmt.Fprintf(stdout, "Stopped orphan session '%s'\n", name) //nolint:errcheck // best-effort stdout
		}
	}
}
