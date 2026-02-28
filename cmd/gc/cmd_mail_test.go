package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/mail/beadmail"
)

// --- gc mail send ---

func TestMailSendSuccess(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout, stderr bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"mayor", "hey, are you still there?"}, nil, &stdout, &stderr)
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
	mp := beadmail.New(store)
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
			code := doMailSend(mp, events.Discard, recipients, "human", tt.args, nil, &bytes.Buffer{}, &stderr)
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
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stderr bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"nobody", "hello"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailSend = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown recipient "nobody"`) {
		t.Errorf("stderr = %q, want unknown recipient error", stderr.String())
	}
}

func TestMailSendToHuman(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "mayor", []string{"human", "task complete"}, nil, &stdout, &bytes.Buffer{})
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
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true, "worker": true}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "worker", []string{"mayor", "found a bug"}, nil, &stdout, &bytes.Buffer{})
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
	mp := beadmail.New(store)

	var stdout, stderr bytes.Buffer
	code := doMailInbox(mp, "mayor", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailInbox = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No unread messages for mayor") {
		t.Errorf("stdout = %q, want no unread message", stdout.String())
	}
}

func TestMailInboxShowsMessages(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "hey there", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Create(beads.Bead{Title: "status?", Type: "message", Assignee: "mayor", From: "worker"})  //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailInbox(mp, "mayor", &stdout, &stderr)
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
	mp := beadmail.New(store)
	// Message to mayor (should appear).
	store.Create(beads.Bead{Title: "for mayor", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	// Message to worker (should not appear in mayor's inbox).
	store.Create(beads.Bead{Title: "for worker", Type: "message", Assignee: "worker", From: "human"}) //nolint:errcheck
	// Task bead (should not appear — wrong type).
	store.Create(beads.Bead{Title: "a task"}) //nolint:errcheck
	// Read message to mayor (should not appear — already closed).
	store.Create(beads.Bead{Title: "already read", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Close("gc-4")                                                                                //nolint:errcheck

	var stdout bytes.Buffer
	code := doMailInbox(mp, "mayor", &stdout, &bytes.Buffer{})
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
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "report", Type: "message", Assignee: "human", From: "mayor"}) //nolint:errcheck

	var stdout bytes.Buffer
	code := doMailInbox(mp, "human", &stdout, &bytes.Buffer{})
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
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "hey, are you still there?", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailRead(mp, events.Discard, []string{"gc-1"}, &stdout, &stderr)
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
	mp := beadmail.New(store)

	var stderr bytes.Buffer
	code := doMailRead(mp, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailRead = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing message ID") {
		t.Errorf("stderr = %q, want 'missing message ID'", stderr.String())
	}
}

func TestMailReadNotFound(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)

	var stderr bytes.Buffer
	code := doMailRead(mp, events.Discard, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailRead = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestMailReadAlreadyRead(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "old news", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Close("gc-1")                                                                            //nolint:errcheck

	// Reading an already-read message should still display it without error.
	var stdout, stderr bytes.Buffer
	code := doMailRead(mp, events.Discard, []string{"gc-1"}, &stdout, &stderr)
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
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "dismiss me", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailArchive(mp, events.Discard, []string{"gc-1"}, &stdout, &stderr)
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
	mp := beadmail.New(store)

	var stderr bytes.Buffer
	code := doMailArchive(mp, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing message ID") {
		t.Errorf("stderr = %q, want 'missing message ID'", stderr.String())
	}
}

func TestMailArchiveNotFound(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)

	var stderr bytes.Buffer
	code := doMailArchive(mp, events.Discard, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestMailArchiveNonMessage(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "a task"}) //nolint:errcheck // Type defaults to "" (task)

	var stderr bytes.Buffer
	code := doMailArchive(mp, events.Discard, []string{"gc-1"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailArchive = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a message") {
		t.Errorf("stderr = %q, want 'not a message'", stderr.String())
	}
}

func TestMailArchiveAlreadyClosed(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "old", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Close("gc-1")                                                                       //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailArchive(mp, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailArchive = %d, want 0; stderr: %s", code, stderr.String())
	}
	// Already-closed messages report as already archived.
	if !strings.Contains(stdout.String(), "Already archived gc-1") {
		t.Errorf("stdout = %q, want 'Already archived'", stdout.String())
	}
}

// --- gc mail send --notify ---

func TestMailSendNotifySuccess(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var nudged string
	nf := func(recipient string) error {
		nudged = recipient
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"mayor", "wake up"}, nf, &stdout, &stderr)
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
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	nf := func(_ string) error {
		return fmt.Errorf("session not found")
	}

	var stdout, stderr bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"mayor", "wake up"}, nf, &stdout, &stderr)
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
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	nudgeCalled := false
	nf := func(_ string) error {
		nudgeCalled = true
		return nil
	}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "mayor", []string{"human", "done"}, nf, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSend = %d, want 0", code)
	}
	if nudgeCalled {
		t.Error("nudgeFn should not be called when recipient is human")
	}
}

func TestMailSendWithoutNotify(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout, stderr bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"mayor", "no nudge"}, nil, &stdout, &stderr)
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
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "deacon", []string{"mayor", "patrol complete"}, nil, &stdout, &bytes.Buffer{})
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
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "witness", []string{"mayor", "health report"}, nil, &stdout, &bytes.Buffer{})
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
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "mayor": true}

	var stdout bytes.Buffer
	code := doMailSend(mp, events.Discard, recipients, "human", []string{"mayor", "hello"}, nil, &stdout, &bytes.Buffer{})
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

// --- gc mail send --all ---

func TestMailSendAll(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "coder": true, "committer": true, "tester": true}

	var stdout, stderr bytes.Buffer
	code := doMailSendAll(mp, events.Discard, recipients, "coder", []string{"status update: tests passing"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doMailSendAll = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	// Should send to committer and tester (not coder/sender, not human).
	if !strings.Contains(out, "Sent message gc-1 to committer") {
		t.Errorf("stdout missing committer send:\n%s", out)
	}
	if !strings.Contains(out, "Sent message gc-2 to tester") {
		t.Errorf("stdout missing tester send:\n%s", out)
	}
	if strings.Contains(out, "to coder") {
		t.Errorf("stdout should not contain send to sender (coder):\n%s", out)
	}
	if strings.Contains(out, "to human") {
		t.Errorf("stdout should not contain send to human:\n%s", out)
	}

	// Verify beads were created for each recipient.
	b1, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b1.Assignee != "committer" {
		t.Errorf("gc-1 Assignee = %q, want %q", b1.Assignee, "committer")
	}
	b2, err := store.Get("gc-2")
	if err != nil {
		t.Fatal(err)
	}
	if b2.Assignee != "tester" {
		t.Errorf("gc-2 Assignee = %q, want %q", b2.Assignee, "tester")
	}
}

func TestMailSendAllMissingBody(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "coder": true}

	var stderr bytes.Buffer
	code := doMailSendAll(mp, events.Discard, recipients, "human", nil, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailSendAll = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr = %q, want usage message", stderr.String())
	}
}

func TestMailSendAllNoRecipients(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	// Only human and sender — no one to broadcast to.
	recipients := map[string]bool{"human": true, "coder": true}

	var stderr bytes.Buffer
	code := doMailSendAll(mp, events.Discard, recipients, "coder", []string{"hello?"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doMailSendAll = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no recipients") {
		t.Errorf("stderr = %q, want 'no recipients'", stderr.String())
	}
}

func TestMailSendAllExcludesSender(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	recipients := map[string]bool{"human": true, "alice": true, "bob": true}

	var stdout bytes.Buffer
	code := doMailSendAll(mp, events.Discard, recipients, "alice", []string{"broadcast"}, nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailSendAll = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "to bob") {
		t.Errorf("stdout missing send to bob:\n%s", out)
	}
	if strings.Contains(out, "to alice") {
		t.Errorf("stdout should not contain send to sender alice:\n%s", out)
	}
}

// --- gc mail check ---

func TestMailCheckNoMail(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)

	var stdout, stderr bytes.Buffer
	code := doMailCheck(mp, "mayor", false, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doMailCheck = %d, want 1 (no mail)", code)
	}
	if stdout.Len() > 0 {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestMailCheckHasMail(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "hey", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Create(beads.Bead{Title: "yo", Type: "message", Assignee: "mayor", From: "worker"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailCheck(mp, "mayor", false, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doMailCheck = %d, want 0 (has mail)", code)
	}
	if !strings.Contains(stdout.String(), "2 unread message(s) for mayor") {
		t.Errorf("stdout = %q, want count message", stdout.String())
	}
}

func TestMailCheckInjectNoMail(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)

	var stdout, stderr bytes.Buffer
	code := doMailCheck(mp, "mayor", true, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doMailCheck = %d, want 0 (--inject always exits 0)", code)
	}
	if stdout.Len() > 0 {
		t.Errorf("unexpected stdout: %q (should be silent when no mail)", stdout.String())
	}
}

func TestMailCheckInjectFormatsMessages(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "Fix the auth bug", Type: "message", Assignee: "worker", From: "mayor"})          //nolint:errcheck
	store.Create(beads.Bead{Title: "PR #17 ready for review", Type: "message", Assignee: "worker", From: "polecat"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doMailCheck(mp, "worker", true, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doMailCheck = %d, want 0", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "<system-reminder>") {
		t.Errorf("stdout missing <system-reminder> tag:\n%s", out)
	}
	if !strings.Contains(out, "</system-reminder>") {
		t.Errorf("stdout missing </system-reminder> tag:\n%s", out)
	}
	if !strings.Contains(out, "2 unread message(s)") {
		t.Errorf("stdout missing message count:\n%s", out)
	}
	if !strings.Contains(out, "gc-1 from mayor: Fix the auth bug") {
		t.Errorf("stdout missing first message:\n%s", out)
	}
	if !strings.Contains(out, "gc-2 from polecat: PR #17 ready for review") {
		t.Errorf("stdout missing second message:\n%s", out)
	}
	if !strings.Contains(out, "gc mail read <id>") {
		t.Errorf("stdout missing read hint:\n%s", out)
	}
}

func TestMailCheckInjectDoesNotCloseBeads(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	store.Create(beads.Bead{Title: "still open", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck

	var stdout bytes.Buffer
	code := doMailCheck(mp, "mayor", true, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailCheck = %d, want 0", code)
	}

	// Bead must remain open after injection.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q (inject must not close beads)", b.Status, "open")
	}
}

func TestMailCheckInjectFiltersCorrectly(t *testing.T) {
	store := beads.NewMemStore()
	mp := beadmail.New(store)
	// Message to mayor (should appear).
	store.Create(beads.Bead{Title: "for mayor", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	// Message to worker (should not appear in mayor's check).
	store.Create(beads.Bead{Title: "for worker", Type: "message", Assignee: "worker", From: "human"}) //nolint:errcheck
	// Task bead (should not appear — wrong type).
	store.Create(beads.Bead{Title: "a task"}) //nolint:errcheck
	// Closed message to mayor (should not appear).
	store.Create(beads.Bead{Title: "already read", Type: "message", Assignee: "mayor", From: "human"}) //nolint:errcheck
	store.Close("gc-4")                                                                                //nolint:errcheck

	var stdout bytes.Buffer
	code := doMailCheck(mp, "mayor", true, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doMailCheck = %d, want 0", code)
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
	if !strings.Contains(out, "1 unread message(s)") {
		t.Errorf("stdout missing correct count:\n%s", out)
	}
}
