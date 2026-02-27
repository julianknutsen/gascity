package beads_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
)

// fakeRunner returns a CommandRunner that returns canned output for specific
// commands, or an error if the command is unrecognized.
func fakeRunner(responses map[string]struct {
	out []byte
	err error
},
) beads.CommandRunner {
	return func(_, name string, args ...string) ([]byte, error) {
		key := name + " " + strings.Join(args, " ")
		if resp, ok := responses[key]; ok {
			return resp.out, resp.err
		}
		return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
	}
}

// --- Create ---

func TestBdStoreCreate(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd create --json Build a widget -t task`: {
			out: []byte(`{"id":"bd-abc-123","title":"Build a widget","status":"open","issue_type":"task","created_at":"2025-01-15T10:30:00Z","owner":""}`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	b, err := s.Create(beads.Bead{Title: "Build a widget"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "bd-abc-123" {
		t.Errorf("ID = %q, want %q", b.ID, "bd-abc-123")
	}
	if b.Title != "Build a widget" {
		t.Errorf("Title = %q, want %q", b.Title, "Build a widget")
	}
	if b.Status != "open" {
		t.Errorf("Status = %q, want %q", b.Status, "open")
	}
	if b.Type != "task" {
		t.Errorf("Type = %q, want %q", b.Type, "task")
	}
}

func TestBdStoreCreateDefaultsTypeToTask(t *testing.T) {
	var gotArgs []string
	runner := func(_, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"id":"bd-x","title":"test","status":"open","issue_type":"task","created_at":"2025-01-15T10:30:00Z"}`), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Create(beads.Bead{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	// Should pass -t task when Type is empty.
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "-t task") {
		t.Errorf("args = %q, want to contain '-t task'", args)
	}
}

