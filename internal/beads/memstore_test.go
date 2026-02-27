package beads_test

import (
	"errors"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
)

func TestMemStore(t *testing.T) {
	factory := func() beads.Store { return beads.NewMemStore() }
	beadstest.RunStoreTests(t, factory)
	beadstest.RunSequentialIDTests(t, factory)
	beadstest.RunCreationOrderTests(t, factory)
}

func TestMemStoreSetMetadata(t *testing.T) {
	s := beads.NewMemStore()
	b, err := s.Create(beads.Bead{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMetadata(b.ID, "merge_strategy", "mr"); err != nil {
		t.Errorf("SetMetadata on existing bead: %v", err)
	}
}

func TestMemStoreSetMetadataNotFound(t *testing.T) {
	s := beads.NewMemStore()
	err := s.SetMetadata("nonexistent-999", "key", "value")
	if err == nil {
		t.Fatal("SetMetadata on nonexistent bead should return error")
	}
	if !errors.Is(err, beads.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestMemStoreMolCook(t *testing.T) {
	s := beads.NewMemStore()
	id, err := s.MolCook("code-review", "Review PR #42", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("MolCook returned empty ID")
	}

	// Verify the created bead.
	b, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if b.Type != "molecule" {
		t.Errorf("Type = %q, want %q", b.Type, "molecule")
	}
	if b.Title != "Review PR #42" {
		t.Errorf("Title = %q, want %q", b.Title, "Review PR #42")
	}
	if b.Ref != "code-review" {
		t.Errorf("Ref = %q, want %q", b.Ref, "code-review")
	}
}

func TestMemStoreMolCookDefaultTitle(t *testing.T) {
	s := beads.NewMemStore()
	id, err := s.MolCook("deploy", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	b, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "deploy" {
		t.Errorf("Title = %q, want %q (formula name as default)", b.Title, "deploy")
	}
}
