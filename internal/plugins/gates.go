package plugins

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/events"
)

// GateResult holds the outcome of a gate check.
type GateResult struct {
	// Due is true if the gate condition is satisfied and the plugin should run.
	Due bool
	// Reason explains why the gate is or isn't due.
	Reason string
	// LastRun is the last execution time (zero if never run).
	LastRun time.Time
}

// LastRunFunc returns the last run time for a named plugin.
// Returns zero time and nil error if never run.
type LastRunFunc func(name string) (time.Time, error)

// CursorFunc returns the event cursor (highest seq) for a named plugin.
// Returns 0 if no cursor exists.
type CursorFunc func(pluginName string) uint64

// CheckGate evaluates a plugin's gate condition and returns whether it's due.
// cityDir is the city root directory, used by event gates to find .gc/events.jsonl.
// cursorFn returns the last-processed event seq for event gates; may be nil for
// non-event gates.
func CheckGate(p Plugin, now time.Time, lastRunFn LastRunFunc, cityDir string, cursorFn CursorFunc) GateResult {
	switch p.Gate {
	case "cooldown":
		return checkCooldown(p, now, lastRunFn)
	case "cron":
		return checkCron(p, now, lastRunFn)
	case "condition":
		return checkCondition(p)
	case "event":
		return checkEvent(p, cityDir, cursorFn)
	case "manual":
		return GateResult{Due: false, Reason: "manual gate — use gc plugin run"}
	default:
		return GateResult{Due: false, Reason: fmt.Sprintf("unknown gate %q", p.Gate)}
	}
}

// checkCooldown checks if enough time has elapsed since the last run.
func checkCooldown(p Plugin, now time.Time, lastRunFn LastRunFunc) GateResult {
	interval, err := time.ParseDuration(p.Interval)
	if err != nil {
		return GateResult{Due: false, Reason: fmt.Sprintf("bad interval: %v", err)}
	}

	last, err := lastRunFn(p.Name)
	if err != nil {
		return GateResult{Due: false, Reason: fmt.Sprintf("error querying last run: %v", err)}
	}

	if last.IsZero() {
		return GateResult{Due: true, Reason: "never run", LastRun: last}
	}

	elapsed := now.Sub(last)
	if elapsed >= interval {
		return GateResult{
			Due:     true,
			Reason:  fmt.Sprintf("elapsed %s >= interval %s", elapsed.Round(time.Second), interval),
			LastRun: last,
		}
	}

	remaining := interval - elapsed
	return GateResult{
		Due:     false,
		Reason:  fmt.Sprintf("cooldown: %s remaining", remaining.Round(time.Second)),
		LastRun: last,
	}
}

// checkCron uses simple minute-granularity matching against the schedule.
// Schedule format: "minute hour day-of-month month day-of-week" (5 fields).
func checkCron(p Plugin, now time.Time, lastRunFn LastRunFunc) GateResult {
	fields := strings.Fields(p.Schedule)
	if len(fields) != 5 {
		return GateResult{Due: false, Reason: fmt.Sprintf("bad cron schedule: want 5 fields, got %d", len(fields))}
	}

	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	if !cronFieldMatches(minute, now.Minute()) ||
		!cronFieldMatches(hour, now.Hour()) ||
		!cronFieldMatches(dom, now.Day()) ||
		!cronFieldMatches(month, int(now.Month())) ||
		!cronFieldMatches(dow, int(now.Weekday())) {
		return GateResult{Due: false, Reason: "cron: schedule not matched"}
	}

	// Schedule matches — check if already run this minute.
	last, err := lastRunFn(p.Name)
	if err != nil {
		return GateResult{Due: false, Reason: fmt.Sprintf("error querying last run: %v", err)}
	}
	if !last.IsZero() && last.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
		return GateResult{Due: false, Reason: "cron: already run this minute", LastRun: last}
	}

	return GateResult{Due: true, Reason: "cron: schedule matched", LastRun: last}
}

// cronFieldMatches checks if a single cron field matches a value.
// Supports: "*" (any), exact integer, or comma-separated values.
func cronFieldMatches(field string, value int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n == value {
			return true
		}
	}
	return false
}

// checkCondition runs the check command and returns due if exit code is 0.
func checkCondition(p Plugin) GateResult {
	cmd := exec.Command("sh", "-c", p.Check)
	if err := cmd.Run(); err != nil {
		return GateResult{Due: false, Reason: fmt.Sprintf("check command failed: %v", err)}
	}
	return GateResult{Due: true, Reason: "condition: check passed (exit 0)"}
}

// checkEvent checks if matching events exist after the last cursor position.
func checkEvent(p Plugin, cityDir string, cursorFn CursorFunc) GateResult {
	var cursor uint64
	if cursorFn != nil {
		cursor = cursorFn(p.Name)
	}

	eventsPath := filepath.Join(cityDir, ".gc", "events.jsonl")
	matched, err := events.ReadFiltered(eventsPath, events.Filter{
		Type:     p.On,
		AfterSeq: cursor,
	})
	if err != nil {
		return GateResult{Due: false, Reason: fmt.Sprintf("event: read error: %v", err)}
	}
	if len(matched) == 0 {
		return GateResult{Due: false, Reason: "event: no matching events"}
	}
	return GateResult{Due: true, Reason: fmt.Sprintf("event: %d %s event(s)", len(matched), p.On)}
}

// MaxSeqFromLabels extracts the highest seq:<N> value from bead labels.
// Used by CLI callers to compute the event cursor from BdStore results.
func MaxSeqFromLabels(labelSets [][]string) uint64 {
	var maxSeq uint64
	for _, labels := range labelSets {
		for _, l := range labels {
			if strings.HasPrefix(l, "seq:") {
				if n, err := strconv.ParseUint(l[4:], 10, 64); err == nil && n > maxSeq {
					maxSeq = n
				}
			}
		}
	}
	return maxSeq
}
