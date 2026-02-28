package automations

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/events"
)

func neverRan(_ string) (time.Time, error) { return time.Time{}, nil }

func TestCheckGateCooldownNeverRun(t *testing.T) {
	a := Automation{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if !result.Due {
		t.Errorf("Due = false, want true (never run)")
	}
	if result.Reason != "never run" {
		t.Errorf("Reason = %q, want %q", result.Reason, "never run")
	}
}

func TestCheckGateCooldownDue(t *testing.T) {
	a := Automation{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	lastRun := now.Add(-25 * time.Hour) // 25h ago — past the 24h interval
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(a, now, lastRunFn, nil, nil)
	if !result.Due {
		t.Errorf("Due = false, want true (25h > 24h)")
	}
}

func TestCheckGateCooldownNotDue(t *testing.T) {
	a := Automation{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	lastRun := now.Add(-12 * time.Hour) // 12h ago — within 24h interval
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(a, now, lastRunFn, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (12h < 24h)")
	}
}

func TestCheckGateManual(t *testing.T) {
	a := Automation{Name: "deploy", Gate: "manual"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (manual never auto-fires)")
	}
}

func TestCheckGateCronMatched(t *testing.T) {
	a := Automation{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	// 03:00 UTC — should match.
	now := time.Date(2026, 2, 27, 3, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if !result.Due {
		t.Errorf("Due = false, want true (schedule matches 03:00)")
	}
}

func TestCheckGateCronNotMatched(t *testing.T) {
	a := Automation{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	// 12:00 UTC — should not match.
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (schedule doesn't match 12:00)")
	}
}

func TestCheckGateCronAlreadyRunThisMinute(t *testing.T) {
	a := Automation{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	now := time.Date(2026, 2, 27, 3, 0, 30, 0, time.UTC)
	lastRun := time.Date(2026, 2, 27, 3, 0, 10, 0, time.UTC) // same minute
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(a, now, lastRunFn, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (already run this minute)")
	}
}

func TestCheckGateCondition(t *testing.T) {
	a := Automation{Name: "check", Gate: "condition", Check: "true"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if !result.Due {
		t.Errorf("Due = false, want true (exit 0)")
	}
}

func TestCheckGateConditionFails(t *testing.T) {
	a := Automation{Name: "check", Gate: "condition", Check: "false"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(a, now, neverRan, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (exit non-zero)")
	}
}

func TestCronFieldMatches(t *testing.T) {
	tests := []struct {
		field string
		value int
		want  bool
	}{
		{"*", 5, true},
		{"5", 5, true},
		{"5", 3, false},
		{"1,3,5", 3, true},
		{"1,3,5", 2, false},
	}
	for _, tt := range tests {
		got := cronFieldMatches(tt.field, tt.value)
		if got != tt.want {
			t.Errorf("cronFieldMatches(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
		}
	}
}

// newEventsProvider creates a FileRecorder-backed Provider with events for tests.
func newEventsProvider(t *testing.T, evts []events.Event) events.Provider {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderr bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evts {
		rec.Record(e)
	}
	t.Cleanup(func() { rec.Close() }) //nolint:errcheck // test cleanup
	return rec
}

func TestCheckGateEventDue(t *testing.T) {
	ep := newEventsProvider(t, []events.Event{
		{Type: "bead.closed"},
		{Type: "bead.created"},
		{Type: "bead.closed"},
	})
	a := Automation{Name: "convoy-check", Gate: "event", On: "bead.closed"}
	// nil cursorFn → cursor=0 → all events considered.
	result := CheckGate(a, time.Time{}, neverRan, ep, nil)
	if !result.Due {
		t.Errorf("Due = false, want true; reason: %s", result.Reason)
	}
	if result.Reason != "event: 2 bead.closed event(s)" {
		t.Errorf("Reason = %q, want %q", result.Reason, "event: 2 bead.closed event(s)")
	}
}

func TestCheckGateEventWithCursor(t *testing.T) {
	ep := newEventsProvider(t, []events.Event{
		{Type: "bead.closed"},
		{Type: "bead.created"},
		{Type: "bead.closed"},
	})
	a := Automation{Name: "convoy-check", Gate: "event", On: "bead.closed"}
	// Cursor at seq 2 → only seq 3 matches.
	cursorFn := func(_ string) uint64 { return 2 }
	result := CheckGate(a, time.Time{}, neverRan, ep, cursorFn)
	if !result.Due {
		t.Errorf("Due = false, want true; reason: %s", result.Reason)
	}
	if result.Reason != "event: 1 bead.closed event(s)" {
		t.Errorf("Reason = %q, want %q", result.Reason, "event: 1 bead.closed event(s)")
	}
}

func TestCheckGateEventCursorPastAll(t *testing.T) {
	ep := newEventsProvider(t, []events.Event{
		{Type: "bead.closed"},
		{Type: "bead.closed"},
	})
	a := Automation{Name: "convoy-check", Gate: "event", On: "bead.closed"}
	// Cursor past all events → not due.
	cursorFn := func(_ string) uint64 { return 5 }
	result := CheckGate(a, time.Time{}, neverRan, ep, cursorFn)
	if result.Due {
		t.Errorf("Due = true, want false (cursor past all events)")
	}
}

func TestCheckGateEventNotDue(t *testing.T) {
	ep := newEventsProvider(t, []events.Event{
		{Type: "bead.created"},
		{Type: "bead.updated"},
	})
	a := Automation{Name: "convoy-check", Gate: "event", On: "bead.closed"}
	result := CheckGate(a, time.Time{}, neverRan, ep, nil)
	if result.Due {
		t.Errorf("Due = true, want false (no matching events)")
	}
}

func TestCheckGateEventNoEventsProvider(t *testing.T) {
	a := Automation{Name: "convoy-check", Gate: "event", On: "bead.closed"}
	result := CheckGate(a, time.Time{}, neverRan, nil, nil)
	if result.Due {
		t.Errorf("Due = true, want false (nil provider)")
	}
}

func TestCheckGateCooldownRigScoped(t *testing.T) {
	// Rig automation should query with scoped name; city automation with plain name.
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)

	queriedNames := []string{}
	lastRunFn := func(name string) (time.Time, error) {
		queriedNames = append(queriedNames, name)
		return time.Time{}, nil
	}

	// Rig-scoped automation.
	rigA := Automation{Name: "dolt-health", Rig: "demo-repo", Gate: "cooldown", Interval: "1h"}
	CheckGate(rigA, now, lastRunFn, nil, nil)

	// City-level automation.
	cityA := Automation{Name: "dolt-health", Gate: "cooldown", Interval: "1h"}
	CheckGate(cityA, now, lastRunFn, nil, nil)

	if len(queriedNames) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(queriedNames))
	}
	if queriedNames[0] != "dolt-health:rig:demo-repo" {
		t.Errorf("rig query = %q, want %q", queriedNames[0], "dolt-health:rig:demo-repo")
	}
	if queriedNames[1] != "dolt-health" {
		t.Errorf("city query = %q, want %q", queriedNames[1], "dolt-health")
	}
}

func TestCheckGateCronRigScoped(t *testing.T) {
	// Rig automation cron gate queries scoped name.
	now := time.Date(2026, 2, 27, 3, 0, 0, 0, time.UTC) // matches "0 3 * * *"

	var queriedName string
	lastRunFn := func(name string) (time.Time, error) {
		queriedName = name
		return time.Time{}, nil
	}

	a := Automation{Name: "cleanup", Rig: "my-rig", Gate: "cron", Schedule: "0 3 * * *"}
	CheckGate(a, now, lastRunFn, nil, nil)

	if queriedName != "cleanup:rig:my-rig" {
		t.Errorf("cron query = %q, want %q", queriedName, "cleanup:rig:my-rig")
	}
}

func TestCheckGateEventRigScoped(t *testing.T) {
	ep := newEventsProvider(t, []events.Event{
		{Type: "bead.closed"},
	})

	var queriedName string
	cursorFn := func(name string) uint64 {
		queriedName = name
		return 0
	}

	a := Automation{Name: "convoy-check", Rig: "my-rig", Gate: "event", On: "bead.closed"}
	CheckGate(a, time.Time{}, neverRan, ep, cursorFn)

	if queriedName != "convoy-check:rig:my-rig" {
		t.Errorf("event cursor query = %q, want %q", queriedName, "convoy-check:rig:my-rig")
	}
}

func TestMaxSeqFromLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels [][]string
		want   uint64
	}{
		{
			name:   "single wisp",
			labels: [][]string{{"automation:convoy-check", "seq:42"}},
			want:   42,
		},
		{
			name:   "multiple wisps pick max",
			labels: [][]string{{"automation:convoy-check", "seq:10"}, {"automation:convoy-check", "seq:99"}},
			want:   99,
		},
		{
			name:   "mixed labels",
			labels: [][]string{{"pool:dog", "seq:5", "automation:convoy-check"}},
			want:   5,
		},
		{
			name:   "no seq labels",
			labels: [][]string{{"automation:convoy-check"}},
			want:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxSeqFromLabels(tt.labels)
			if got != tt.want {
				t.Errorf("MaxSeqFromLabels = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMaxSeqFromLabelsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		labels [][]string
	}{
		{"nil", nil},
		{"empty", [][]string{}},
		{"no labels", [][]string{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxSeqFromLabels(tt.labels)
			if got != 0 {
				t.Errorf("MaxSeqFromLabels = %d, want 0", got)
			}
		})
	}
}
