package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

var testCookingFormula = []byte(`
formula = "cooking"
description = "Generic cooking workflow"

[[steps]]
id = "dry"
title = "Gather dry ingredients"
description = "Measure and combine all dry ingredients from the recipe."

[[steps]]
id = "wet"
title = "Gather wet ingredients"
description = "Measure and combine all wet ingredients from the recipe."

[[steps]]
id = "combine"
title = "Combine wet and dry"
description = "Fold wet into dry according to recipe instructions."
needs = ["dry", "wet"]

[[steps]]
id = "cook"
title = "Cook"
description = "Cook according to the recipe's method and temperature."
needs = ["combine"]

[[steps]]
id = "serve"
title = "Serve"
description = "Plate and garnish according to the recipe."
needs = ["cook"]
`)

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

func testAllFormulasFS() *fsys.Fake {
	fs := testFormulaFS()
	fs.Files["/formulas/cooking.formula.toml"] = testCookingFormula
	return fs
}

// --- gc mol create ---

func TestMolCreateSuccess(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()

	var stdout, stderr bytes.Buffer
	code := doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &stdout, &stderr)
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
	code := doMolCreate(store, events.Discard, fs, "/formulas", "nonexistent", "", &bytes.Buffer{}, &stderr)
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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	if !strings.Contains(stderr.String(), "not a molecule and has no attached molecule") {
		t.Errorf("stderr = %q, want 'not a molecule and has no attached molecule'", stderr.String())
	}
}

func TestMolStatusAllComplete(t *testing.T) {
	store := beads.NewMemStore()
	fs := testFormulaFS()
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

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
	doMolCreate(store, events.Discard, fs, "/formulas", "pancakes", "", &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "nonexistent", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolStepDone = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want 'not found'", stderr.String())
	}
}

// --- Tutorial 07: Attached molecules ---

func TestMolCreateOnBead(t *testing.T) {
	store := beads.NewMemStore()
	fs := testAllFormulasFS()

	// Create the base bead (gc-1).
	_, err := store.Create(beads.Bead{
		Title:       "Pancakes recipe",
		Description: "dry=flour,sugar,salt. wet=eggs,milk,butter. temp=375F",
	})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	// gc-1 is base, molecule root will be gc-2, steps gc-3..gc-7.
	code := doMolCreate(store, events.Discard, fs, "/formulas", "cooking", "gc-1", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolCreate = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "attached to gc-1") {
		t.Errorf("stdout = %q, want 'attached to gc-1'", stdout.String())
	}

	// Verify base bead's description has the attachment.
	base, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	got := beads.GetAttachedMol(base.Description)
	if got != "gc-2" {
		t.Errorf("attached_molecule = %q, want %q", got, "gc-2")
	}
}

func TestMolCreateOnBeadNotFound(t *testing.T) {
	store := beads.NewMemStore()
	fs := testAllFormulasFS()

	var stderr bytes.Buffer
	code := doMolCreate(store, events.Discard, fs, "/formulas", "cooking", "gc-999", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("doMolCreate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc-999") {
		t.Errorf("stderr = %q, want mention of gc-999", stderr.String())
	}
}

func TestMolStatusAttached(t *testing.T) {
	store := beads.NewMemStore()
	fs := testAllFormulasFS()

	_, _ = store.Create(beads.Bead{
		Title:       "Pancakes recipe",
		Description: "dry=flour,sugar,salt. wet=eggs,milk,butter. temp=375F",
	})
	doMolCreate(store, events.Discard, fs, "/formulas", "cooking", "gc-1", &bytes.Buffer{}, &bytes.Buffer{})

	var stdout, stderr bytes.Buffer
	code := doMolStatus(store, "gc-1", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolStatus = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	// Should show bead context.
	if !strings.Contains(out, "BEAD") {
		t.Errorf("stdout missing 'BEAD': %q", out)
	}
	if !strings.Contains(out, "Context:") {
		t.Errorf("stdout missing 'Context:': %q", out)
	}
	if !strings.Contains(out, "dry=flour") {
		t.Errorf("stdout missing bead description: %q", out)
	}
	// Should show molecule progress.
	if !strings.Contains(out, "MOLECULE  gc-2") {
		t.Errorf("stdout missing 'MOLECULE  gc-2': %q", out)
	}
	if !strings.Contains(out, "0/5 complete") {
		t.Errorf("stdout missing '0/5 complete': %q", out)
	}
	// Hint command should use base bead ID.
	if !strings.Contains(out, "gc mol step done gc-1 dry") {
		t.Errorf("stdout missing 'gc mol step done gc-1 dry': %q", out)
	}
}

func TestMolStepDoneAttached(t *testing.T) {
	store := beads.NewMemStore()
	fs := testAllFormulasFS()

	_, _ = store.Create(beads.Bead{
		Title:       "Pancakes recipe",
		Description: "dry=flour,sugar,salt. wet=eggs,milk,butter. temp=375F",
	})
	doMolCreate(store, events.Discard, fs, "/formulas", "cooking", "gc-1", &bytes.Buffer{}, &bytes.Buffer{})

	var stdout, stderr bytes.Buffer
	code := doMolStepDone(store, events.Discard, "gc-1", "dry", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMolStepDone = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Step 1/5: dry") {
		t.Errorf("stdout missing step progress: %q", out)
	}
	if !strings.Contains(out, "Current step: wet") {
		t.Errorf("stdout missing next step: %q", out)
	}
	// Hint should use base bead ID.
	if !strings.Contains(out, "gc mol step done gc-1 wet") {
		t.Errorf("stdout missing hint with base ID: %q", out)
	}
}

func TestMolStepDoneAttachedLastStep(t *testing.T) {
	store := beads.NewMemStore()
	fs := testAllFormulasFS()

	_, _ = store.Create(beads.Bead{
		Title:       "Pancakes recipe",
		Description: "dry=flour,sugar,salt. wet=eggs,milk,butter. temp=375F",
	})
	doMolCreate(store, events.Discard, fs, "/formulas", "cooking", "gc-1", &bytes.Buffer{}, &bytes.Buffer{})

	// Close all steps except serve via base bead.
	for _, ref := range []string{"dry", "wet", "combine", "cook"} {
		doMolStepDone(store, events.Discard, "gc-1", ref, &bytes.Buffer{}, &bytes.Buffer{})
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

	// Molecule root (gc-2) should be closed.
	mol, _ := store.Get("gc-2")
	if mol.Status != "closed" {
		t.Errorf("molecule.Status = %q, want %q", mol.Status, "closed")
	}

	// Base bead (gc-1) should still be open.
	base, _ := store.Get("gc-1")
	if base.Status != "open" {
		t.Errorf("base.Status = %q, want %q", base.Status, "open")
	}
}
