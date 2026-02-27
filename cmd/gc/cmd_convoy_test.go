package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

// --- gc convoy create ---

func TestConvoyCreate(t *testing.T) {
	store := beads.NewMemStore()

	var stdout, stderr bytes.Buffer
	code := doConvoyCreate(store, events.Discard, []string{"deploy v2.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyCreate = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `Created convoy gc-1 "deploy v2.0"`) {
		t.Errorf("stdout = %q, want convoy creation confirmation", stdout.String())
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Type != "convoy" {
		t.Errorf("bead Type = %q, want %q", b.Type, "convoy")
	}
	if b.Title != "deploy v2.0" {
		t.Errorf("bead Title = %q, want %q", b.Title, "deploy v2.0")
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q", b.Status, "open")
	}
}

func TestConvoyCreateWithIssues(t *testing.T) {
	store := beads.NewMemStore()
	// Pre-create issues.
	_, _ = store.Create(beads.Bead{Title: "fix auth"})    // gc-1
	_, _ = store.Create(beads.Bead{Title: "fix logging"}) // gc-2

	var stdout, stderr bytes.Buffer
	code := doConvoyCreate(store, events.Discard, []string{"security fixes", "gc-1", "gc-2"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyCreate = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "tracking 2 issue(s)") {
		t.Errorf("stdout = %q, want tracking count", stdout.String())
	}

	// Verify issues have convoy as parent.
	for _, id := range []string{"gc-1", "gc-2"} {
		b, err := store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if b.ParentID != "gc-3" {
			t.Errorf("bead %s ParentID = %q, want %q", id, b.ParentID, "gc-3")
		}
	}
}

func TestConvoyCreateMissingName(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doConvoyCreate(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyCreate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing convoy name") {
		t.Errorf("stderr = %q, want missing name error", stderr.String())
	}
}

func TestConvoyCreateBadIssueID(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doConvoyCreate(store, events.Discard, []string{"batch", "gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyCreate = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want not found error", stderr.String())
	}
}

// --- gc convoy list ---

func TestConvoyList(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch 1", Type: "convoy"}) // gc-1
	_, _ = store.Create(beads.Bead{Title: "fix auth", ParentID: "gc-1"})
	_, _ = store.Create(beads.Bead{Title: "fix logs", ParentID: "gc-1"})
	_ = store.Close("gc-3") // close one child

	var stdout, stderr bytes.Buffer
	code := doConvoyList(store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyList = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"ID", "TITLE", "PROGRESS", "gc-1", "batch 1", "1/2 closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestConvoyListEmpty(t *testing.T) {
	store := beads.NewMemStore()

	var stdout, stderr bytes.Buffer
	code := doConvoyList(store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No open convoys") {
		t.Errorf("stdout = %q, want no open convoys message", stdout.String())
	}
}

func TestConvoyListExcludesClosed(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "done batch", Type: "convoy"})
	_ = store.Close("gc-1")

	var stdout bytes.Buffer
	code := doConvoyList(store, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doConvoyList = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No open convoys") {
		t.Errorf("stdout = %q, want no open convoys (closed convoy excluded)", stdout.String())
	}
}

// --- gc convoy status ---

func TestConvoyStatus(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "deploy", Type: "convoy"})                       // gc-1
	_, _ = store.Create(beads.Bead{Title: "task A", ParentID: "gc-1"})                     // gc-2
	_, _ = store.Create(beads.Bead{Title: "task B", ParentID: "gc-1", Assignee: "worker"}) // gc-3
	_ = store.Close("gc-2")

	var stdout, stderr bytes.Buffer
	code := doConvoyStatus(store, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyStatus = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"Convoy:   gc-1",
		"Title:    deploy",
		"Status:   open",
		"1/2 closed",
		"task A", "closed",
		"task B", "worker",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestConvoyStatusNotConvoy(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "just a task"}) // type=task

	var stderr bytes.Buffer
	code := doConvoyStatus(store, []string{"gc-1"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyStatus = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a convoy") {
		t.Errorf("stderr = %q, want 'not a convoy'", stderr.String())
	}
}

func TestConvoyStatusMissingID(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doConvoyStatus(store, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyStatus = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing convoy ID") {
		t.Errorf("stderr = %q, want missing ID error", stderr.String())
	}
}

// --- gc convoy add ---

func TestConvoyAdd(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"}) // gc-1
	_, _ = store.Create(beads.Bead{Title: "task A"})                // gc-2

	var stdout, stderr bytes.Buffer
	code := doConvoyAdd(store, []string{"gc-1", "gc-2"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyAdd = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Added gc-2 to convoy gc-1") {
		t.Errorf("stdout = %q, want add confirmation", stdout.String())
	}

	b, err := store.Get("gc-2")
	if err != nil {
		t.Fatal(err)
	}
	if b.ParentID != "gc-1" {
		t.Errorf("bead ParentID = %q, want %q", b.ParentID, "gc-1")
	}
}

func TestConvoyAddNotConvoy(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "just a task"}) // type=task
	_, _ = store.Create(beads.Bead{Title: "another"})

	var stderr bytes.Buffer
	code := doConvoyAdd(store, []string{"gc-1", "gc-2"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a convoy") {
		t.Errorf("stderr = %q, want 'not a convoy'", stderr.String())
	}
}

func TestConvoyAddMissingArgs(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doConvoyAdd(store, []string{"gc-1"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr = %q, want usage message", stderr.String())
	}
}

// --- gc convoy close ---

func TestConvoyClose(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})

	var stdout, stderr bytes.Buffer
	code := doConvoyClose(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyClose = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Closed convoy gc-1") {
		t.Errorf("stdout = %q, want close confirmation", stdout.String())
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q", b.Status, "closed")
	}
}

