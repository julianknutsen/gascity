package formula

import (
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
)

// stubResolver returns a Resolver that returns a fixed formula by name.
func stubResolver(formulas map[string]*Formula) Resolver {
	return func(name string) (*Formula, error) {
		f, ok := formulas[name]
		if !ok {
			return nil, fmt.Errorf("formula %q not found", name)
		}
		return f, nil
	}
}

func TestComposeMolCook(t *testing.T) {
	store := beads.NewMemStore()
	f := &Formula{
		Name: "deploy",
		Steps: []Step{
			{ID: "build", Title: "Build binary"},
			{ID: "test", Title: "Run tests", Needs: []string{"build"}},
			{ID: "ship", Title: "Ship it", Needs: []string{"test"}},
		},
	}
	resolver := stubResolver(map[string]*Formula{"deploy": f})

	rootID, err := ComposeMolCook(store, resolver, "deploy", "Deploy v2", nil)
	if err != nil {
		t.Fatalf("ComposeMolCook: %v", err)
	}

	// Verify root bead.
	root, err := store.Get(rootID)
	if err != nil {
		t.Fatalf("Get root: %v", err)
	}
	if root.Type != "molecule" {
		t.Errorf("root.Type = %q, want %q", root.Type, "molecule")
	}
	if root.Ref != "deploy" {
		t.Errorf("root.Ref = %q, want %q", root.Ref, "deploy")
	}
	if root.Title != "Deploy v2" {
		t.Errorf("root.Title = %q, want %q", root.Title, "Deploy v2")
	}

	// Verify step beads.
	children, err := store.Children(rootID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("len(children) = %d, want 3", len(children))
	}

	if children[0].Ref != "build" {
		t.Errorf("children[0].Ref = %q, want %q", children[0].Ref, "build")
	}
	if children[0].Type != "task" {
		t.Errorf("children[0].Type = %q, want %q", children[0].Type, "task")
	}
	if children[0].ParentID != rootID {
		t.Errorf("children[0].ParentID = %q, want %q", children[0].ParentID, rootID)
	}

	if children[1].Ref != "test" {
		t.Errorf("children[1].Ref = %q, want %q", children[1].Ref, "test")
	}
	if children[2].Ref != "ship" {
		t.Errorf("children[2].Ref = %q, want %q", children[2].Ref, "ship")
	}
}

func TestComposeMolCookVars(t *testing.T) {
	store := beads.NewMemStore()
	f := &Formula{
		Name: "greet",
		Steps: []Step{
			{ID: "say-hi", Title: "Greet", Description: "Say hello to {{name}} in {{lang}}"},
		},
	}
	resolver := stubResolver(map[string]*Formula{"greet": f})

	rootID, err := ComposeMolCook(store, resolver, "greet", "Greeting", []string{"name=World", "lang=Go"})
	if err != nil {
		t.Fatalf("ComposeMolCook: %v", err)
	}

	children, err := store.Children(rootID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}
	want := "Say hello to World in Go"
	if children[0].Description != want {
		t.Errorf("Description = %q, want %q", children[0].Description, want)
	}
}

func TestComposeMolCookNeeds(t *testing.T) {
	store := beads.NewMemStore()
	f := &Formula{
		Name: "pipeline",
		Steps: []Step{
			{ID: "a", Title: "Step A"},
			{ID: "b", Title: "Step B", Needs: []string{"a"}},
		},
	}
	resolver := stubResolver(map[string]*Formula{"pipeline": f})

	rootID, err := ComposeMolCook(store, resolver, "pipeline", "", nil)
	if err != nil {
		t.Fatalf("ComposeMolCook: %v", err)
	}

	children, err := store.Children(rootID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("len(children) = %d, want 2", len(children))
	}
	if len(children[0].Needs) != 0 {
		t.Errorf("children[0].Needs = %v, want empty", children[0].Needs)
	}
	if len(children[1].Needs) != 1 || children[1].Needs[0] != "a" {
		t.Errorf("children[1].Needs = %v, want [a]", children[1].Needs)
	}
}

func TestComposeMolCookDefaultTitle(t *testing.T) {
	store := beads.NewMemStore()
	f := &Formula{
		Name:  "quick",
		Steps: []Step{{ID: "s1", Title: "Only step"}},
	}
	resolver := stubResolver(map[string]*Formula{"quick": f})

	rootID, err := ComposeMolCook(store, resolver, "quick", "", nil)
	if err != nil {
		t.Fatalf("ComposeMolCook: %v", err)
	}
	root, err := store.Get(rootID)
	if err != nil {
		t.Fatalf("Get root: %v", err)
	}
	if root.Title != "quick" {
		t.Errorf("root.Title = %q, want %q (formula name as default)", root.Title, "quick")
	}
}

func TestComposeMolCookResolverError(t *testing.T) {
	store := beads.NewMemStore()
	resolver := stubResolver(map[string]*Formula{})

	_, err := ComposeMolCook(store, resolver, "nonexistent", "title", nil)
	if err == nil {
		t.Fatal("expected error for missing formula")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error = %q, want to contain formula name", err)
	}
}

func TestSubstituteVars(t *testing.T) {
	f := &Formula{
		Name: "test",
		Steps: []Step{
			{ID: "a", Title: "Step A", Description: "Deploy {{app}} to {{env}}"},
			{ID: "b", Title: "Step B", Description: "No vars here"},
		},
	}

	got := SubstituteVars(f, []string{"app=myapp", "env=prod"})

	// Original is not modified.
	if f.Steps[0].Description != "Deploy {{app}} to {{env}}" {
		t.Errorf("original modified: %q", f.Steps[0].Description)
	}

	if got.Steps[0].Description != "Deploy myapp to prod" {
		t.Errorf("Steps[0].Description = %q, want %q", got.Steps[0].Description, "Deploy myapp to prod")
	}
	if got.Steps[1].Description != "No vars here" {
		t.Errorf("Steps[1].Description = %q, want %q", got.Steps[1].Description, "No vars here")
	}
}

func TestSubstituteVarsEmpty(t *testing.T) {
	f := &Formula{
		Name:  "test",
		Steps: []Step{{ID: "a", Title: "A", Description: "{{x}}"}},
	}

	// No vars → same formula returned.
	got := SubstituteVars(f, nil)
	if got != f {
		t.Error("SubstituteVars with nil vars should return same pointer")
	}

	got = SubstituteVars(f, []string{})
	if got != f {
		t.Error("SubstituteVars with empty vars should return same pointer")
	}
}

func TestSubstituteVarsInvalidFormat(t *testing.T) {
	f := &Formula{
		Name:  "test",
		Steps: []Step{{ID: "a", Title: "A", Description: "{{x}}"}},
	}

	// Vars without = are ignored.
	got := SubstituteVars(f, []string{"no-equals"})
	if got != f {
		t.Error("SubstituteVars with no valid pairs should return same pointer")
	}
}
