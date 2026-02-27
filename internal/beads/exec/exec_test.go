package exec //nolint:revive // internal package, always imported with alias

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/formula"
)

// writeScript creates an executable shell script in dir and returns its path.
func writeScript(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "beads-provider")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// allOpsScript returns a script body that handles all bead store operations
// with simple, predictable responses.
func allOpsScript() string {
	return `
op="$1"; shift

case "$op" in
  create)
    cat > /dev/null  # consume stdin
    echo '{"id":"EX-1","title":"test","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z"}'
    ;;
  get)
    echo '{"id":"'"$1"'","title":"found","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z"}'
    ;;
  update)
    cat > /dev/null  # consume stdin
    ;;
  close)
    ;;
  list)
    echo '[{"id":"EX-1","title":"alpha","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z"},{"id":"EX-2","title":"beta","status":"closed","type":"bug","created_at":"2026-02-27T11:00:00Z"}]'
    ;;
  ready)
    echo '[{"id":"EX-1","title":"alpha","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z"}]'
    ;;
  children)
    echo '[{"id":"EX-3","title":"child","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z","parent_id":"'"$1"'"}]'
    ;;
  list-by-label)
    echo '[{"id":"EX-5","title":"labeled","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z","labels":["'"$1"'"]}]'
    ;;
  set-metadata)
    cat > /dev/null  # consume stdin
    ;;
  mol-cook)
    cat > /dev/null  # consume stdin
    echo "EX-99"
    ;;
  *) exit 2 ;;  # unknown operation
esac
`
}

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	b, err := s.Create(beads.Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.ID != "EX-1" {
		t.Errorf("ID = %q, want %q", b.ID, "EX-1")
	}
	if b.Status != "open" {
		t.Errorf("Status = %q, want %q", b.Status, "open")
	}
	if b.Type != "task" {
		t.Errorf("Type = %q, want %q", b.Type, "task")
	}
}

func TestCreate_stdinReachesScript(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
op="$1"
case "$op" in
  create)
    cat > "`+outFile+`"
    echo '{"id":"EX-1","title":"test","status":"open","type":"bug","created_at":"2026-02-27T10:00:00Z"}'
    ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	_, err := s.Create(beads.Bead{
		Title:    "my task",
		Type:     "bug",
		Labels:   []string{"pool:dog"},
		ParentID: "WP-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured stdin: %v", err)
	}
	stdin := string(data)
	if !strings.Contains(stdin, `"title":"my task"`) {
		t.Errorf("stdin missing title, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"type":"bug"`) {
		t.Errorf("stdin missing type, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"pool:dog"`) {
		t.Errorf("stdin missing label, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"parent_id":"WP-1"`) {
		t.Errorf("stdin missing parent_id, got: %s", stdin)
	}
}

func TestCreate_defaultsTypeToTask(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
case "$1" in
  create)
    cat > "`+outFile+`"
    echo '{"id":"EX-1","title":"test","status":"open","type":"task","created_at":"2026-02-27T10:00:00Z"}'
    ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)
	_, err := s.Create(beads.Bead{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type":"task"`) {
		t.Errorf("stdin should contain type=task, got: %s", string(data))
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	b, err := s.Get("EX-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if b.ID != "EX-1" {
		t.Errorf("ID = %q, want %q", b.ID, "EX-1")
	}
	if b.Title != "found" {
		t.Errorf("Title = %q, want %q", b.Title, "found")
	}
}

func TestGet_notFound(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  get) echo "not found" >&2; exit 1 ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, beads.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
case "$1" in
  update) cat > "`+outFile+`" ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	desc := "new description"
	err := s.Update("EX-1", beads.UpdateOpts{
		Description: &desc,
		Labels:      []string{"extra"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	stdin := string(data)
	if !strings.Contains(stdin, `"description":"new description"`) {
		t.Errorf("stdin missing description, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"extra"`) {
		t.Errorf("stdin missing label, got: %s", stdin)
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	if err := s.Close("EX-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List returned %d beads, want 2", len(got))
	}
	if got[0].Title != "alpha" {
		t.Errorf("got[0].Title = %q, want %q", got[0].Title, "alpha")
	}
	if got[1].Title != "beta" {
		t.Errorf("got[1].Title = %q, want %q", got[1].Title, "beta")
	}
}

func TestList_empty(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  list) echo "[]" ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List returned %d beads, want 0", len(got))
	}
}

func TestReady(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	got, err := s.Ready()
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Ready returned %d beads, want 1", len(got))
	}
	if got[0].Status != "open" {
		t.Errorf("got[0].Status = %q, want %q", got[0].Status, "open")
	}
}

func TestChildren(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	got, err := s.Children("EX-1")
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Children returned %d beads, want 1", len(got))
	}
	if got[0].ParentID != "EX-1" {
		t.Errorf("got[0].ParentID = %q, want %q", got[0].ParentID, "EX-1")
	}
}

func TestListByLabel(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	got, err := s.ListByLabel("plugin-run:lint", 0)
	if err != nil {
		t.Fatalf("ListByLabel: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListByLabel returned %d beads, want 1", len(got))
	}
	if got[0].Title != "labeled" {
		t.Errorf("got[0].Title = %q, want %q", got[0].Title, "labeled")
	}
}

