package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/events"
)

func TestEventsEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doEvents("/nonexistent/events.jsonl", "", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEvents = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No events.") {
		t.Errorf("stdout = %q, want 'No events.'", stdout.String())
	}
}

func TestEventsShowsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human", Subject: "gc-1", Message: "Build Tower of Hanoi"})
	rec.Record(events.Event{Type: events.AgentStarted, Actor: "gc", Subject: "mayor", Message: "gc-bright-lights-mayor"})
	rec.Close() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	code := doEvents(path, "", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEvents = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"SEQ", "TYPE", "ACTOR", "SUBJECT", "MESSAGE", "TIME",
		"1", "bead.created", "human", "gc-1", "Build Tower of Hanoi",
		"2", "agent.started", "gc", "mayor",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestEventsFilterByType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human", Subject: "gc-1"})
	rec.Record(events.Event{Type: events.BeadClosed, Actor: "human", Subject: "gc-1"})
	rec.Record(events.Event{Type: events.AgentStarted, Actor: "gc", Subject: "mayor"})
	rec.Close() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	code := doEvents(path, events.BeadCreated, "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEvents = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bead.created") {
		t.Errorf("stdout missing 'bead.created': %q", out)
	}
	if strings.Contains(out, "bead.closed") {
		t.Errorf("stdout should not contain 'bead.closed': %q", out)
	}
	if strings.Contains(out, "agent.started") {
		t.Errorf("stdout should not contain 'agent.started': %q", out)
	}
}

func TestEventsFilterBySince(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human", Subject: "gc-1", Ts: old})
	rec.Record(events.Event{Type: events.AgentStarted, Actor: "gc", Subject: "mayor"})
	rec.Close() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	code := doEvents(path, "", "1h", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEvents = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, "bead.created") {
		t.Errorf("stdout should not contain old event: %q", out)
	}
	if !strings.Contains(out, "agent.started") {
		t.Errorf("stdout missing recent event: %q", out)
	}
}

func TestEventsInvalidSince(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doEvents("/nonexistent/events.jsonl", "", "notaduration", &stdout, &stderr)
	if code != 1 {
		t.Errorf("doEvents = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid --since") {
		t.Errorf("stderr = %q, want 'invalid --since'", stderr.String())
	}
}
