package formula

import (
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
)

// --- Parse ---

func TestParseValid(t *testing.T) {
	data := []byte(`
formula = "pancakes"
description = "Make pancakes from scratch"

[[steps]]
id = "dry"
title = "Mix dry ingredients"
description = "Combine flour, sugar, baking powder, salt"

[[steps]]
id = "wet"
title = "Mix wet ingredients"
description = "Whisk eggs, milk, butter"

[[steps]]
id = "combine"
title = "Combine"
description = "Fold wet into dry"
needs = ["dry", "wet"]
`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Name != "pancakes" {
		t.Errorf("Name = %q, want %q", f.Name, "pancakes")
	}
	if f.Description != "Make pancakes from scratch" {
		t.Errorf("Description = %q", f.Description)
	}
	if len(f.Steps) != 3 {
		t.Fatalf("len(Steps) = %d, want 3", len(f.Steps))
	}
	if f.Steps[0].ID != "dry" {
		t.Errorf("Steps[0].ID = %q, want %q", f.Steps[0].ID, "dry")
	}
	if len(f.Steps[2].Needs) != 2 {
		t.Errorf("Steps[2].Needs = %v, want [dry wet]", f.Steps[2].Needs)
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte(`not valid toml {{{}}`))
	if err == nil {
		t.Fatal("Parse should fail on invalid TOML")
	}
}

// --- Validate ---

func TestValidateSuccess(t *testing.T) {
	f := &Formula{
		Name: "test",
		Steps: []Step{
			{ID: "a", Title: "A"},
			{ID: "b", Title: "B", Needs: []string{"a"}},
		},
	}
	if err := Validate(f); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateNoName(t *testing.T) {
	f := &Formula{Steps: []Step{{ID: "a"}}}
	err := Validate(f)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateNoSteps(t *testing.T) {
	f := &Formula{Name: "empty"}
	err := Validate(f)
	if err == nil {
		t.Fatal("expected error for no steps")
	}
	if !strings.Contains(err.Error(), "no steps") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateDuplicateID(t *testing.T) {
	f := &Formula{
		Name:  "dup",
		Steps: []Step{{ID: "a"}, {ID: "a"}},
	}
	err := Validate(f)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
	if !strings.Contains(err.Error(), "duplicate step ID") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateUnknownNeed(t *testing.T) {
	f := &Formula{
		Name:  "bad",
		Steps: []Step{{ID: "a", Needs: []string{"nonexistent"}}},
	}
	err := Validate(f)
	if err == nil {
		t.Fatal("expected error for unknown need")
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateCycle(t *testing.T) {
	f := &Formula{
		Name: "cycle",
		Steps: []Step{
			{ID: "a", Needs: []string{"b"}},
			{ID: "b", Needs: []string{"a"}},
		},
	}
	err := Validate(f)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q", err)
	}
}

// --- CurrentStep ---

func TestCurrentStepFirstReady(t *testing.T) {
	steps := []beads.Bead{
		{Ref: "a", Status: "open"},
		{Ref: "b", Status: "open", Needs: []string{"a"}},
	}
	got := CurrentStep(steps)
	if got == nil {
		t.Fatal("CurrentStep returned nil")
	}
	if got.Ref != "a" {
		t.Errorf("CurrentStep.Ref = %q, want %q", got.Ref, "a")
	}
}

func TestCurrentStepAfterClose(t *testing.T) {
	steps := []beads.Bead{
		{Ref: "a", Status: "closed"},
		{Ref: "b", Status: "open", Needs: []string{"a"}},
	}
	got := CurrentStep(steps)
	if got == nil {
		t.Fatal("CurrentStep returned nil")
	}
	if got.Ref != "b" {
		t.Errorf("CurrentStep.Ref = %q, want %q", got.Ref, "b")
	}
}

func TestCurrentStepAllDone(t *testing.T) {
	steps := []beads.Bead{
		{Ref: "a", Status: "closed"},
		{Ref: "b", Status: "closed"},
	}
	got := CurrentStep(steps)
	if got != nil {
		t.Errorf("CurrentStep = %v, want nil (all done)", got)
	}
}

func TestCurrentStepBlocked(t *testing.T) {
	steps := []beads.Bead{
		{Ref: "a", Status: "open", Needs: []string{"b"}},
		{Ref: "b", Status: "open", Needs: []string{"a"}},
	}
	got := CurrentStep(steps)
	if got != nil {
		t.Errorf("CurrentStep = %v, want nil (all blocked)", got)
	}
}

// --- CompletedCount ---

func TestCompletedCount(t *testing.T) {
	steps := []beads.Bead{
		{Status: "closed"},
		{Status: "open"},
		{Status: "closed"},
	}
	if got := CompletedCount(steps); got != 2 {
		t.Errorf("CompletedCount = %d, want 2", got)
	}
}

func TestCompletedCountEmpty(t *testing.T) {
	if got := CompletedCount(nil); got != 0 {
		t.Errorf("CompletedCount(nil) = %d, want 0", got)
	}
}

// --- StepIndex ---

func TestStepIndex(t *testing.T) {
	steps := []beads.Bead{
		{Ref: "a"},
		{Ref: "b"},
		{Ref: "c"},
	}
	if got := StepIndex(steps, "b"); got != 2 {
		t.Errorf("StepIndex(b) = %d, want 2", got)
	}
}

func TestStepIndexNotFound(t *testing.T) {
	steps := []beads.Bead{{Ref: "a"}}
	if got := StepIndex(steps, "z"); got != 0 {
		t.Errorf("StepIndex(z) = %d, want 0", got)
	}
}
