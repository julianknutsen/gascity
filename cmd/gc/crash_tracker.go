package main

import "time"

// crashTracker tracks agent restart history for crash loop detection.
// The controller holds one instance for its lifetime. State is in-memory
// only — intentionally lost on controller restart (counter reset, same as
// Erlang/OTP). Nil means no crash tracking (backward compatible).
type crashTracker interface {
	// recordStart notes that a session was (re)started at the given time.
	recordStart(sessionName string, at time.Time)

	// isQuarantined returns true if the session has exceeded max_restarts
	// within the restart window and the window hasn't expired yet.
	isQuarantined(sessionName string, now time.Time) bool

	// clearHistory removes all tracking for a session (used when an agent
	// is removed from config so orphan cleanup doesn't leave stale tracking).
	clearHistory(sessionName string)
}

// memoryCrashTracker is the production implementation of crashTracker.
type memoryCrashTracker struct {
	maxRestarts   int
	restartWindow time.Duration
	starts        map[string][]time.Time // session → recent start timestamps
}

// newCrashTracker creates a crash tracker with the given thresholds. Returns
// nil if maxRestarts <= 0 (disabled / unlimited restarts). Callers check for
// nil before using, same pattern as drainOps.
func newCrashTracker(maxRestarts int, window time.Duration) crashTracker {
	if maxRestarts <= 0 {
		return nil
	}
	return &memoryCrashTracker{
		maxRestarts:   maxRestarts,
		restartWindow: window,
		starts:        make(map[string][]time.Time),
	}
}

func (m *memoryCrashTracker) recordStart(sessionName string, at time.Time) {
	m.prune(sessionName, at)
	m.starts[sessionName] = append(m.starts[sessionName], at)
}

func (m *memoryCrashTracker) isQuarantined(sessionName string, now time.Time) bool {
	m.prune(sessionName, now)
	return len(m.starts[sessionName]) >= m.maxRestarts
}

func (m *memoryCrashTracker) clearHistory(sessionName string) {
	delete(m.starts, sessionName)
}

// prune removes entries older than the restart window to bound memory.
func (m *memoryCrashTracker) prune(sessionName string, now time.Time) {
	times := m.starts[sessionName]
	if len(times) == 0 {
		return
	}
	cutoff := now.Add(-m.restartWindow)
	i := 0
	for i < len(times) && times[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		m.starts[sessionName] = times[i:]
	}
	// Clean up empty slices.
	if len(m.starts[sessionName]) == 0 {
		delete(m.starts, sessionName)
	}
}
