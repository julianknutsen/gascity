package exec //nolint:revive // internal package, always imported with alias

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/events"
)

// writeScript creates an executable shell script in dir and returns its path.
func writeScript(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "events-provider")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// allOpsScript returns a script body that handles all events operations.
func allOpsScript() string {
	return `
op="$1"

case "$op" in
  ensure-running) ;; # no-op, stateless
  record)
    cat > /dev/null  # consume stdin
    ;;
  list)
    cat > /dev/null  # consume stdin (filter)
    echo '[{"seq":1,"type":"bead.created","ts":"2025-06-15T10:30:00Z","actor":"human","subject":"gc-1"},{"seq":2,"type":"bead.closed","ts":"2025-06-15T11:00:00Z","actor":"human","subject":"gc-1"}]'
    ;;
  latest-seq)
    echo '2'
    ;;
  watch)
    echo '{"seq":3,"type":"bead.created","ts":"2025-06-15T12:00:00Z","actor":"human","subject":"gc-2"}'
    ;;
  *) exit 2 ;; # unknown operation
esac
`
}

func TestRecord(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  record) cat > "`+outFile+`" ;;
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	p.Record(events.Event{
		Type:    events.BeadCreated,
		Actor:   "human",
		Subject: "gc-1",
		Message: "Build Tower of Hanoi",
	})

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured stdin: %v", err)
	}
	var e events.Event
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unmarshal stdin: %v", err)
	}
	if e.Type != events.BeadCreated {
		t.Errorf("Type = %q, want %q", e.Type, events.BeadCreated)
	}
	if e.Actor != "human" {
		t.Errorf("Actor = %q, want %q", e.Actor, "human")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	p := NewProvider(script)

	evts, err := p.List(events.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("List = %d events, want 2", len(evts))
	}
	if evts[0].Seq != 1 {
		t.Errorf("evts[0].Seq = %d, want 1", evts[0].Seq)
	}
	if evts[1].Type != events.BeadClosed {
		t.Errorf("evts[1].Type = %q, want %q", evts[1].Type, events.BeadClosed)
	}
}

func TestListEmpty(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  list) cat > /dev/null ;; # empty stdout = no events
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	evts, err := p.List(events.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(evts) != 0 {
		t.Errorf("List = %d events, want 0", len(evts))
	}
}

func TestListSendsFilter(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  list) cat > "`+outFile+`"
    echo '[]'
    ;;
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	_, err := p.List(events.Filter{Type: events.BeadCreated, AfterSeq: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read filter: %v", err)
	}
	var f events.Filter
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal filter: %v", err)
	}
	if f.Type != events.BeadCreated {
		t.Errorf("filter.Type = %q, want %q", f.Type, events.BeadCreated)
	}
	if f.AfterSeq != 5 {
		t.Errorf("filter.AfterSeq = %d, want 5", f.AfterSeq)
	}
}

func TestLatestSeq(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	p := NewProvider(script)

	seq, err := p.LatestSeq()
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq != 2 {
		t.Errorf("LatestSeq = %d, want 2", seq)
	}
}

func TestLatestSeqEmpty(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  latest-seq) ;; # empty stdout = 0
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	seq, err := p.LatestSeq()
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq != 0 {
		t.Errorf("LatestSeq = %d, want 0", seq)
	}
}

func TestWatch(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	p := NewProvider(script)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w, err := p.Watch(ctx, 0)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Close() //nolint:errcheck // test cleanup

	e, err := w.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if e.Seq != 3 {
		t.Errorf("Seq = %d, want 3", e.Seq)
	}
	if e.Type != events.BeadCreated {
		t.Errorf("Type = %q, want %q", e.Type, events.BeadCreated)
	}
}

// --- ensure-running ---

func TestEnsureRunningCalledOnce(t *testing.T) {
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	os.WriteFile(countFile, []byte("0"), 0o644) //nolint:errcheck

	script := writeScript(t, dir, `
case "$1" in
  ensure-running)
    count=$(cat "`+countFile+`")
    echo $((count + 1)) > "`+countFile+`"
    ;;
  list) cat > /dev/null; echo '[]' ;;
  latest-seq) echo '0' ;;
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	// Multiple operations should only call ensure-running once.
	p.List(events.Filter{}) //nolint:errcheck
	p.LatestSeq()           //nolint:errcheck
	p.List(events.Filter{}) //nolint:errcheck

	data, _ := os.ReadFile(countFile)
	count := strings.TrimSpace(string(data))
	if count != "1" {
		t.Errorf("ensure-running called %s times, want 1", count)
	}
}

func TestEnsureRunningExit2Stateless(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  list) cat > /dev/null; echo '[]' ;;
  *) exit 2 ;;
esac
`)
	p := NewProvider(script)

	evts, err := p.List(events.Filter{})
	if err != nil {
		t.Fatalf("List after ensure-running exit 2: %v", err)
	}
	if len(evts) != 0 {
		t.Errorf("List = %d events, want 0", len(evts))
	}
}

// --- Error handling ---

func TestErrorPropagation(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  *)
    echo "something went wrong" >&2
    exit 1
    ;;
esac
`)
	p := NewProvider(script)

	_, err := p.List(events.Filter{})
	if err == nil {
		t.Fatal("expected error from exit 1, got nil")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %q, want stderr content", err.Error())
	}
}

func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test")
	}

	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  ensure-running) exit 2 ;;
  *) sleep 60 ;;
esac
`)
	p := NewProvider(script)
	p.timeout = 500 * time.Millisecond

	start := time.Now()
	_, err := p.List(events.Filter{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("timeout took %v, expected ~500ms", elapsed)
	}
}

// Compile-time interface check.
var _ events.Provider = (*Provider)(nil)
