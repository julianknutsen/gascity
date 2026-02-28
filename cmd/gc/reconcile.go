package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/telemetry"
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

// providerReconcileOps implements reconcileOps using session.Provider metadata.
type providerReconcileOps struct {
	sp session.Provider
}

func (o *providerReconcileOps) listRunning(prefix string) ([]string, error) {
	return o.sp.ListRunning(prefix)
}

func (o *providerReconcileOps) storeConfigHash(name, hash string) error {
	return o.sp.SetMeta(name, "GC_CONFIG_HASH", hash)
}

func (o *providerReconcileOps) configHash(name string) (string, error) {
	val, err := o.sp.GetMeta(name, "GC_CONFIG_HASH")
	if err != nil {
		// No hash stored yet — not an error for reconciliation.
		return "", nil
	}
	return val, nil
}

// newReconcileOps creates a reconcileOps from a session.Provider.
func newReconcileOps(sp session.Provider) reconcileOps {
	return &providerReconcileOps{sp: sp}
}

// doReconcileAgents performs declarative reconciliation: make reality match
// the desired agent list. It handles four rows:
//
//  1. Not running + in config → Start
//  2. Running + healthy (same hash) → Skip
//  3. Running + NOT in config → Stop (orphan cleanup)
//  4. Running + wrong config (hash differs) → Stop + Start
//
// If rops is nil, reconciliation degrades gracefully to the simpler
// start-if-not-running behavior (no drift detection, no orphan cleanup).
func doReconcileAgents(agents []agent.Agent,
	sp session.Provider, rops reconcileOps, dops drainOps,
	ct crashTracker, it idleTracker,
	rec events.Recorder, cityPrefix string,
	poolSessions map[string]time.Duration,
	suspendedNames map[string]bool,
	stdout, stderr io.Writer,
) int {
	// Build desired session name set for orphan detection.
	desired := make(map[string]bool, len(agents))

	// Phase 1a (sequential): Triage each agent — collect those that need
	// starting, handle running agents inline (drift, restart, idle are fast
	// and touch more shared state).
	type startCandidate struct {
		agent  agent.Agent
		reason string
	}
	var toStart []startCandidate

	for _, a := range agents {
		desired[a.SessionName()] = true

		if !a.IsRunning() {
			// Row 1: not running → candidate for parallel start.

			// Zombie capture: session exists but agent process dead.
			// Grab pane output for crash forensics before Start() kills the zombie.
			zombieCaptured := false
			if sp.IsRunning(a.SessionName()) {
				zombieCaptured = true
				output, err := sp.Peek(a.SessionName(), 50)
				if err == nil && output != "" {
					rec.Record(events.Event{
						Type:    events.AgentCrashed,
						Actor:   "gc",
						Subject: a.Name(),
						Message: output,
					})
					telemetry.RecordAgentCrash(context.Background(), a.Name(), output)
				}
			}

			// Check crash loop quarantine.
			if ct != nil && ct.isQuarantined(a.SessionName(), time.Now()) {
				continue // skip silently — event was emitted when quarantine started
			}

			reason := "initial start"
			if zombieCaptured {
				reason = "crash recovery"
			} else if _, isPool := poolSessions[a.SessionName()]; isPool {
				reason = "pool scale-up"
			}
			toStart = append(toStart, startCandidate{agent: a, reason: reason})
			continue
		}

		// Running — clear drain if this desired agent was previously being drained
		// (handles scale-back-up: agent returns to desired set while draining).
		if dops != nil {
			if draining, _ := dops.isDraining(a.SessionName()); draining {
				_ = dops.clearDrain(a.SessionName())
			}
		}

		// Running — check if agent requested a restart (context exhaustion, etc.).
		if dops != nil {
			if restart, _ := dops.isRestartRequested(a.SessionName()); restart {
				fmt.Fprintf(stdout, "Agent '%s' requested restart, restarting...\n", a.Name()) //nolint:errcheck // best-effort stdout
				rec.Record(events.Event{
					Type:    events.AgentStopped,
					Actor:   "gc",
					Subject: a.Name(),
					Message: "restart requested by agent",
				})
				if err := a.Stop(); err != nil {
					fmt.Fprintf(stderr, "gc start: stopping %s for restart: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
					continue
				}
				if err := a.Start(); err != nil {
					fmt.Fprintf(stderr, "gc start: restarting %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
					continue
				}
				_ = sp.ClearScrollback(a.SessionName())                 // best-effort: clean slate after restart
				fmt.Fprintf(stdout, "Restarted agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
				rec.Record(events.Event{
					Type:    events.AgentStarted,
					Actor:   "gc",
					Subject: a.Name(),
				})
				if ct != nil {
					ct.recordStart(a.SessionName(), time.Now())
				}
				if rops != nil {
					hash := session.ConfigFingerprint(a.SessionConfig())
					_ = rops.storeConfigHash(a.SessionName(), hash)
				}
				continue
			}
		}

		// Running — check idle timeout (opt-in per agent).
		if it != nil && it.checkIdle(a.SessionName(), time.Now()) {
			fmt.Fprintf(stdout, "Agent '%s' idle too long, restarting...\n", a.Name()) //nolint:errcheck // best-effort stdout
			rec.Record(events.Event{
				Type:    events.AgentIdleKilled,
				Actor:   "gc",
				Subject: a.Name(),
			})
			telemetry.RecordAgentIdleKill(context.Background(), a.Name())
			if err := a.Stop(); err != nil {
				fmt.Fprintf(stderr, "gc start: stopping idle %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
				continue
			}
			if err := a.Start(); err != nil {
				fmt.Fprintf(stderr, "gc start: restarting idle %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
				continue
			}
			_ = sp.ClearScrollback(a.SessionName())                 // best-effort: clean slate after restart
			fmt.Fprintf(stdout, "Restarted agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
			rec.Record(events.Event{
				Type:    events.AgentStarted,
				Actor:   "gc",
				Subject: a.Name(),
			})
			// Record for crash tracking (idle kills count as restarts).
			if ct != nil {
				ct.recordStart(a.SessionName(), time.Now())
			}
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
		_ = sp.ClearScrollback(a.SessionName())                 // best-effort: clean slate after restart
		fmt.Fprintf(stdout, "Restarted agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
		rec.Record(events.Event{
			Type:    events.AgentStarted,
			Actor:   "gc",
			Subject: a.Name(),
		})
		hash := session.ConfigFingerprint(a.SessionConfig())
		_ = rops.storeConfigHash(a.SessionName(), hash) // best-effort
	}

	// Phase 1b (parallel): Start all pending agents concurrently.
	// Each goroutine writes to its own slot — no shared writes.
	type startResult struct {
		agent  agent.Agent
		reason string
		err    error
	}
	results := make([]startResult, len(toStart))
	var wg sync.WaitGroup
	for i, c := range toStart {
		wg.Add(1)
		go func(idx int, a agent.Agent, reason string) {
			defer wg.Done()
			results[idx] = startResult{agent: a, reason: reason, err: a.Start()}
		}(i, c.agent, c.reason)
	}
	wg.Wait()

	// Phase 1c (sequential): Process start results — logging, crash tracking,
	// event recording, config hash storage.
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(stderr, "gc start: starting %s: %v\n", r.agent.Name(), r.err) //nolint:errcheck // best-effort stderr
			continue
		}

		// Record the start for crash tracking.
		if ct != nil {
			ct.recordStart(r.agent.SessionName(), time.Now())
			// Check if this start just tripped the threshold.
			if ct.isQuarantined(r.agent.SessionName(), time.Now()) {
				rec.Record(events.Event{
					Type:    events.AgentQuarantined,
					Actor:   "gc",
					Subject: r.agent.Name(),
					Message: "crash loop detected",
				})
				telemetry.RecordAgentQuarantine(context.Background(), r.agent.Name())
				fmt.Fprintf(stderr, "gc start: agent '%s' quarantined (crash loop: restarted too many times within window)\n", r.agent.Name()) //nolint:errcheck // best-effort stderr
			}
		}

		fmt.Fprintf(stdout, "Started agent '%s' (%s)\n", r.agent.Name(), r.reason) //nolint:errcheck // best-effort stdout
		rec.Record(events.Event{
			Type:    events.AgentStarted,
			Actor:   "gc",
			Subject: r.agent.Name(),
		})
		telemetry.RecordAgentStart(context.Background(), r.agent.SessionName(), r.agent.Name(), nil)
		// Store config hash after successful start.
		if rops != nil {
			hash := session.ConfigFingerprint(r.agent.SessionConfig())
			_ = rops.storeConfigHash(r.agent.SessionName(), hash) // best-effort
		}
	}

	// Phase 2: Orphan cleanup — stop sessions with the city prefix that
	// are not in the desired set. Excess pool members are drained
	// gracefully (if drain ops available); true orphans are killed.
	if rops != nil {
		running, err := rops.listRunning(cityPrefix)
		if err != nil {
			fmt.Fprintf(stderr, "gc start: listing sessions: %v\n", err) //nolint:errcheck // best-effort stderr
		} else {
			for _, name := range running {
				if desired[name] {
					continue
				}
				// Excess pool member → drain gracefully.
				drainTimeout, isPoolSession := poolSessions[name]
				if dops != nil && isPoolSession {
					draining, _ := dops.isDraining(name)
					if !draining {
						_ = dops.setDrain(name)
						fmt.Fprintf(stdout, "Draining '%s' (pool scaling down)\n", name) //nolint:errcheck // best-effort stdout
						continue
					}
					// Already draining — check if agent acknowledged.
					acked, _ := dops.isDrainAcked(name)
					if acked {
						// Agent ack'd drain → stop the session.
						if err := sp.Stop(name); err != nil {
							fmt.Fprintf(stderr, "gc start: stopping drained %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
						} else {
							fmt.Fprintf(stdout, "Stopped drained session '%s'\n", name) //nolint:errcheck // best-effort stdout
						}
						continue
					}
					// Check drain timeout.
					if drainTimeout > 0 {
						started, err := dops.drainStartTime(name)
						if err == nil && time.Since(started) > drainTimeout {
							// Force-kill: drain timed out.
							if err := sp.Stop(name); err != nil {
								fmt.Fprintf(stderr, "gc start: stopping timed-out %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
							} else {
								fmt.Fprintf(stdout, "Killed drained session '%s' (timeout after %s)\n", name, drainTimeout) //nolint:errcheck // best-effort stdout
							}
							continue
						}
					}
					continue // still winding down
				}
				// Suspended agent → stop with distinct messaging.
				if suspendedNames[name] {
					if err := sp.Stop(name); err != nil {
						fmt.Fprintf(stderr, "gc start: stopping suspended %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
					} else {
						fmt.Fprintf(stdout, "Stopped suspended agent '%s'\n", name) //nolint:errcheck // best-effort stdout
						rec.Record(events.Event{
							Type:    events.AgentSuspended,
							Actor:   "gc",
							Subject: name,
						})
					}
					continue
				}
				// True orphan → kill.
				if err := sp.Stop(name); err != nil {
					fmt.Fprintf(stderr, "gc start: stopping orphan %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
				} else {
					fmt.Fprintf(stdout, "Stopped orphan session '%s'\n", name) //nolint:errcheck // best-effort stdout
					telemetry.RecordAgentStop(context.Background(), name, "orphan", nil)
				}
			}
		}
	}

	return 0
}

// doStopOrphans stops sessions with the city prefix that are not in the
// desired set. Used by gc stop to clean up orphans after stopping config agents.
// Uses gracefulStopAll for two-pass shutdown.
func doStopOrphans(sp session.Provider, rops reconcileOps, desired map[string]bool,
	cityPrefix string, timeout time.Duration, rec events.Recorder, stdout, stderr io.Writer,
) {
	if rops == nil {
		return
	}
	running, err := rops.listRunning(cityPrefix)
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: listing sessions: %v\n", err) //nolint:errcheck // best-effort stderr
		return
	}
	var orphans []string
	for _, name := range running {
		if desired[name] {
			continue
		}
		orphans = append(orphans, name)
	}
	gracefulStopAll(orphans, sp, timeout, rec, stdout, stderr)
}
