package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
)

// --- gc mail send ---

func TestMailSendSuccess(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout, stderr bytes.Buffer
	code := doMailSend(store, recipients, "human", []string{"mayor", "hey, are you still there?"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sent message gc-1 to mayor") {
		t.Errorf("stdout = %q, want sent confirmation", stdout.String())
	}

	// Verify the bead was created correctly.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Type != "message" {
		t.Errorf("bead Type = %q, want %q", b.Type, "message")
	}
	if b.Assignee != "mayor" {
		t.Errorf("bead Assignee = %q, want %q", b.Assignee, "mayor")
	}
	if b.From != "human" {
		t.Errorf("bead From = %q, want %q", b.From, "human")
	}
	if b.Title != "hey, are you still there?" {
		t.Errorf("bead Title = %q, want message body", b.Title)
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q", b.Status, "open")
	}
}

func TestMailSendMissingArgs(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true}

	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"only recipient", []string{"mayor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := doMailSend(store, recipients, "human", tt.args, &bytes.Buffer{}, &stderr)
			if code != 1 {
				t.Errorf("doMailSend = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), "usage:") {
				t.Errorf("stderr = %q, want usage message", stderr.String())
			}
		})
	}
}

func TestMailSendInvalidRecipient(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stderr bytes.Buffer
	code := doMailSend(store, recipients, "human", []string{"nobody", "hello"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailSend = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown recipient "nobody"`) {
		t.Errorf("stderr = %q, want unknown recipient error", stderr.String())
	}
}

func TestMailSendToHuman(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(store, recipients, "mayor", []string{"human", "task complete"}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Assignee != "human" {
		t.Errorf("bead Assignee = %q, want %q", b.Assignee, "human")
	}
	if b.From != "mayor" {
		t.Errorf("bead From = %q, want %q", b.From, "mayor")
	}
}

func TestMailSendAgentToAgent(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true, "worker": true}

	var stdout bytes.Buffer
	code := doMailSend(store, recipients, "worker", []string{"mayor", "found a bug"}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.From != "worker" {
		t.Errorf("bead From = %q, want %q", b.From, "worker")
	}
	if b.Assignee != "mayor" {
		t.Errorf("bead Assignee = %q, want %q", b.Assignee, "mayor")
	}
}

// --- gc mail inbox ---

func TestMailInboxEmpty(t *testing.T) {
	store := beads.NewMemStore()

	var stdout, stderr bytes.Buffer
	code := doMailInbox(store, "mayor", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailInbox = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No unread messages for mayor") {
		t.Errorf("stdout = %q, want no unread message", stdout.String())
	}
}

func TestMailInboxShowsMessages(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "hey there", Type: "message", Assignee: "mayor", From: "human"})
	_, _ = store.Create(beads.Bead{Title: "status?", Type: "message", Assignee: "mayor", From: "worker"})

	var stdout, stderr bytes.Buffer
	code := doMailInbox(store, "mayor", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailInbox = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"ID", "FROM", "BODY", "gc-1", "human", "hey there", "gc-2", "worker", "status?"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestMailInboxFiltersCorrectly(t *testing.T) {
	store := beads.NewMemStore()
	// Message to mayor (should appear).
	_, _ = store.Create(beads.Bead{Title: "for mayor", Type: "message", Assignee: "mayor", From: "human"})
	// Message to worker (should not appear in mayor's inbox).
	_, _ = store.Create(beads.Bead{Title: "for worker", Type: "message", Assignee: "worker", From: "human"})
	// Task bead (should not appear — wrong type).
	_, _ = store.Create(beads.Bead{Title: "a task"})
	// Read message to mayor (should not appear — already closed).
	_, _ = store.Create(beads.Bead{Title: "already read", Type: "message", Assignee: "mayor", From: "human"})
	_ = store.Close("gc-4")

	var stdout bytes.Buffer
	code := doMailInbox(store, "mayor", &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailInbox = %d, want 0", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "for mayor") {
		t.Errorf("stdout missing 'for mayor': %q", out)
	}
	if strings.Contains(out, "for worker") {
		t.Errorf("stdout should not contain 'for worker': %q", out)
	}
	if strings.Contains(out, "a task") {
		t.Errorf("stdout should not contain 'a task': %q", out)
	}
	if strings.Contains(out, "already read") {
		t.Errorf("stdout should not contain 'already read': %q", out)
	}
}

func TestMailInboxDefaultsToHuman(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "report", Type: "message", Assignee: "human", From: "mayor"})

	var stdout bytes.Buffer
	code := doMailInbox(store, "human", &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailInbox = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "report") {
		t.Errorf("stdout = %q, want 'report'", stdout.String())
	}
}

// --- gc mail read ---

func TestMailReadSuccess(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "hey, are you still there?", Type: "message", Assignee: "mayor", From: "human"})

	var stdout, stderr bytes.Buffer
	code := doMailRead(store, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailRead = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ID:     gc-1",
		"From:   human",
		"To:     mayor",
		"Body:   hey, are you still there?",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}

	// Verify bead is now closed (marked as read).
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q (marked as read)", b.Status, "closed")
	}
}

func TestMailReadMissingID(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doMailRead(store, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailRead = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing message ID") {
		t.Errorf("stderr = %q, want 'missing message ID'", stderr.String())
	}
}

func TestMailReadNotFound(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doMailRead(store, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailRead = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestMailReadAlreadyRead(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "old news", Type: "message", Assignee: "mayor", From: "human"})
	_ = store.Close("gc-1")

	// Reading an already-read message should still display it without error.
	var stdout, stderr bytes.Buffer
	code := doMailRead(store, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailRead = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "old news") {
		t.Errorf("stdout = %q, want 'old news'", stdout.String())
	}
}
