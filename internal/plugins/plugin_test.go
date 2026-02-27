package plugins

import (
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte(`
[plugin]
description = "Generate daily digest"
formula = "mol-digest-generate"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Formula != "mol-digest-generate" {
		t.Errorf("Formula = %q, want %q", p.Formula, "mol-digest-generate")
	}
	if p.Gate != "cooldown" {
		t.Errorf("Gate = %q, want %q", p.Gate, "cooldown")
	}
	if p.Interval != "24h" {
		t.Errorf("Interval = %q, want %q", p.Interval, "24h")
	}
	if p.Pool != "dog" {
		t.Errorf("Pool = %q, want %q", p.Pool, "dog")
	}
	if p.Description != "Generate daily digest" {
		t.Errorf("Description = %q, want %q", p.Description, "Generate daily digest")
	}
}

func TestParseEnabledDefault(t *testing.T) {
	data := []byte(`
[plugin]
formula = "test"
gate = "manual"
`)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.IsEnabled() {
		t.Error("IsEnabled() = false, want true (default)")
	}
}

func TestParseEnabledExplicitFalse(t *testing.T) {
	data := []byte(`
[plugin]
formula = "test"
gate = "manual"
enabled = false
`)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.IsEnabled() {
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
	p := Plugin{Name: "digest", Formula: "mol-digest", Gate: "cooldown", Interval: "24h"}
	if err := Validate(p); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateCooldownMissingInterval(t *testing.T) {
	p := Plugin{Name: "digest", Formula: "mol-digest", Gate: "cooldown"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: cooldown without interval")
	}
}

func TestValidateCooldownBadInterval(t *testing.T) {
	p := Plugin{Name: "digest", Formula: "mol-digest", Gate: "cooldown", Interval: "not-a-duration"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: invalid interval")
	}
}

func TestValidateCron(t *testing.T) {
	p := Plugin{Name: "cleanup", Formula: "mol-cleanup", Gate: "cron", Schedule: "0 3 * * *"}
	if err := Validate(p); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateCronMissingSchedule(t *testing.T) {
	p := Plugin{Name: "cleanup", Formula: "mol-cleanup", Gate: "cron"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: cron without schedule")
	}
}

func TestValidateCondition(t *testing.T) {
	p := Plugin{Name: "check", Formula: "mol-check", Gate: "condition", Check: "test -f /tmp/flag"}
	if err := Validate(p); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateConditionMissingCheck(t *testing.T) {
	p := Plugin{Name: "check", Formula: "mol-check", Gate: "condition"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: condition without check")
	}
}

func TestValidateManual(t *testing.T) {
	p := Plugin{Name: "deploy", Formula: "mol-deploy", Gate: "manual"}
	if err := Validate(p); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateMissingFormula(t *testing.T) {
	p := Plugin{Name: "bad", Gate: "manual"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: missing formula")
	}
}

func TestValidateMissingGate(t *testing.T) {
	p := Plugin{Name: "bad", Formula: "mol-bad"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: missing gate")
	}
}

func TestValidateUnknownGate(t *testing.T) {
	p := Plugin{Name: "bad", Formula: "mol-bad", Gate: "random"}
	if err := Validate(p); err == nil {
		t.Error("Validate should fail: unknown gate type")
	}
}
