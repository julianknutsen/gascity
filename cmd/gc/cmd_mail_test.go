package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

// --- gc mail send ---

func TestMailSendSuccess(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout, stderr bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "human", []string{"mayor", "hey, are you still there?"}, nil, &stdout, &stderr)
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
			code := doMailSend(store, events.Discard, recipients, "human", tt.args, nil, &bytes.Buffer{}, &stderr)
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
	code := doMailSend(store, events.Discard, recipients, "human", []string{"nobody", "hello"}, nil, &bytes.Buffer{}, &stderr)
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
	code := doMailSend(store, events.Discard, recipients, "mayor", []string{"human", "task complete"}, nil, &stdout, &bytes.Buffer{})
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
	code := doMailSend(store, events.Discard, recipients, "worker", []string{"mayor", "found a bug"}, nil, &stdout, &bytes.Buffer{})
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
	code := doMailRead(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
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
	code := doMailRead(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
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
	code := doMailRead(store, events.Discard, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
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
	code := doMailRead(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailRead = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "old news") {
		t.Errorf("stdout = %q, want 'old news'", stdout.String())
	}
}

// --- gc mail archive ---

func TestMailArchiveSuccess(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "dismiss me", Type: "message", Assignee: "mayor", From: "human"})

	var stdout, stderr bytes.Buffer
	code := doMailArchive(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailArchive = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Archived message gc-1") {
		t.Errorf("stdout = %q, want archived confirmation", stdout.String())
	}

	// Verify bead is now closed.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q", b.Status, "closed")
	}
}

func TestMailArchiveMissingID(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doMailArchive(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing message ID") {
		t.Errorf("stderr = %q, want 'missing message ID'", stderr.String())
	}
}

func TestMailArchiveNotFound(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doMailArchive(store, events.Discard, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestMailArchiveNonMessage(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "a task"}) // Type defaults to "" (task)

	var stderr bytes.Buffer
	code := doMailArchive(store, events.Discard, []string{"gc-1"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a message") {
		t.Errorf("stderr = %q, want 'not a message'", stderr.String())
	}
}

func TestMailArchiveAlreadyClosed(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "old", Type: "message", Assignee: "mayor", From: "human"})
	_ = store.Close("gc-1")

	var stdout, stderr bytes.Buffer
	code := doMailArchive(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailArchive = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Already archived gc-1") {
		t.Errorf("stdout = %q, want 'Already archived'", stdout.String())
	}
}

// --- gc mail send --notify ---

func TestMailSendNotifySuccess(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var nudged string
	nf := func(recipient string) error {
		nudged = recipient
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "human", []string{"mayor", "wake up"}, nf, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sent message gc-1 to mayor") {
		t.Errorf("stdout = %q, want sent confirmation", stdout.String())
	}
	if nudged != "mayor" {
		t.Errorf("nudgeFn called with %q, want %q", nudged, "mayor")
	}
}

func TestMailSendNotifyNudgeError(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	nf := func(_ string) error {
		return fmt.Errorf("session not found")
	}

	var stdout, stderr bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "human", []string{"mayor", "wake up"}, nf, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0 (nudge failure is non-fatal); stderr: %s", code, stderr.String())
	}
	// Mail should still be sent.
	if !strings.Contains(stdout.String(), "Sent message gc-1 to mayor") {
		t.Errorf("stdout = %q, want sent confirmation", stdout.String())
	}
	// Warning should appear on stderr.
	if !strings.Contains(stderr.String(), "nudge failed") {
		t.Errorf("stderr = %q, want nudge failure warning", stderr.String())
	}
}

func TestMailSendNotifyToHuman(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	nudgeCalled := false
	nf := func(_ string) error {
		nudgeCalled = true
		return nil
	}

	var stdout bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "mayor", []string{"human", "done"}, nf, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}
	if nudgeCalled {
		t.Error("nudgeFn should not be called when recipient is human")
	}
}

func TestMailSendWithoutNotify(t *testing.T) {
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout, stderr bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "human", []string{"mayor", "no nudge"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sent message gc-1 to mayor") {
		t.Errorf("stdout = %q, want sent confirmation", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
}

// --- gc mail send --from ---

func TestMailSendFromFlag(t *testing.T) {
	// --from sets the sender field on the created bead.
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "deacon", []string{"mayor", "patrol complete"}, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.From != "deacon" {
		t.Errorf("bead From = %q, want %q", b.From, "deacon")
	}
}

func TestMailSendFromFlagOverridesEnv(t *testing.T) {
	// The --from flag value is passed as the sender parameter to doMailSend,
	// which takes priority over any env-var-based resolution done upstream
	// in cmdMailSend. We verify the sender parameter is used as-is.
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	// Simulate: --from=witness but GC_AGENT would have been "polecat-1".
	// cmdMailSend resolves: from > GC_AGENT > "human". By the time doMailSend
	// is called, sender is already resolved. We just verify the final sender.
	var stdout bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "witness", []string{"mayor", "health report"}, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.From != "witness" {
		t.Errorf("bead From = %q, want %q (--from should override env)", b.From, "witness")
	}
}

func TestMailSendFromDefault(t *testing.T) {
	// Without --from and without GC_AGENT, sender defaults to "human".
	// doMailSend receives the already-resolved sender from cmdMailSend.
	store := beads.NewMemStore()
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(store, events.Discard, recipients, "human", []string{"mayor", "hello"}, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.From != "human" {
		t.Errorf("bead From = %q, want %q (default when no --from and no GC_AGENT)", b.From, "human")
	}
}
