package main

import (
	"bytes"
	"encoding/json"
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

// --- Seq mode tests ---

func TestDoEventsSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"})
	rec.Record(events.Event{Type: events.BeadClosed, Actor: "human"})
	rec.Record(events.Event{Type: events.AgentStarted, Actor: "gc"})
	rec.Close() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	code := doEventsSeq(path, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsSeq = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "3" {
		t.Errorf("seq = %q, want 3", strings.TrimSpace(stdout.String()))
	}
}

func TestDoEventsSeqEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doEventsSeq("/nonexistent/events.jsonl", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsSeq = %d, want 0; stderr: %s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "0" {
		t.Errorf("seq = %q, want 0", strings.TrimSpace(stdout.String()))
	}
}

// --- Watch mode tests ---

func TestDoEventsWatchImmediate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human", Subject: "gc-1"})
	rec.Record(events.Event{Type: events.BeadClosed, Actor: "human", Subject: "gc-1"})
	rec.Close() //nolint:errcheck // test cleanup

	// afterSeq=0 means "current head" (seq 2), but we set afterSeq=0
	// explicitly and the function will read head as 2.
	// To test "already past", use afterSeq=1 so seq 2 is already there.
	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, "", nil, 1, 100*time.Millisecond, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if out == "" {
		t.Fatal("expected JSON output, got empty")
	}

	// Parse the output JSON line.
	var e events.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &e); err != nil {
		t.Fatalf("unmarshal output: %v; output: %q", err, out)
	}
	if e.Seq != 2 {
		t.Errorf("Seq = %d, want 2", e.Seq)
	}
	if e.Type != events.BeadClosed {
		t.Errorf("Type = %q, want %q", e.Type, events.BeadClosed)
	}
}

func TestDoEventsWatchTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// No events file — should timeout with empty output.

	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, "", nil, 0, 50*time.Millisecond, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("expected empty stdout on timeout, got %q", stdout.String())
	}
}

func TestDoEventsWatchTypeFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"}) // seq 1
	rec.Close()                                                        //nolint:errcheck // test cleanup

	// Watch for bead.closed after seq 0. A goroutine will append it.
	go func() {
		time.Sleep(30 * time.Millisecond)
		rec2, err := events.NewFileRecorder(path, &stderrBuf)
		if err != nil {
			return
		}
		rec2.Record(events.Event{Type: events.BeadCreated, Actor: "human"}) // seq 2 — not matching
		rec2.Record(events.Event{Type: events.BeadClosed, Actor: "human"})  // seq 3 — matching
		rec2.Close()                                                        //nolint:errcheck // test cleanup
	}()

	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, events.BeadClosed, nil, 0, 2*time.Second, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bead.closed") {
		t.Errorf("output missing 'bead.closed': %q", out)
	}
	if strings.Contains(out, "bead.created") {
		t.Errorf("output should not contain 'bead.created': %q", out)
	}
}

func TestDoEventsWatchAfterSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"})
	}
	rec.Close() //nolint:errcheck // test cleanup

	// Watch with explicit afterSeq=3 — should return seq 4 and 5.
	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, "", nil, 3, 100*time.Millisecond, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2; output: %q", len(lines), stdout.String())
	}

	var e1, e2 events.Event
	if err := json.Unmarshal([]byte(lines[0]), &e1); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if e1.Seq != 4 {
		t.Errorf("line 0 Seq = %d, want 4", e1.Seq)
	}
	if e2.Seq != 5 {
		t.Errorf("line 1 Seq = %d, want 5", e2.Seq)
	}
}

func TestDoEventsWatchDefaultAfterSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"}) // seq 1
	rec.Close()                                                        //nolint:errcheck // test cleanup

	// afterSeq=0 means "current head" (seq 1). A goroutine appends after delay.
	go func() {
		time.Sleep(30 * time.Millisecond)
		rec2, err := events.NewFileRecorder(path, &stderrBuf)
		if err != nil {
			return
		}
		rec2.Record(events.Event{Type: events.AgentStarted, Actor: "gc", Subject: "mayor"}) // seq 2
		rec2.Close()                                                                        //nolint:errcheck // test cleanup
	}()

	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, "", nil, 0, 2*time.Second, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	// Should only contain the new event (seq 2), not the existing one (seq 1).
	if !strings.Contains(out, "agent.started") {
		t.Errorf("output missing 'agent.started': %q", out)
	}

	var e events.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &e); err != nil {
		t.Fatalf("unmarshal: %v; output: %q", err, out)
	}
	if e.Seq != 2 {
		t.Errorf("Seq = %d, want 2", e.Seq)
	}
}

func TestDoEventsWatchNoTypeFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"}) // seq 1
	rec.Close()                                                        //nolint:errcheck // test cleanup

	// Watch with no type filter. Append mixed event types after delay.
	go func() {
		time.Sleep(30 * time.Millisecond)
		rec2, err := events.NewFileRecorder(path, &stderrBuf)
		if err != nil {
			return
		}
		rec2.Record(events.Event{Type: events.BeadClosed, Actor: "human"}) // seq 2
		rec2.Record(events.Event{Type: events.AgentStarted, Actor: "gc"})  // seq 3
		rec2.Close()                                                       //nolint:errcheck // test cleanup
	}()

	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, "", nil, 0, 2*time.Second, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2; output: %q", len(lines), out)
	}
	// Both event types should be present (no type filter).
	if !strings.Contains(out, "bead.closed") {
		t.Errorf("output missing 'bead.closed': %q", out)
	}
	if !strings.Contains(out, "agent.started") {
		t.Errorf("output missing 'agent.started': %q", out)
	}
}

func TestDoEventsWatchPayloadMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	rec.Record(events.Event{Type: events.BeadCreated, Actor: "human"}) // seq 1 — no payload
	rec.Close()                                                        //nolint:errcheck // test cleanup

	// Append events with payloads after a delay.
	go func() {
		time.Sleep(30 * time.Millisecond)
		rec2, err := events.NewFileRecorder(path, &stderrBuf)
		if err != nil {
			return
		}
		// seq 2: task bead — should NOT match
		rec2.Record(events.Event{
			Type:    events.BeadCreated,
			Actor:   "human",
			Subject: "gc-10",
			Payload: json.RawMessage(`{"type":"task","title":"Build thing"}`),
		})
		// seq 3: merge-request bead — should match
		rec2.Record(events.Event{
			Type:    events.BeadCreated,
			Actor:   "polecat",
			Subject: "gc-11",
			Payload: json.RawMessage(`{"type":"merge-request","title":"Fix login"}`),
		})
		rec2.Close() //nolint:errcheck // test cleanup
	}()

	pm := map[string]string{"type": "merge-request"}
	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, events.BeadCreated, pm, 0, 2*time.Second, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "gc-11") {
		t.Errorf("output missing merge-request bead 'gc-11': %q", out)
	}
	if strings.Contains(out, "gc-10") {
		t.Errorf("output should not contain task bead 'gc-10': %q", out)
	}
}

func TestDoEventsWatchPayloadMatchTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var stderrBuf bytes.Buffer
	rec, err := events.NewFileRecorder(path, &stderrBuf)
	if err != nil {
		t.Fatal(err)
	}
	// Only task beads — no merge-request, so payload-match should timeout.
	rec.Record(events.Event{
		Type:    events.BeadCreated,
		Actor:   "human",
		Payload: json.RawMessage(`{"type":"task"}`),
	})
	rec.Close() //nolint:errcheck // test cleanup

	pm := map[string]string{"type": "merge-request"}
	var stdout, stderr bytes.Buffer
	code := doEventsWatch(path, events.BeadCreated, pm, 1, 50*time.Millisecond, 10*time.Millisecond, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doEventsWatch = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("expected empty stdout on timeout, got %q", stdout.String())
	}
}

func TestParsePayloadMatch(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		m, err := parsePayloadMatch([]string{"type=merge-request", "status=open"})
		if err != nil {
			t.Fatal(err)
		}
		if m["type"] != "merge-request" {
			t.Errorf("type = %q, want merge-request", m["type"])
		}
		if m["status"] != "open" {
			t.Errorf("status = %q, want open", m["status"])
		}
	})

	t.Run("empty", func(t *testing.T) {
		m, err := parsePayloadMatch(nil)
		if err != nil {
			t.Fatal(err)
		}
		if m != nil {
			t.Errorf("got %v, want nil", m)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := parsePayloadMatch([]string{"noequals"})
		if err == nil {
			t.Error("expected error for missing =")
		}
	})

	t.Run("value_with_equals", func(t *testing.T) {
		m, err := parsePayloadMatch([]string{"expr=a=b"})
		if err != nil {
			t.Fatal(err)
		}
		if m["expr"] != "a=b" {
			t.Errorf("expr = %q, want a=b", m["expr"])
		}
	})
}

func TestMatchPayload(t *testing.T) {
	t.Run("nil_match_always_true", func(t *testing.T) {
		if !matchPayload(nil, nil) {
			t.Error("nil match should return true")
		}
	})

	t.Run("empty_payload_no_match", func(t *testing.T) {
		if matchPayload(nil, map[string]string{"type": "task"}) {
			t.Error("nil payload should not match")
		}
	})

	t.Run("string_value", func(t *testing.T) {
		p := json.RawMessage(`{"type":"merge-request","title":"Fix"}`)
		if !matchPayload(p, map[string]string{"type": "merge-request"}) {
			t.Error("should match")
		}
		if matchPayload(p, map[string]string{"type": "task"}) {
			t.Error("should not match wrong value")
		}
	})

	t.Run("multiple_keys", func(t *testing.T) {
		p := json.RawMessage(`{"type":"merge-request","assignee":"refinery"}`)
		if !matchPayload(p, map[string]string{"type": "merge-request", "assignee": "refinery"}) {
			t.Error("should match both keys")
		}
		if matchPayload(p, map[string]string{"type": "merge-request", "assignee": "deacon"}) {
			t.Error("should not match wrong second value")
		}
	})

	t.Run("missing_key", func(t *testing.T) {
		p := json.RawMessage(`{"type":"task"}`)
		if matchPayload(p, map[string]string{"assignee": "refinery"}) {
			t.Error("should not match missing key")
		}
	})

	t.Run("non_string_value", func(t *testing.T) {
		p := json.RawMessage(`{"count":42}`)
		if !matchPayload(p, map[string]string{"count": "42"}) {
			t.Error("should match numeric value via raw comparison")
		}
	})
}