func TestConvoyCloseNotConvoy(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "a task"})

	var stderr bytes.Buffer
	code := doConvoyClose(store, events.Discard, []string{"gc-1"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyClose = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not a convoy") {
		t.Errorf("stderr = %q, want 'not a convoy'", stderr.String())
	}
}

func TestConvoyCloseMissingID(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doConvoyClose(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doConvoyClose = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing convoy ID") {
		t.Errorf("stderr = %q, want missing ID error", stderr.String())
	}
}

// --- gc convoy check ---

func TestConvoyCheck(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})    // gc-1
	_, _ = store.Create(beads.Bead{Title: "task A", ParentID: "gc-1"}) // gc-2
	_, _ = store.Create(beads.Bead{Title: "task B", ParentID: "gc-1"}) // gc-3
	_ = store.Close("gc-2")
	_ = store.Close("gc-3")

	var stdout, stderr bytes.Buffer
	code := doConvoyCheck(store, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyCheck = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `Auto-closed convoy gc-1 "batch"`) {
		t.Errorf("stdout missing auto-close message:\n%s", out)
	}
	if !strings.Contains(out, "1 convoy(s) auto-closed") {
		t.Errorf("stdout missing summary:\n%s", out)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "closed" {
		t.Errorf("bead Status = %q, want %q", b.Status, "closed")
	}
}

func TestConvoyCheckPartial(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})    // gc-1
	_, _ = store.Create(beads.Bead{Title: "task A", ParentID: "gc-1"}) // gc-2
	_, _ = store.Create(beads.Bead{Title: "task B", ParentID: "gc-1"}) // gc-3
	_ = store.Close("gc-2")                                            // only one closed

	var stdout, stderr bytes.Buffer
	code := doConvoyCheck(store, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyCheck = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, "Auto-closed") {
		t.Errorf("stdout should not contain Auto-closed (partial completion):\n%s", out)
	}
	if !strings.Contains(out, "0 convoy(s) auto-closed") {
		t.Errorf("stdout missing zero summary:\n%s", out)
	}

	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "open" {
		t.Errorf("bead Status = %q, want %q (should stay open)", b.Status, "open")
	}
}

func TestConvoyCheckEmpty(t *testing.T) {
	store := beads.NewMemStore()
	// Convoy with no children should not be auto-closed.
	_, _ = store.Create(beads.Bead{Title: "empty batch", Type: "convoy"})

	var stdout bytes.Buffer
	code := doConvoyCheck(store, events.Discard, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doConvoyCheck = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "0 convoy(s) auto-closed") {
		t.Errorf("stdout = %q, want zero summary (empty convoy not auto-closed)", stdout.String())
	}
}

// --- gc convoy stranded ---

func TestConvoyStranded(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})                          // gc-1
	_, _ = store.Create(beads.Bead{Title: "assigned", ParentID: "gc-1", Assignee: "worker"}) // gc-2 — has worker
	_, _ = store.Create(beads.Bead{Title: "unassigned", ParentID: "gc-1"})                   // gc-3 — stranded

	var stdout, stderr bytes.Buffer
	code := doConvoyStranded(store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doConvoyStranded = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "gc-3") {
		t.Errorf("stdout missing stranded issue gc-3:\n%s", out)
	}
	if !strings.Contains(out, "unassigned") {
		t.Errorf("stdout missing stranded issue title:\n%s", out)
	}
	// Assigned issue should not appear as stranded.
	if strings.Contains(out, "assigned\t") && !strings.Contains(out, "unassigned") {
		t.Errorf("stdout should not show assigned issues as stranded:\n%s", out)
	}
}

func TestConvoyStrandedNone(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})
	_, _ = store.Create(beads.Bead{Title: "done", ParentID: "gc-1", Assignee: "worker"})

	var stdout bytes.Buffer
	code := doConvoyStranded(store, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doConvoyStranded = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No stranded work") {
		t.Errorf("stdout = %q, want no stranded message", stdout.String())
	}
}

func TestConvoyStrandedClosedExcluded(t *testing.T) {
	store := beads.NewMemStore()
	_, _ = store.Create(beads.Bead{Title: "batch", Type: "convoy"})
	_, _ = store.Create(beads.Bead{Title: "done task", ParentID: "gc-1"}) // no assignee but closed
	_ = store.Close("gc-2")

	var stdout bytes.Buffer
	code := doConvoyStranded(store, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("doConvoyStranded = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No stranded work") {
		t.Errorf("stdout = %q, want no stranded (closed issues excluded)", stdout.String())
	}
}
