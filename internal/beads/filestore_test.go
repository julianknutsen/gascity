package beads_test

import (
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
)

func TestFileStore(t *testing.T) {
	beadstest.RunStoreTests(t, func() beads.Store {
		path := filepath.Join(t.TempDir(), "beads.json")
		s, err := beads.OpenFileStore(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestFileStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.json")

	// First process: create two beads.
	s1, err := beads.OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := s1.Create(beads.Bead{Title: "first"})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := s1.Create(beads.Bead{Title: "second"})
	if err != nil {
		t.Fatal(err)
	}

	// Second process: open a new FileStore on the same path.
	s2, err := beads.OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Get works for both beads.
	got1, err := s2.Get(b1.ID)
	if err != nil {
		t.Fatalf("Get(%q) after reopen: %v", b1.ID, err)
	}
	if got1.Title != "first" {
		t.Errorf("Title = %q, want %q", got1.Title, "first")
	}

	got2, err := s2.Get(b2.ID)
	if err != nil {
		t.Fatalf("Get(%q) after reopen: %v", b2.ID, err)
	}
	if got2.Title != "second" {
		t.Errorf("Title = %q, want %q", got2.Title, "second")
	}

	// Verify next Create continues the sequence.
	b3, err := s2.Create(beads.Bead{Title: "third"})
	if err != nil {
		t.Fatal(err)
	}
	if b3.ID != "gc-3" {
		t.Errorf("third bead ID = %q, want %q", b3.ID, "gc-3")
	}
}

func TestFileStoreOpenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "beads.json")

	// Opening a non-existent file should succeed (creates parent dirs).
	s, err := beads.OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}

	// First bead should be gc-1.
	b, err := s.Create(beads.Bead{Title: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "gc-1" {
		t.Errorf("ID = %q, want %q", b.ID, "gc-1")
	}
}
