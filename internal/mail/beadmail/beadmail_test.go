package beadmail

import (
	"errors"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/mail"
)

// --- Send ---

func TestSend(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	m, err := p.Send("human", "mayor", "hello there")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if m.ID == "" {
		t.Error("Send returned empty ID")
	}
	if m.From != "human" {
		t.Errorf("From = %q, want %q", m.From, "human")
	}
	if m.To != "mayor" {
		t.Errorf("To = %q, want %q", m.To, "mayor")
	}
	if m.Body != "hello there" {
		t.Errorf("Body = %q, want %q", m.Body, "hello there")
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	// Verify underlying bead.
	b, err := store.Get(m.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if b.Type != "message" {
		t.Errorf("bead Type = %q, want %q", b.Type, "message")
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q", b.Status, "open")
	}
}

// --- Inbox ---

func TestInboxEmpty(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	msgs, err := p.Inbox("mayor")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Inbox = %d messages, want 0", len(msgs))
	}
}

func TestInboxFilters(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	// Message to mayor.
	if _, err := p.Send("human", "mayor", "for mayor"); err != nil {
		t.Fatal(err)
	}
	// Message to worker.
	if _, err := p.Send("human", "worker", "for worker"); err != nil {
		t.Fatal(err)
	}
	// Task bead (not a message).
	store.Create(beads.Bead{Title: "a task"}) //nolint:errcheck

	msgs, err := p.Inbox("mayor")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Inbox = %d messages, want 1", len(msgs))
	}
	if msgs[0].Body != "for mayor" {
		t.Errorf("Body = %q, want %q", msgs[0].Body, "for mayor")
	}
}

func TestInboxExcludesClosed(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	m, err := p.Send("human", "mayor", "will be read")
	if err != nil {
		t.Fatal(err)
	}
	// Read (marks as closed).
	if _, err := p.Read(m.ID); err != nil {
		t.Fatal(err)
	}

	msgs, err := p.Inbox("mayor")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Inbox = %d messages, want 0 (read messages excluded)", len(msgs))
	}
}

// --- Read ---

func TestRead(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	sent, err := p.Send("human", "mayor", "read me")
	if err != nil {
		t.Fatal(err)
	}

	m, err := p.Read(sent.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if m.Body != "read me" {
		t.Errorf("Body = %q, want %q", m.Body, "read me")
	}

	// Bead should be closed now.
	b, err := store.Get(sent.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q", b.Status, "closed")
	}
}

func TestReadAlreadyClosed(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	sent, err := p.Send("human", "mayor", "old news")
	if err != nil {
		t.Fatal(err)
	}
	// Close it first.
	store.Close(sent.ID) //nolint:errcheck

	// Reading already-closed message should still return it.
	m, err := p.Read(sent.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if m.Body != "old news" {
		t.Errorf("Body = %q, want %q", m.Body, "old news")
	}
}

func TestReadNotFound(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	_, err := p.Read("gc-999")
	if err == nil {
		t.Error("Read should fail for nonexistent ID")
	}
}

// --- Archive ---

func TestArchive(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	sent, err := p.Send("human", "mayor", "dismiss me")
	if err != nil {
		t.Fatal(err)
	}

	if err := p.Archive(sent.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Bead should be closed.
	b, err := store.Get(sent.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q", b.Status, "closed")
	}
}

func TestArchiveNonMessage(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	// Create a task bead (not a message).
	b, err := store.Create(beads.Bead{Title: "a task"})
	if err != nil {
		t.Fatal(err)
	}

	err = p.Archive(b.ID)
	if err == nil {
		t.Error("Archive should fail for non-message beads")
	}
}

func TestArchiveAlreadyClosed(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	sent, err := p.Send("human", "mayor", "old")
	if err != nil {
		t.Fatal(err)
	}
	store.Close(sent.ID) //nolint:errcheck

	// Archiving already-closed message returns ErrAlreadyArchived.
	err = p.Archive(sent.ID)
	if !errors.Is(err, mail.ErrAlreadyArchived) {
		t.Errorf("Archive already closed: got %v, want ErrAlreadyArchived", err)
	}
}

func TestArchiveNotFound(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	err := p.Archive("gc-999")
	if err == nil {
		t.Error("Archive should fail for nonexistent ID")
	}
}

// --- Check ---

func TestCheck(t *testing.T) {
	store := beads.NewMemStore()
	p := New(store)

	if _, err := p.Send("human", "mayor", "check me"); err != nil {
		t.Fatal(err)
	}

	msgs, err := p.Check("mayor")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Check = %d messages, want 1", len(msgs))
	}
	if msgs[0].Body != "check me" {
		t.Errorf("Body = %q, want %q", msgs[0].Body, "check me")
	}

	// Check should NOT mark as read (bead still open).
	b, err := store.Get(msgs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q (Check must not close beads)", b.Status, "open")
	}
}

// --- Compile-time interface check ---

var _ mail.Provider = (*Provider)(nil)
