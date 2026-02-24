package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
)

func TestParseBeadFormat(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantFmt  string
		wantRest []string
	}{
		{"nil args", nil, "text", nil},
		{"empty args", []string{}, "text", nil},
		{"no format flag", []string{"gc-1"}, "text", []string{"gc-1"}},
		{"--format json", []string{"--format", "json", "gc-1"}, "json", []string{"gc-1"}},
		{"--format toon", []string{"--format", "toon"}, "toon", nil},
		{"--format=json", []string{"--format=json", "gc-1"}, "json", []string{"gc-1"}},
		{"--format=toon", []string{"--format=toon"}, "toon", nil},
		{"--format text", []string{"--format", "text"}, "text", nil},
		{"--json shorthand", []string{"--json", "gc-1"}, "json", []string{"gc-1"}},
		{"format after positional", []string{"gc-1", "--format", "json"}, "json", []string{"gc-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFmt, gotRest := parseBeadFormat(tt.args)
			if gotFmt != tt.wantFmt {
				t.Errorf("format = %q, want %q", gotFmt, tt.wantFmt)
			}
			if len(gotRest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", gotRest, tt.wantRest)
			} else {
				for i := range gotRest {
					if gotRest[i] != tt.wantRest[i] {
						t.Errorf("rest[%d] = %q, want %q", i, gotRest[i], tt.wantRest[i])
					}
				}
			}
		})
	}
}

func TestToonVal(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"has,comma", `"has,comma"`},
		{`has"quote`, `"has""quote"`},
		{"has\nnewline", `"has` + "\n" + `newline"`},
		{"", ""},
	}
	for _, tt := range tests {
		got := toonVal(tt.in)
		if got != tt.want {
			t.Errorf("toonVal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBeadShowJSON(t *testing.T) {
	store := beads.NewMemStore()
	b, err := store.Create(beads.Bead{Title: "Test bead"})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadShow(store, []string{"--format", "json", b.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadShow --format json = %d, want 0; stderr: %s", code, stderr.String())
	}

	var parsed beads.Bead
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if parsed.ID != b.ID {
		t.Errorf("ID = %q, want %q", parsed.ID, b.ID)
	}
	if parsed.Title != "Test bead" {
		t.Errorf("Title = %q, want %q", parsed.Title, "Test bead")
	}
}

func TestBeadShowTOON(t *testing.T) {
	store := beads.NewMemStore()
	b, err := store.Create(beads.Bead{Title: "Test bead"})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadShow(store, []string{"--format", "toon", b.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadShow --format toon = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "[1]{id,title,status,type,created_at,assignee}:") {
		t.Errorf("TOON header missing, got: %s", out)
	}
	if !strings.Contains(out, b.ID) {
		t.Errorf("TOON body missing bead ID %q, got: %s", b.ID, out)
	}
}

func TestBeadListJSON(t *testing.T) {
	store := beads.NewMemStore()
	store.Create(beads.Bead{Title: "First"})  //nolint:errcheck
	store.Create(beads.Bead{Title: "Second"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doBeadList(store, []string{"--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadList --format json = %d, want 0; stderr: %s", code, stderr.String())
	}

	var parsed []beads.Bead
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if len(parsed) != 2 {
		t.Errorf("got %d beads, want 2", len(parsed))
	}
}

func TestBeadListTOON(t *testing.T) {
	store := beads.NewMemStore()
	store.Create(beads.Bead{Title: "First"})  //nolint:errcheck
	store.Create(beads.Bead{Title: "Second"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doBeadList(store, []string{"--format", "toon"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadList --format toon = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "[2]{id,status,assignee,title}:") {
		t.Errorf("TOON header wrong, got: %s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Errorf("got %d lines, want 3: %s", len(lines), out)
	}
}

func TestBeadReadyJSON(t *testing.T) {
	store := beads.NewMemStore()
	store.Create(beads.Bead{Title: "Ready one"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doBeadReady(store, []string{"--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadReady --json = %d, want 0; stderr: %s", code, stderr.String())
	}

	var parsed []beads.Bead
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if len(parsed) != 1 {
		t.Errorf("got %d beads, want 1", len(parsed))
	}
}

func TestBeadReadyTOON(t *testing.T) {
	store := beads.NewMemStore()
	store.Create(beads.Bead{Title: "Ready one"}) //nolint:errcheck
	store.Create(beads.Bead{Title: "Ready two"}) //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doBeadReady(store, []string{"--format=toon"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadReady --format toon = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "[2]{id,status,title}:") {
		t.Errorf("TOON header wrong, got: %s", out)
	}
}

func TestAgentClaimedJSON(t *testing.T) {
	store := beads.NewMemStore()
	b, _ := store.Create(beads.Bead{Title: "Claimed work"})
	store.Claim(b.ID, "worker") //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doAgentClaimed(store, []string{"--format", "json", "worker"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentClaimed --format json = %d, want 0; stderr: %s", code, stderr.String())
	}

	var parsed beads.Bead
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if parsed.ID != b.ID {
		t.Errorf("ID = %q, want %q", parsed.ID, b.ID)
	}
	if parsed.Assignee != "worker" {
		t.Errorf("Assignee = %q, want %q", parsed.Assignee, "worker")
	}
}

func TestAgentClaimedTOON(t *testing.T) {
	store := beads.NewMemStore()
	b, _ := store.Create(beads.Bead{Title: "Claimed work"})
	store.Claim(b.ID, "worker") //nolint:errcheck

	var stdout, stderr bytes.Buffer
	code := doAgentClaimed(store, []string{"--format=toon", "worker"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentClaimed --format toon = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "[1]{id,title,status,type,created_at,assignee}:") {
		t.Errorf("TOON header missing, got: %s", out)
	}
	if !strings.Contains(out, "worker") {
		t.Errorf("TOON body missing assignee 'worker', got: %s", out)
	}
}

func TestBeadCloseStripsFormatFlag(t *testing.T) {
	store := beads.NewMemStore()
	b, _ := store.Create(beads.Bead{Title: "Close me"})

	var stdout, stderr bytes.Buffer
	code := doBeadClose(store, events.Discard, []string{"--format", "json", b.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadClose with --format = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Closed bead") {
		t.Errorf("stdout = %q, want 'Closed bead'", stdout.String())
	}
}

func TestBeadCreateStripsFormatFlag(t *testing.T) {
	store := beads.NewMemStore()

	var stdout, stderr bytes.Buffer
	code := doBeadCreate(store, events.Discard, []string{"--json", "My title"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadCreate with --json = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Created bead") {
		t.Errorf("stdout = %q, want 'Created bead'", stdout.String())
	}
}
