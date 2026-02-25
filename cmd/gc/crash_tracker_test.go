package main

import (
	"testing"
	"time"
)

// fakeCrashTracker is a test double for crashTracker.
type fakeCrashTracker struct {
	quarantined map[string]bool
	starts      map[string][]time.Time
}

func newFakeCrashTracker() *fakeCrashTracker {
	return &fakeCrashTracker{
		quarantined: make(map[string]bool),
		starts:      make(map[string][]time.Time),
	}
}

func (f *fakeCrashTracker) recordStart(sessionName string, at time.Time) {
	f.starts[sessionName] = append(f.starts[sessionName], at)
}

func (f *fakeCrashTracker) isQuarantined(sessionName string, _ time.Time) bool {
	return f.quarantined[sessionName]
}

func (f *fakeCrashTracker) clearHistory(sessionName string) {
	delete(f.starts, sessionName)
	delete(f.quarantined, sessionName)
}

// --- memoryCrashTracker unit tests ---

func TestCrashTrackerNoQuarantineUnderLimit(t *testing.T) {
	ct := newCrashTracker(5, time.Hour)
	now := time.Now()

	// 2 starts — well under threshold of 5.
	ct.recordStart("gc-test-mayor", now)
	ct.recordStart("gc-test-mayor", now.Add(time.Minute))

	if ct.isQuarantined("gc-test-mayor", now.Add(2*time.Minute)) {
		t.Error("should not be quarantined with only 2 starts")
	}
}

func TestCrashTrackerQuarantineAtLimit(t *testing.T) {
	ct := newCrashTracker(5, time.Hour)
	now := time.Now()

	// 5 starts within window → quarantined.
	for i := 0; i < 5; i++ {
		ct.recordStart("gc-test-mayor", now.Add(time.Duration(i)*time.Minute))
	}

	if !ct.isQuarantined("gc-test-mayor", now.Add(6*time.Minute)) {
		t.Error("should be quarantined at 5 starts (threshold=5)")
	}
}

func TestCrashTrackerAutoClears(t *testing.T) {
	ct := newCrashTracker(3, 10*time.Minute)
	now := time.Now()

	// 3 starts at t=0..2m → quarantined.
	for i := 0; i < 3; i++ {
		ct.recordStart("gc-test-agent", now.Add(time.Duration(i)*time.Minute))
	}
	if !ct.isQuarantined("gc-test-agent", now.Add(3*time.Minute)) {
		t.Error("should be quarantined at 3 starts")
	}

	// After 10 minutes, all timestamps slide out of the window → auto-cleared.
	if ct.isQuarantined("gc-test-agent", now.Add(11*time.Minute)) {
		t.Error("should NOT be quarantined after window expires")
	}
}

func TestCrashTrackerPrunesOldEntries(t *testing.T) {
	ct := newCrashTracker(5, 10*time.Minute).(*memoryCrashTracker)
	now := time.Now()

	// Record 10 starts spread across 20 minutes.
	for i := 0; i < 10; i++ {
		ct.recordStart("gc-test-agent", now.Add(time.Duration(i)*2*time.Minute))
	}

	// Check at t=20m: only entries from t=10m..t=18m survive (5 entries).
	ct.isQuarantined("gc-test-agent", now.Add(20*time.Minute))
	if len(ct.starts["gc-test-agent"]) > 5 {
		t.Errorf("expected <= 5 entries after prune, got %d", len(ct.starts["gc-test-agent"]))
	}
}

func TestCrashTrackerClearHistory(t *testing.T) {
	ct := newCrashTracker(3, time.Hour)
	now := time.Now()

	for i := 0; i < 3; i++ {
		ct.recordStart("gc-test-agent", now.Add(time.Duration(i)*time.Minute))
	}
	if !ct.isQuarantined("gc-test-agent", now.Add(4*time.Minute)) {
		t.Fatal("precondition: should be quarantined")
	}

	ct.clearHistory("gc-test-agent")

	if ct.isQuarantined("gc-test-agent", now.Add(5*time.Minute)) {
		t.Error("should NOT be quarantined after clearHistory")
	}
}

func TestCrashTrackerNilSafe(t *testing.T) {
	// Callers guard with `if ct != nil` — verify nil tracker is returned
	// for disabled config, and callers won't panic.
	var ct crashTracker
	if ct != nil {
		t.Error("nil crashTracker should be nil")
	}
}

func TestCrashTrackerUnlimitedDisabled(t *testing.T) {
	// maxRestarts=0 → returns nil (disabled).
	ct := newCrashTracker(0, time.Hour)
	if ct != nil {
		t.Error("newCrashTracker(0, ...) should return nil")
	}

	// Negative also disabled.
	ct = newCrashTracker(-1, time.Hour)
	if ct != nil {
		t.Error("newCrashTracker(-1, ...) should return nil")
	}
}

func TestCrashTrackerPartialWindowSlide(t *testing.T) {
	// 5 starts: 3 old + 2 recent. After partial slide, only 2 remain.
	ct := newCrashTracker(3, 10*time.Minute)
	now := time.Now()

	// 3 starts at t=0..2m (old).
	for i := 0; i < 3; i++ {
		ct.recordStart("gc-test-agent", now.Add(time.Duration(i)*time.Minute))
	}
	if !ct.isQuarantined("gc-test-agent", now.Add(3*time.Minute)) {
		t.Fatal("precondition: should be quarantined")
	}

	// 2 more starts at t=12m and t=13m. The 3 old ones slide out.
	ct.recordStart("gc-test-agent", now.Add(12*time.Minute))
	ct.recordStart("gc-test-agent", now.Add(13*time.Minute))

	// At t=14m: old entries (t=0,1,2) are outside window (10m before t=14m = t=4m).
	// Only t=12m and t=13m survive → 2 < 3 → not quarantined.
	if ct.isQuarantined("gc-test-agent", now.Add(14*time.Minute)) {
		t.Error("should NOT be quarantined: old entries slid out, only 2 recent")
	}
}

func TestCrashTrackerDifferentSessions(t *testing.T) {
	ct := newCrashTracker(2, time.Hour)
	now := time.Now()

	// Quarantine agent A.
	ct.recordStart("gc-test-a", now)
	ct.recordStart("gc-test-a", now.Add(time.Minute))

	// Agent B has only 1 start.
	ct.recordStart("gc-test-b", now)

	if !ct.isQuarantined("gc-test-a", now.Add(2*time.Minute)) {
		t.Error("agent A should be quarantined")
	}
	if ct.isQuarantined("gc-test-b", now.Add(2*time.Minute)) {
		t.Error("agent B should NOT be quarantined")
	}
}

func TestCrashTrackerUnknownSession(t *testing.T) {
	ct := newCrashTracker(5, time.Hour)
	// Never-seen session should not be quarantined.
	if ct.isQuarantined("gc-test-unknown", time.Now()) {
		t.Error("unknown session should not be quarantined")
	}
}
