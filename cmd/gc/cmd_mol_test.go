package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

var testPancakesFormula = []byte(`
formula = "pancakes"
description = "Make pancakes from scratch"

[[steps]]
id = "dry"
title = "Mix dry ingredients"
description = "Combine flour, sugar, baking powder, salt."

[[steps]]
id = "wet"
title = "Mix wet ingredients"
description = "Whisk eggs, milk, butter."

[[steps]]
id = "combine"
title = "Combine"
description = "Fold wet into dry."
needs = ["dry", "wet"]

[[steps]]
id = "cook"
title = "Cook the pancakes"
description = "Heat griddle to 375F."
needs = ["combine"]

[[steps]]
id = "serve"
title = "Serve"
description = "Stack pancakes."
needs = ["cook"]
`)

func testFormulaFS() *fsys.Fake {
	fs := fsys.NewFake()
	fs.Dirs["/formulas"] = true
	fs.Files["/formulas/pancakes.formula.toml"] = testPancakesFormula
	return fs
}

// --- gc mol create ---

func TestMolCreateSuccess(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()

	var stdout, stderr bytes.Buffer
	code := doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolCreate = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Created molecule gc-1") {
		t.Errorf("stdout = %q, want creation confirmation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "5 steps") {
		t.Errorf("stdout = %q, want '5 steps'", stdout.String())
	}

	// Verify root bead.
	root, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if root.Type != "molecule" {
		t.Errorf("root.Type = %q, want %q", root.Type, "molecule")
	}
	if root.Ref != "pancakes" {
		t.Errorf("root.Ref = %q, want %q", root.Ref, "pancakes")
	}

	// Verify step beads.
	children, err := store.Children("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 5 {
		t.Fatalf("children count = %d, want 5", len(children))
	}
	if children[0].Ref != "dry" {
		t.Errorf("children[0].Ref = %q, want %q", children[0].Ref, "dry")
	}
	if children[0].Type != "step" {
		t.Errorf("children[0].Type = %q, want %q", children[0].Type, "step")
	}
	if children[0].ParentID != "gc-1" {
		t.Errorf("children[0].ParentID = %q, want %q", children[0].ParentID, "gc-1")
	}
	if children[2].Ref != "combine" {
		t.Errorf("children[2].Ref = %q, want %q", children[2].Ref, "combine")
	}
	if len(children[2].Needs) != 2 {
		t.Errorf("children[2].Needs = %v, want [dry wet]", children[2].Needs)
	}
}

func TestMolCreateFormulaNotFound(t *testing.T) {
	store := beads.NewMemStore()
	fs := fsys.NewFake()
	fs.Dirs["/formulas"] = true

	var stderr bytes.Buffer
	code := doMolCreate(store, events.Discard, fs, "/formulas", "nonexistent", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolCreate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want 'not found'", stderr.String())
	}
}

// --- gc mol list ---

func TestMolListEmpty(t *testing.T) {
	store := beads.NewMemStore()

	var stdout, stderr bytes.Buffer
	code := doMolList(store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No molecules found") {
		t.Errorf("stdout = %q, want 'No molecules found'", stdout.String())
	}
}

func TestMolListWithMolecules(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()

	// Create a molecule.
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	// Create a non-molecule bead (should be excluded).
	_, _ = store.Create(beads.Bead{Title: "regular task"})

	var stdout, stderr bytes.Buffer
	code := doMolList(store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolList = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "gc-1") {
		t.Errorf("stdout missing 'gc-1': %q", out)
	}
	if !strings.Contains(out, "pancakes") {
		t.Errorf("stdout missing 'pancakes': %q", out)
	}
	if !strings.Contains(out, "0/5") {
		t.Errorf("stdout missing '0/5': %q", out)
	}
	if strings.Contains(out, "regular task") {
		t.Errorf("stdout should not contain 'regular task': %q", out)
	}
}

// --- gc mol status ---

func TestMolStatusShowsCurrentStep(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	var stdout, stderr bytes.Buffer
	code := doMolStatus(store, "gc-1", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolStatus = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	// With no steps closed, "dry" should be the current step (it has no needs).
	if !strings.Contains(out, "Current step: dry") {
		t.Errorf("stdout missing 'Current step: dry': %q", out)
	}
	if !strings.Contains(out, "0/5 complete") {
		t.Errorf("stdout missing '0/5 complete': %q", out)
	}
	if !strings.Contains(out, "gc mol step done gc-1 dry") {
		t.Errorf("stdout missing done command: %q", out)
	}
}

func TestMolStatusNotAMolecule(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "just a task"})

	var stderr bytes.Buffer
	code := doMolStatus(store, "gc-1", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolStatus = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a molecule") {
		t.Errorf("stderr = %q, want 'not a molecule'", stderr.String())
	}
}

func TestMolStatusAllComplete(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	// Close all steps.
	children, _ := store.Children("gc-1")
	for _, c := range children {
		_ = store.Close(c.ID)
	}

	var stdout bytes.Buffer
	code := doMolStatus(store, "gc-1", &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMolStatus = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "All steps complete") {
		t.Errorf("stdout = %q, want 'All steps complete'", stdout.String())
	}
}

// --- gc mol step done ---

func TestMolStepDoneSuccess(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	var stdout, stderr bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "dry", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolStepDone = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Step 1/5: dry") {
		t.Errorf("stdout missing step progress: %q", out)
	}
	// After closing "dry", "wet" should be the next step (it has no needs).
	if !strings.Contains(out, "Current step: wet") {
		t.Errorf("stdout missing next step: %q", out)
	}
}

func TestMolStepDoneLastStep(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	// Close all but the last step.
	children, _ := store.Children("gc-1")
	for _, c := range children {
		if c.Ref != "serve" {
			_ = store.Close(c.ID)
		}
	}

	var stdout bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "serve", &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMolStepDone = %d, want 0", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "All steps complete") {
		t.Errorf("stdout missing 'All steps complete': %q", out)
	}

	// Verify molecule root is closed.
	root, _ := store.Get("gc-1")
	if root.Status != "closed" {
		t.Errorf("root.Status = %q, want %q", root.Status, "closed")
	}
}

func TestMolStepDoneAlreadyClosed(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	// Close "dry".
	doMolStepDone(store, events.Discard, "gc-1", "dry", &bytes.Buffer{}, &bytes.Buffer{})

	// Try to close it again.
	var stderr bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "dry", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolStepDone = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already closed") {
		t.Errorf("stderr = %q, want 'already closed'", stderr.String())
	}
}

func TestMolStepDoneNotFound(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "nonexistent", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolStepDone = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want 'not found'", stderr.String())
	}
}