func TestBdStoreCreatePreservesExplicitType(t *testing.T) {
	var gotArgs []string
	runner := func(_, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"id":"bd-x","title":"test","status":"open","issue_type":"bug","created_at":"2025-01-15T10:30:00Z"}`), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Create(beads.Bead{Title: "test", Type: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "-t bug") {
		t.Errorf("args = %q, want to contain '-t bug'", args)
	}
}

func TestBdStoreCreateError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Create(beads.Bead{Title: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd create") {
		t.Errorf("error = %q, want to contain 'bd create'", err)
	}
}

func TestBdStoreCreateBadJSON(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte(`{not json`), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Create(beads.Bead{Title: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parsing JSON") {
		t.Errorf("error = %q, want to contain 'parsing JSON'", err)
	}
}

// --- Get ---

func TestBdStoreGet(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd show --json bd-abc-123`: {
			out: []byte(`[{"id":"bd-abc-123","title":"Build a widget","status":"open","issue_type":"task","created_at":"2025-01-15T10:30:00Z","assignee":"alice"}]`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	b, err := s.Get("bd-abc-123")
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "bd-abc-123" {
		t.Errorf("ID = %q, want %q", b.ID, "bd-abc-123")
	}
	if b.Assignee != "alice" {
		t.Errorf("Assignee = %q, want %q", b.Assignee, "alice")
	}
}

func TestBdStoreGetNotFound(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Get("nonexistent-999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, beads.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestBdStoreGetBadJSON(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte(`not json`), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Get("bd-abc-123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parsing JSON") {
		t.Errorf("error = %q, want to contain 'parsing JSON'", err)
	}
}

func TestBdStoreGetEmptyArray(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte(`[]`), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Get("bd-abc-123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, beads.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// --- Close ---

func TestBdStoreClose(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd close --json bd-abc-123`: {
			out: []byte(`[{"id":"bd-abc-123","title":"test","status":"closed","issue_type":"task","created_at":"2025-01-15T10:30:00Z"}]`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	if err := s.Close("bd-abc-123"); err != nil {
		t.Fatal(err)
	}
}

func TestBdStoreCloseNotFound(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	err := s.Close("nonexistent-999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, beads.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// --- List ---

func TestBdStoreList(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd list --json --limit 0 --all`: {
			out: []byte(`[{"id":"bd-aaa","title":"first","status":"open","issue_type":"task","created_at":"2025-01-15T10:30:00Z"},{"id":"bd-bbb","title":"second","status":"closed","issue_type":"bug","created_at":"2025-01-15T10:31:00Z"}]`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List() returned %d beads, want 2", len(got))
	}
	if got[0].ID != "bd-aaa" {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, "bd-aaa")
	}
	if got[1].Status != "closed" {
		t.Errorf("got[1].Status = %q, want %q", got[1].Status, "closed")
	}
}

func TestBdStoreListEmpty(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd list --json --limit 0 --all`: {out: []byte(`[]`)},
	})
	s := beads.NewBdStore("/city", runner)
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("List() returned %d beads, want 0", len(got))
	}
}

func TestBdStoreListError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.List()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd list") {
		t.Errorf("error = %q, want to contain 'bd list'", err)
	}
}

// --- Ready ---

func TestBdStoreReady(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd ready --json --limit 0`: {
			out: []byte(`[{"id":"bd-aaa","title":"ready one","status":"open","issue_type":"task","created_at":"2025-01-15T10:30:00Z"}]`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	got, err := s.Ready()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Ready() returned %d beads, want 1", len(got))
	}
	if got[0].Title != "ready one" {
		t.Errorf("got[0].Title = %q, want %q", got[0].Title, "ready one")
	}
}

func TestBdStoreReadyEmpty(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd ready --json --limit 0`: {out: []byte(`[]`)},
	})
	s := beads.NewBdStore("/city", runner)
	got, err := s.Ready()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Ready() returned %d beads, want 0", len(got))
	}
}

func TestBdStoreReadyError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.Ready()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd ready") {
		t.Errorf("error = %q, want to contain 'bd ready'", err)
	}
}

// --- Status mapping ---

func TestBdStoreStatusMapping(t *testing.T) {
	tests := []struct {
		bdStatus   string
		wantStatus string
	}{
		{"open", "open"},
		{"in_progress", "in_progress"},
		{"blocked", "open"},
		{"review", "open"},
		{"testing", "open"},
		{"closed", "closed"},
	}
	for _, tt := range tests {
		t.Run(tt.bdStatus, func(t *testing.T) {
			runner := fakeRunner(map[string]struct {
				out []byte
				err error
			}{
				`bd show --json bd-x`: {
					out: []byte(fmt.Sprintf(`[{"id":"bd-x","title":"test","status":%q,"issue_type":"task","created_at":"2025-01-15T10:30:00Z"}]`, tt.bdStatus)),
				},
			})
			s := beads.NewBdStore("/city", runner)
			b, err := s.Get("bd-x")
			if err != nil {
				t.Fatal(err)
			}
			if b.Status != tt.wantStatus {
				t.Errorf("status %q → %q, want %q", tt.bdStatus, b.Status, tt.wantStatus)
			}
		})
	}
}

// --- Init ---

func TestBdStoreInit(t *testing.T) {
	var gotDir, gotName string
	var gotArgs []string
	runner := func(dir, name string, args ...string) ([]byte, error) {
		gotDir = dir
		gotName = name
		gotArgs = args
		return nil, nil
	}
	s := beads.NewBdStore("/my/city", runner)
	if err := s.Init("bright-lights"); err != nil {
		t.Fatal(err)
	}
	if gotDir != "/my/city" {
		t.Errorf("dir = %q, want %q", gotDir, "/my/city")
	}
	if gotName != "bd" {
		t.Errorf("name = %q, want %q", gotName, "bd")
	}
	wantArgs := "init --server -p bright-lights --skip-hooks"
	if strings.Join(gotArgs, " ") != wantArgs {
		t.Errorf("args = %q, want %q", strings.Join(gotArgs, " "), wantArgs)
	}
}

func TestBdStoreInitError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte("init failed"), fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	err := s.Init("test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd init") {
		t.Errorf("error = %q, want to contain 'bd init'", err)
	}
}

// --- ConfigSet ---

func TestBdStoreConfigSet(t *testing.T) {
	var gotArgs []string
	runner := func(_, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return nil, nil
	}
	s := beads.NewBdStore("/city", runner)
	if err := s.ConfigSet("issue_prefix", "bl"); err != nil {
		t.Fatal(err)
	}
	wantArgs := "config set issue_prefix bl"
	if strings.Join(gotArgs, " ") != wantArgs {
		t.Errorf("args = %q, want %q", strings.Join(gotArgs, " "), wantArgs)
	}
}

func TestBdStoreConfigSetError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte("config failed"), fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	err := s.ConfigSet("issue_prefix", "bl")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd config set") {
		t.Errorf("error = %q, want to contain 'bd config set'", err)
	}
}

// --- Purge ---

func TestBdStorePurge(t *testing.T) {
	var gotArgs []string
	var gotDir string
	var gotEnv []string
	s := beads.NewBdStore("/city", nil)
	s.SetPurgeRunner(func(dir string, env []string, args ...string) ([]byte, error) {
		gotDir = dir
		gotArgs = args
		gotEnv = env
		return []byte(`{"purged_count": 5}`), nil
	})
	result, err := s.Purge("/city/rigs/fe/.beads", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Purged != 5 {
		t.Errorf("Purged = %d, want 5", result.Purged)
	}
	// Verify args include --allow-stale purge --json.
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--allow-stale") || !strings.Contains(args, "purge") || !strings.Contains(args, "--json") {
		t.Errorf("args = %q, want --allow-stale purge --json", args)
	}
	// Should NOT contain --dry-run.
	if strings.Contains(args, "--dry-run") {
		t.Errorf("args = %q, should not contain --dry-run", args)
	}
	// Dir should be parent of beads dir.
	if gotDir != "/city/rigs/fe" {
		t.Errorf("dir = %q, want %q", gotDir, "/city/rigs/fe")
	}
	// Env should contain BEADS_DIR.
	foundBeadsDir := false
	for _, e := range gotEnv {
		if e == "BEADS_DIR=/city/rigs/fe/.beads" {
			foundBeadsDir = true
		}
	}
	if !foundBeadsDir {
		t.Errorf("env missing BEADS_DIR; got %v", gotEnv)
	}
}

func TestBdStorePurgeDryRun(t *testing.T) {
	var gotArgs []string
	s := beads.NewBdStore("/city", nil)
	s.SetPurgeRunner(func(_ string, _ []string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"purged_count": 0}`), nil
	})
	_, err := s.Purge("/city/.beads", true)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--dry-run") {
		t.Errorf("args = %q, want --dry-run", args)
	}
}

func TestBdStorePurgeError(t *testing.T) {
	s := beads.NewBdStore("/city", nil)
	s.SetPurgeRunner(func(_ string, _ []string, _ ...string) ([]byte, error) {
		return []byte("purge failed"), fmt.Errorf("exit status 1")
	})
	_, err := s.Purge("/city/.beads", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd purge") {
		t.Errorf("error = %q, want to contain 'bd purge'", err)
	}
}

func TestBdStorePurgeBadJSON(t *testing.T) {
	s := beads.NewBdStore("/city", nil)
	s.SetPurgeRunner(func(_ string, _ []string, _ ...string) ([]byte, error) {
		return []byte("not json"), nil
	})
	_, err := s.Purge("/city/.beads", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected output") {
		t.Errorf("error = %q, want to contain 'unexpected output'", err)
	}
}

func TestBdStorePurgeMissingCount(t *testing.T) {
	s := beads.NewBdStore("/city", nil)
	s.SetPurgeRunner(func(_ string, _ []string, _ ...string) ([]byte, error) {
		return []byte(`{"other_field": true}`), nil
	})
	result, err := s.Purge("/city/.beads", false)
	if err != nil {
		t.Fatal(err)
	}
	// Missing purged_count should return 0 (not an error).
	if result.Purged != 0 {
		t.Errorf("Purged = %d, want 0 (missing field)", result.Purged)
	}
}

// --- MolCook ---

func TestBdStoreMolCook(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd mol cook --formula=code-review`: {
			out: []byte("WP-42\n"),
		},
	})
	s := beads.NewBdStore("/city", runner)
	rootID, err := s.MolCook("code-review", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if rootID != "WP-42" {
		t.Errorf("rootID = %q, want %q", rootID, "WP-42")
	}
}

func TestBdStoreMolCookWithTitle(t *testing.T) {
	var gotArgs []string
	runner := func(_, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("WP-99\n"), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.MolCook("code-review", "my-review", nil)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--title=my-review") {
		t.Errorf("args = %q, want --title=my-review", args)
	}
}

func TestBdStoreMolCookWithVars(t *testing.T) {
	var gotArgs []string
	runner := func(_, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("WP-100\n"), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.MolCook("code-review", "", []string{"version=1.0", "pr=123"})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--var version=1.0") {
		t.Errorf("args = %q, want --var version=1.0", args)
	}
	if !strings.Contains(args, "--var pr=123") {
		t.Errorf("args = %q, want --var pr=123", args)
	}
}

func TestBdStoreMolCookError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.MolCook("nonexistent", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bd mol cook") {
		t.Errorf("error = %q, want to contain 'bd mol cook'", err)
	}
}

func TestBdStoreMolCookEmptyOutput(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return []byte("   \n"), nil
	}
	s := beads.NewBdStore("/city", runner)
	_, err := s.MolCook("bad-formula", "", nil)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("err = %v, want 'empty output'", err)
	}
}

// --- Verify working directory is passed ---

func TestBdStorePassesDir(t *testing.T) {
	var gotDir string
	runner := func(dir, _ string, _ ...string) ([]byte, error) {
		gotDir = dir
		return []byte(`[]`), nil
	}
	s := beads.NewBdStore("/my/city", runner)
	_, _ = s.List()
	if gotDir != "/my/city" {
		t.Errorf("dir = %q, want %q", gotDir, "/my/city")
	}
}
