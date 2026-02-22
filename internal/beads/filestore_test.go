package beads_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestFileStoreOpenCorruptedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.json")
	if err := os.WriteFile(path, []byte("{not json!!!"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := beads.OpenFileStore(path)
	if err == nil {
		t.Fatal("expected error for corrupted JSON")
	}
	if !strings.Contains(err.Error(), "opening file store") {
		t.Errorf("error = %q, want 'opening file store' prefix", err)
	}
}

func TestFileStoreOpenUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not prevent reading on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can read any file")
	}

	path := filepath.Join(t.TempDir(), "beads.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) }) //nolint:errcheck // best-effort cleanup

	_, err := beads.OpenFileStore(path)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if !strings.Contains(err.Error(), "opening file store") {
		t.Errorf("error = %q, want 'opening file store' prefix", err)
	}
}
