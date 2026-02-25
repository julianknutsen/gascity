package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/events"
)

func TestDoEventEmitSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	var stderr bytes.Buffer
	code := doEventEmit(path, events.BeadCreated, "gc-1", "Build Tower of Hanoi", "mayor", &stderr)
	if code != 0 {
		t.Fatalf("doEventEmit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	// Verify the event was written.
	evts, err := events.ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Type != events.BeadCreated {
		t.Errorf("Type = %q, want %q", e.Type, events.BeadCreated)
	}
	if e.Subject != "gc-1" {
		t.Errorf("Subject = %q, want %q", e.Subject, "gc-1")
	}
	if e.Message != "Build Tower of Hanoi" {
		t.Errorf("Message = %q, want %q", e.Message, "Build Tower of Hanoi")
	}
	if e.Actor != "mayor" {
		t.Errorf("Actor = %q, want %q", e.Actor, "mayor")
	}
	if e.Seq != 1 {
		t.Errorf("Seq = %d, want 1", e.Seq)
	}
}

func TestDoEventEmitDefaultActor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	var stderr bytes.Buffer
	code := doEventEmit(path, events.BeadClosed, "gc-1", "", "", &stderr)
	if code != 0 {
		t.Fatalf("doEventEmit = %d, want 0", code)
	}

	evts, err := events.ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(evts))
	}
	// Default actor when GC_AGENT is not set.
	if evts[0].Actor != "human" {
		t.Errorf("Actor = %q, want %q", evts[0].Actor, "human")
	}
}

func TestDoEventEmitGCAgentEnv(t *testing.T) {
	t.Setenv("GC_AGENT", "worker")

	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	var stderr bytes.Buffer
	code := doEventEmit(path, events.BeadCreated, "gc-1", "task", "", &stderr)
	if code != 0 {
		t.Fatalf("doEventEmit = %d, want 0", code)
	}

	evts, err := events.ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if evts[0].Actor != "worker" {
		t.Errorf("Actor = %q, want %q (from GC_AGENT)", evts[0].Actor, "worker")
	}
}

func TestDoEventEmitBestEffort(t *testing.T) {
	// Write to an invalid path — should return 0 (best-effort, never fail).
	var stderr bytes.Buffer
	code := doEventEmit("/nonexistent/dir/events.jsonl", events.BeadCreated, "gc-1", "", "", &stderr)
	if code != 0 {
		t.Errorf("doEventEmit = %d, want 0 (best-effort)", code)
	}
}

func TestEventEmitViaCLI(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	t.Setenv("GC_SESSION", "fake")

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"init", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc init = %d; stderr: %s", code, stderr.String())
	}

	// Use --city flag in args (run() creates fresh cobra root, resetting cityFlag).
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", dir, "event", "emit", "bead.created", "--subject", "gc-1", "--message", "Build Hanoi"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc event emit = %d; stderr: %s", code, stderr.String())
	}

	// Verify via gc events.
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", dir, "events"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc events = %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "bead.created") {
		t.Errorf("gc events output missing 'bead.created': %q", out)
	}
	if !strings.Contains(out, "gc-1") {
		t.Errorf("gc events output missing 'gc-1': %q", out)
	}
	if !strings.Contains(out, "Build Hanoi") {
		t.Errorf("gc events output missing 'Build Hanoi': %q", out)
	}
}

func TestEventMissingSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"event"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("gc event = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing subcommand") {
		t.Errorf("stderr = %q, want 'missing subcommand'", stderr.String())
	}
}

func TestEventEmitMissingType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"event", "emit"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("gc event emit = %d, want 1 (missing type arg)", code)
	}
}