func TestSetMetadata(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "meta.txt")

	script := writeScript(t, dir, `
case "$1" in
  set-metadata) cat > "`+outFile+`" ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	if err := s.SetMetadata("EX-1", "merge_strategy", "mr"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mr" {
		t.Errorf("metadata value = %q, want %q", string(data), "mr")
	}
}

func TestMolCook(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, allOpsScript())
	s := NewStore(script)

	id, err := s.MolCook("code-review", "Review PR #42", nil)
	if err != nil {
		t.Fatalf("MolCook: %v", err)
	}
	if id != "EX-99" {
		t.Errorf("MolCook ID = %q, want %q", id, "EX-99")
	}
}

func TestMolCook_stdinReachesScript(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdin.json")

	script := writeScript(t, dir, `
case "$1" in
  mol-cook)
    cat > "`+outFile+`"
    echo "EX-42"
    ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	_, err := s.MolCook("deploy", "Deploy v2", []string{"env=prod"})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	stdin := string(data)
	if !strings.Contains(stdin, `"formula":"deploy"`) {
		t.Errorf("stdin missing formula, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"title":"Deploy v2"`) {
		t.Errorf("stdin missing title, got: %s", stdin)
	}
	if !strings.Contains(stdin, `"env=prod"`) {
		t.Errorf("stdin missing vars, got: %s", stdin)
	}
}

func TestMolCook_emptyOutput(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  mol-cook) cat > /dev/null ;; # empty stdout
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	_, err := s.MolCook("deploy", "", nil)
	if err == nil {
		t.Fatal("expected error for empty mol-cook output")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("error = %q, want to contain 'empty output'", err)
	}
}

// --- Error handling ---

func TestErrorPropagation(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
echo "something went wrong" >&2
exit 1
`)
	s := NewStore(script)

	_, err := s.List()
	if err == nil {
		t.Fatal("expected error from exit 1, got nil")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %q, want stderr content", err.Error())
	}
}

func TestUnknownOperation_exit2(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `exit 2`)
	s := NewStore(script)

	// Exit 2 → unknown operation → treated as success.
	// List returns empty because stdout is empty.
	got, err := s.List()
	if err != nil {
		t.Fatalf("exit 2 should be treated as success, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List returned %d beads on exit 2, want 0", len(got))
	}
}

func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test")
	}

	dir := t.TempDir()
	script := writeScript(t, dir, `sleep 60`)
	s := NewStore(script)
	s.timeout = 500 * time.Millisecond

	start := time.Now()
	_, err := s.List()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took %v, expected ~500ms", elapsed)
	}
}

func TestCreate_badJSON(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `
case "$1" in
  create) cat > /dev/null; echo '{not json' ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)

	_, err := s.Create(beads.Bead{Title: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parsing JSON") {
		t.Errorf("error = %q, want to contain 'parsing JSON'", err)
	}
}

// --- Composed MolCook ---

func TestMolCook_withResolver(t *testing.T) {
	dir := t.TempDir()

	// Track all creates the script sees.
	logFile := filepath.Join(dir, "creates.log")
	script := writeScript(t, dir, `
op="$1"; shift
case "$op" in
  create)
    input=$(cat)
    echo "$input" >> "`+logFile+`"
    # Return incrementing IDs.
    n=$(wc -l < "`+logFile+`" 2>/dev/null || echo 1)
    echo "{\"id\":\"EX-$n\",\"title\":\"t\",\"status\":\"open\",\"type\":\"task\",\"created_at\":\"2026-02-27T10:00:00Z\"}"
    ;;
  *) exit 2 ;;
esac
`)
	s := NewStore(script)
	s.SetFormulaResolver(func(name string) (*formula.Formula, error) {
		if name != "deploy" {
			return nil, fmt.Errorf("unknown formula %q", name)
		}
		return &formula.Formula{
			Name: "deploy",
			Steps: []formula.Step{
				{ID: "build", Title: "Build"},
				{ID: "test", Title: "Test", Needs: []string{"build"}},
			},
		}, nil
	})

	rootID, err := s.MolCook("deploy", "Deploy v3", nil)
	if err != nil {
		t.Fatalf("MolCook: %v", err)
	}
	if rootID == "" {
		t.Fatal("MolCook returned empty root ID")
	}

	// Verify the script received 3 create calls (1 root + 2 steps).
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("script received %d creates, want 3", len(lines))
	}

	// First create should be the molecule root.
	if !strings.Contains(lines[0], `"type":"molecule"`) {
		t.Errorf("first create missing type=molecule: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"ref":"deploy"`) {
		t.Errorf("first create missing ref=deploy: %s", lines[0])
	}
}

func TestMolCook_withResolverError(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, `exit 2`)
	s := NewStore(script)
	s.SetFormulaResolver(func(name string) (*formula.Formula, error) {
		return nil, fmt.Errorf("formula %q not found", name)
	})

	_, err := s.MolCook("missing", "title", nil)
	if err == nil {
		t.Fatal("expected error for missing formula")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want to contain formula name", err)
	}
}

// --- Compile-time interface check ---

var _ beads.Store = (*Store)(nil)
