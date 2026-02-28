package automations

import (
	"testing"
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

func TestValidateMissingFormula(t *testing.T) {
	a := Automation{Name: "bad", Gate: "manual"}
	if err := Validate(a); err == nil {
		t.Error("Validate should fail: missing formula")
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
