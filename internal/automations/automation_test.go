package automations

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	data := []byte(`
[automation]
description = "Generate daily digest"
formula = "mol-digest-generate"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	a, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a.Formula != "mol-digest-generate" {
		t.Errorf("Formula = %q, want %q", a.Formula, "mol-digest-generate")
	}
	if a.Gate != "cooldown" {
		t.Errorf("Gate = %q, want %q", a.Gate, "cooldown")
	}
	if a.Interval != "24h" {
		t.Errorf("Interval = %q, want %q", a.Interval, "24h")
	}
	if a.Pool != "dog" {
		t.Errorf("Pool = %q, want %q", a.Pool, "dog")
	}
	if a.Description != "Generate daily digest" {
		t.Errorf("Description = %q, want %q", a.Description, "Generate daily digest")
	}
}

func TestParseEnabledDefault(t *testing.T) {
	data := []byte(`
[automation]
formula = "test"
gate = "manual"
`)
	a, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !a.IsEnabled() {
		t.Error("IsEnabled() = false, want true (default)")
	}
}

func TestParseEnabledExplicitFalse(t *testing.T) {
	data := []byte(`
[automation]
formula = "test"
gate = "manual"
enabled = false
`)
	a, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a.IsEnabled() {
		t.Error("IsEnabled() = true, want false")
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte(`not valid toml {{{`))
	if err == nil {
		t.Fatal("Parse should fail on invalid TOML")
	}
}

func TestValidateCooldown(t *testing.T) {
	a := Automation{Name: "digest", Formula: "mol-digest", Gate: "cooldown", Interval: "24h"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateCooldownMissingInterval(t *testing.T) {
	a := Automation{Name: "digest", Formula: "mol-digest", Gate: "cooldown"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: cooldown without interval")
	}
}

func TestValidateCooldownBadInterval(t *testing.T) {
	a := Automation{Name: "digest", Formula: "mol-digest", Gate: "cooldown", Interval: "not-a-duration"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: invalid interval")
	}
}

func TestValidateCron(t *testing.T) {
	a := Automation{Name: "cleanup", Formula: "mol-cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateCronMissingSchedule(t *testing.T) {
	a := Automation{Name: "cleanup", Formula: "mol-cleanup", Gate: "cron"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: cron without schedule")
	}
}

func TestValidateCondition(t *testing.T) {
	a := Automation{Name: "check", Formula: "mol-check", Gate: "condition", Check: "test -f /tmp/flag"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateConditionMissingCheck(t *testing.T) {
	a := Automation{Name: "check", Formula: "mol-check", Gate: "condition"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: condition without check")
	}
}

func TestValidateManual(t *testing.T) {
	a := Automation{Name: "deploy", Formula: "mol-deploy", Gate: "manual"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateMissingFormulaAndExec(t *testing.T) {
	a := Automation{Name: "bad", Gate: "manual"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: missing formula and exec")
	}
}

func TestValidateExecAutomation(t *testing.T) {
	a := Automation{Name: "poller", Exec: "scripts/poll.sh", Gate: "cooldown", Interval: "2m"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateExecAndFormulaMutuallyExclusive(t *testing.T) {
	a := Automation{Name: "both", Formula: "mol-x", Exec: "scripts/x.sh", Gate: "manual"}
	err := Validate(a)
	if err == nil {
		t.Error("Validate should fail: formula and exec both set")
	}
}

func TestValidateExecWithPool(t *testing.T) {
	a := Automation{Name: "bad", Exec: "scripts/x.sh", Gate: "manual", Pool: "worker"}
	err := Validate(a)
	if err == nil {
		t.Error("Validate should fail: exec with pool")
	}
}

func TestValidateTimeout(t *testing.T) {
	a := Automation{Name: "t", Formula: "mol-t", Gate: "manual", Timeout: "90s"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateTimeoutInvalid(t *testing.T) {
	a := Automation{Name: "t", Formula: "mol-t", Gate: "manual", Timeout: "not-a-duration"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: invalid timeout")
	}
}

func TestIsExec(t *testing.T) {
	exec := Automation{Name: "e", Exec: "scripts/x.sh"}
	if !exec.IsExec() {
		t.Error("IsExec() = false, want true")
	}
	formula := Automation{Name: "f", Formula: "mol-f"}
	if formula.IsExec() {
		t.Error("IsExec() = true, want false")
	}
}

func TestTimeoutOrDefault(t *testing.T) {
	tests := []struct {
		name string
		a    Automation
		want time.Duration
	}{
		{"exec default", Automation{Exec: "x.sh"}, 60 * time.Second},
		{"formula default", Automation{Formula: "mol-x"}, 30 * time.Second},
		{"custom timeout", Automation{Exec: "x.sh", Timeout: "90s"}, 90 * time.Second},
		{"invalid timeout falls back", Automation{Exec: "x.sh", Timeout: "bad"}, 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.TimeoutOrDefault()
			if got != tt.want {
				t.Errorf("TimeoutOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseExecAutomation(t *testing.T) {
	data := []byte(`
[automation]
description = "Poll wasteland"
exec = "$AUTOMATION_DIR/scripts/poll.sh"
gate = "cooldown"
interval = "2m"
timeout = "90s"
`)
	a, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a.Exec != "$AUTOMATION_DIR/scripts/poll.sh" {
		t.Errorf("Exec = %q, want %q", a.Exec, "$AUTOMATION_DIR/scripts/poll.sh")
	}
	if a.Formula != "" {
		t.Errorf("Formula = %q, want empty", a.Formula)
	}
	if a.Timeout != "90s" {
		t.Errorf("Timeout = %q, want %q", a.Timeout, "90s")
	}
}

func TestValidateMissingGate(t *testing.T) {
	a := Automation{Name: "bad", Formula: "mol-bad"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: missing gate")
	}
}

func TestValidateUnknownGate(t *testing.T) {
	a := Automation{Name: "bad", Formula: "mol-bad", Gate: "random"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: unknown gate type")
	}
}

func TestValidateEvent(t *testing.T) {
	a := Automation{Name: "convoy-check", Formula: "mol-convoy-check", Gate: "event", On: "bead.closed"}
	if err := Validate(a); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestScopedNameCityLevel(t *testing.T) {
	a := Automation{Name: "dolt-health"}
	if got := a.ScopedName(); got != "dolt-health" {
		t.Errorf("ScopedName() = %q, want %q", got, "dolt-health")
	}
}

func TestScopedNameRigLevel(t *testing.T) {
	a := Automation{Name: "dolt-health", Rig: "demo-repo"}
	want := "dolt-health:rig:demo-repo"
	if got := a.ScopedName(); got != want {
		t.Errorf("ScopedName() = %q, want %q", got, want)
	}
}

func TestValidateEventMissingOn(t *testing.T) {
	a := Automation{Name: "convoy-check", Formula: "mol-convoy-check", Gate: "event"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: event without on")
	}
}

func TestParseEventAutomation(t *testing.T) {
	data := []byte(`
[automation]
description = "Auto-close convoys where all children are closed"
formula = "mol-convoy-check"
gate = "event"
on = "bead.closed"
`)
	a, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a.Gate != "event" {
		t.Errorf("Gate = %q, want %q", a.Gate, "event")
	}
	if a.On != "bead.closed" {
		t.Errorf("On = %q, want %q", a.On, "bead.closed")
	}
	if a.Formula != "mol-convoy-check" {
		t.Errorf("Formula = %q, want %q", a.Formula, "mol-convoy-check")
	}
}
