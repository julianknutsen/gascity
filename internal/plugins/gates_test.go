package plugins

import (
	"testing"
	"time"
)

func neverRan(_ string) (time.Time, error) { return time.Time{}, nil }

func TestCheckGateCooldownNeverRun(t *testing.T) {
	p := Plugin{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
	if !result.Due {
		t.Errorf("Due = false, want true (never run)")
	}
	if result.Reason != "never run" {
		t.Errorf("Reason = %q, want %q", result.Reason, "never run")
	}
}

func TestCheckGateCooldownDue(t *testing.T) {
	p := Plugin{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	lastRun := now.Add(-25 * time.Hour) // 25h ago — past the 24h interval
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(p, now, lastRunFn)
	if !result.Due {
		t.Errorf("Due = false, want true (25h > 24h)")
	}
}

func TestCheckGateCooldownNotDue(t *testing.T) {
	p := Plugin{Name: "digest", Gate: "cooldown", Interval: "24h"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	lastRun := now.Add(-12 * time.Hour) // 12h ago — within 24h interval
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(p, now, lastRunFn)
	if result.Due {
		t.Errorf("Due = true, want false (12h < 24h)")
	}
}

func TestCheckGateManual(t *testing.T) {
	p := Plugin{Name: "deploy", Gate: "manual"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
	if result.Due {
		t.Errorf("Due = true, want false (manual never auto-fires)")
	}
}

func TestCheckGateCronMatched(t *testing.T) {
	p := Plugin{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	// 03:00 UTC — should match.
	now := time.Date(2026, 2, 27, 3, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
	if !result.Due {
		t.Errorf("Due = false, want true (schedule matches 03:00)")
	}
}

func TestCheckGateCronNotMatched(t *testing.T) {
	p := Plugin{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	// 12:00 UTC — should not match.
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
	if result.Due {
		t.Errorf("Due = true, want false (schedule doesn't match 12:00)")
	}
}

func TestCheckGateCronAlreadyRunThisMinute(t *testing.T) {
	p := Plugin{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	now := time.Date(2026, 2, 27, 3, 0, 30, 0, time.UTC)
	lastRun := time.Date(2026, 2, 27, 3, 0, 10, 0, time.UTC) // same minute
	lastRunFn := func(_ string) (time.Time, error) { return lastRun, nil }

	result := CheckGate(p, now, lastRunFn)
	if result.Due {
		t.Errorf("Due = true, want false (already run this minute)")
	}
}

func TestCheckGateCondition(t *testing.T) {
	p := Plugin{Name: "check", Gate: "condition", Check: "true"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
	if !result.Due {
		t.Errorf("Due = false, want true (exit 0)")
	}
}

func TestCheckGateConditionFails(t *testing.T) {
	p := Plugin{Name: "check", Gate: "condition", Check: "false"}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	result := CheckGate(p, now, neverRan)
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
