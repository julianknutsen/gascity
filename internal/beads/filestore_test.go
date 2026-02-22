package beads_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
	"github.com/steveyegge/gascity/internal/fsys"
)

func TestFileStore(t *testing.T) {
	factory := func() beads.Store {
		path := filepath.Join(t.TempDir(), "beads.json")
		s, err := beads.OpenFileStore(fsys.OSFS{}, path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	beadstest.RunStoreTests(t, factory)
	beadstest.RunSequentialIDTests(t, factory)
}

func TestFileStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beads.json")

	// First process: create two beads.
	s1, err := beads.OpenFileStore(fsys.OSFS{}, path)
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
	s2, err := beads.OpenFileStore(fsys.OSFS{}, path)
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
	s, err := beads.OpenFileStore(fsys.OSFS{}, path)
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

	_, err := beads.OpenFileStore(fsys.OSFS{}, path)
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

	_, err := beads.OpenFileStore(fsys.OSFS{}, path)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
	if !strings.Contains(err.Error(), "opening file store") {
		t.Errorf("error = %q, want 'opening file store' prefix", err)
	}
}

// --- failure-path tests with fsys.Fake ---

func TestFileStoreOpenMkdirFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors["/city/.gc"] = fmt.Errorf("permission denied")

	_, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want 'permission denied'", err)
	}
}

func TestFileStoreOpenReadFileFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors["/city/.gc/beads.json"] = fmt.Errorf("disk error")

	_, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err == nil {
		t.Fatal("expected error when ReadFile fails")
	}
	if !strings.Contains(err.Error(), "disk error") {
		t.Errorf("error = %q, want 'disk error'", err)
	}
}

func TestFileStoreOpenCorruptedJSONFake(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/.gc/beads.json"] = []byte("{not json!!!")

	_, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err == nil {
		t.Fatal("expected error for corrupted JSON")
	}
	if !strings.Contains(err.Error(), "opening file store") {
		t.Errorf("error = %q, want 'opening file store' prefix", err)
	}
}

func TestFileStoreSaveWriteFails(t *testing.T) {
	f := fsys.NewFake()
	s, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err != nil {
		t.Fatal(err)
	}

	// Inject error on the temp file write.
	f.Errors["/city/.gc/beads.json.tmp"] = fmt.Errorf("disk full")

	_, err = s.Create(beads.Bead{Title: "test"})
	if err == nil {
		t.Fatal("expected error when WriteFile fails")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q, want 'disk full'", err)
	}
}

func TestFileStoreSaveRenameFails(t *testing.T) {
	f := fsys.NewFake()
	s, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err != nil {
		t.Fatal(err)
	}

	// Inject error on the rename (atomic commit step).
	f.Errors["/city/.gc/beads.json.tmp"] = fmt.Errorf("rename failed")

	_, err = s.Create(beads.Bead{Title: "test"})
	if err == nil {
		t.Fatal("expected error when Rename fails")
	}
	if !strings.Contains(err.Error(), "rename failed") {
		t.Errorf("error = %q, want 'rename failed'", err)
	}
}

func TestFileStoreCloseWriteFails(t *testing.T) {
	f := fsys.NewFake()
	s, err := beads.OpenFileStore(f, "/city/.gc/beads.json")
	if err != nil {
		t.Fatal(err)
	}

	// Create a bead successfully first.
	b, err := s.Create(beads.Bead{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}

	// Now inject error on the next save (Close flushes).
	f.Errors["/city/.gc/beads.json.tmp"] = fmt.Errorf("disk full")

	err = s.Close(b.ID)
	if err == nil {
		t.Fatal("expected error when save fails during Close")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q, want 'disk full'", err)
	}
}
