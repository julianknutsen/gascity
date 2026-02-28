// Package mailtest provides a conformance test suite for [mail.Provider]
// implementations. Each implementation's test file calls [RunProviderTests]
// with its own factory function.
package mailtest

import (
	"errors"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/mail"
)

// RunProviderTests runs the full conformance suite against a Provider.
// newProvider returns a fresh, empty provider per test.
func RunProviderTests(t *testing.T, newProvider func(t *testing.T) mail.Provider) {
	t.Helper()

	// --- Group 1: Send ---

	t.Run("Send_ReturnsMatchingFields", func(t *testing.T) {
		p := newProvider(t)
		m, err := p.Send("alice", "bob", "hello")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if m.ID == "" {
			t.Error("Send returned empty ID")
		}
		if m.From != "alice" {
			t.Errorf("From = %q, want %q", m.From, "alice")
		}
		if m.To != "bob" {
			t.Errorf("To = %q, want %q", m.To, "bob")
		}
		if m.Body != "hello" {
			t.Errorf("Body = %q, want %q", m.Body, "hello")
		}
	})

	t.Run("Send_AssignsUniqueIDs", func(t *testing.T) {
		p := newProvider(t)
		m1, err := p.Send("alice", "bob", "first")
		if err != nil {
			t.Fatalf("Send 1: %v", err)
		}
		m2, err := p.Send("alice", "bob", "second")
		if err != nil {
			t.Fatalf("Send 2: %v", err)
		}
		if m1.ID == m2.ID {
			t.Errorf("two sends produced same ID %q", m1.ID)
		}
	})

	t.Run("Send_SetsRecentCreatedAt", func(t *testing.T) {
		p := newProvider(t)
		before := time.Now().Add(-time.Minute)
		m, err := p.Send("alice", "bob", "timestamped")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		after := time.Now().Add(time.Minute)
		if m.CreatedAt.Before(before) || m.CreatedAt.After(after) {
			t.Errorf("CreatedAt = %v, want within ±1 minute of now", m.CreatedAt)
		}
	})

	// --- Group 2: Inbox ---

	t.Run("Inbox_EmptyReturnsNil", func(t *testing.T) {
		p := newProvider(t)
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if msgs != nil {
			t.Errorf("Inbox on empty provider = %v, want nil", msgs)
		}
	})

	t.Run("Inbox_ReturnsMessagesSentToRecipient", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "for bob")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("Inbox = %d messages, want 1", len(msgs))
		}
		if msgs[0].ID != sent.ID {
			t.Errorf("Inbox msg ID = %q, want %q", msgs[0].ID, sent.ID)
		}
	})

	t.Run("Inbox_FiltersByRecipient", func(t *testing.T) {
		p := newProvider(t)
		if _, err := p.Send("alice", "bob", "for bob"); err != nil {
			t.Fatalf("Send to bob: %v", err)
		}
		if _, err := p.Send("alice", "charlie", "for charlie"); err != nil {
			t.Fatalf("Send to charlie: %v", err)
		}
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("Inbox(bob) = %d messages, want 1", len(msgs))
		}
		if msgs[0].To != "bob" {
			t.Errorf("Inbox msg To = %q, want %q", msgs[0].To, "bob")
		}
	})

	t.Run("Inbox_ExcludesReadMessages", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "will be read")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if _, err := p.Read(sent.ID); err != nil {
			t.Fatalf("Read: %v", err)
		}
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Inbox after Read = %d messages, want 0", len(msgs))
		}
	})

	// --- Group 3: Check ---

	t.Run("Check_ReturnsUnreadMessages", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "check me")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		msgs, err := p.Check("bob")
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("Check = %d messages, want 1", len(msgs))
		}
		if msgs[0].ID != sent.ID {
			t.Errorf("Check msg ID = %q, want %q", msgs[0].ID, sent.ID)
		}
	})

	t.Run("Check_DoesNotMarkAsRead", func(t *testing.T) {
		p := newProvider(t)
		if _, err := p.Send("alice", "bob", "peek"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		if _, err := p.Check("bob"); err != nil {
			t.Fatalf("Check: %v", err)
		}
		// Inbox should still see the message.
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 1 {
			t.Errorf("Inbox after Check = %d messages, want 1", len(msgs))
		}
	})

	// --- Group 4: Read ---

	t.Run("Read_ReturnsCorrectMessage", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "read me")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		m, err := p.Read(sent.ID)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if m.ID != sent.ID {
			t.Errorf("ID = %q, want %q", m.ID, sent.ID)
		}
		if m.From != "alice" {
			t.Errorf("From = %q, want %q", m.From, "alice")
		}
		if m.To != "bob" {
			t.Errorf("To = %q, want %q", m.To, "bob")
		}
		if m.Body != "read me" {
			t.Errorf("Body = %q, want %q", m.Body, "read me")
		}
	})

	t.Run("Read_MarksAsRead", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "once")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if _, err := p.Read(sent.ID); err != nil {
			t.Fatalf("Read: %v", err)
		}
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Inbox after Read = %d messages, want 0", len(msgs))
		}
	})

	t.Run("Read_UnknownIDReturnsError", func(t *testing.T) {
		p := newProvider(t)
		_, err := p.Read("nonexistent")
		if err == nil {
			t.Error("Read(nonexistent) should return error")
		}
	})

	// --- Group 5: Archive ---

	t.Run("Archive_RemovesFromInbox", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "archive me")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if err := p.Archive(sent.ID); err != nil {
			t.Fatalf("Archive: %v", err)
		}
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Inbox after Archive = %d messages, want 0", len(msgs))
		}
	})

	t.Run("Archive_AlreadyArchivedReturnsError", func(t *testing.T) {
		p := newProvider(t)
		sent, err := p.Send("alice", "bob", "double archive")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if err := p.Archive(sent.ID); err != nil {
			t.Fatalf("first Archive: %v", err)
		}
		err = p.Archive(sent.ID)
		if !errors.Is(err, mail.ErrAlreadyArchived) {
			t.Errorf("second Archive = %v, want ErrAlreadyArchived", err)
		}
	})

	t.Run("Archive_UnknownIDReturnsError", func(t *testing.T) {
		p := newProvider(t)
		err := p.Archive("nonexistent")
		if err == nil {
			t.Error("Archive(nonexistent) should return error")
		}
	})

	// --- Group 6: Lifecycle ---

	t.Run("Lifecycle_SendInboxReadInboxEmpty", func(t *testing.T) {
		p := newProvider(t)

		// Send.
		sent, err := p.Send("alice", "bob", "lifecycle")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}

		// Inbox shows it.
		msgs, err := p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("Inbox = %d messages, want 1", len(msgs))
		}

		// Read it.
		m, err := p.Read(sent.ID)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if m.Body != "lifecycle" {
			t.Errorf("Body = %q, want %q", m.Body, "lifecycle")
		}

		// Inbox now empty.
		msgs, err = p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox after Read: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Inbox after Read = %d messages, want 0", len(msgs))
		}
	})

	t.Run("Lifecycle_SendCheckArchiveInboxEmpty", func(t *testing.T) {
		p := newProvider(t)

		// Send.
		sent, err := p.Send("alice", "bob", "check-archive")
		if err != nil {
			t.Fatalf("Send: %v", err)
		}

		// Check (doesn't mark read).
		msgs, err := p.Check("bob")
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("Check = %d messages, want 1", len(msgs))
		}

		// Archive.
		if err := p.Archive(sent.ID); err != nil {
			t.Fatalf("Archive: %v", err)
		}

		// Inbox now empty.
		msgs, err = p.Inbox("bob")
		if err != nil {
			t.Fatalf("Inbox after Archive: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("Inbox after Archive = %d messages, want 0", len(msgs))
		}
	})
}
