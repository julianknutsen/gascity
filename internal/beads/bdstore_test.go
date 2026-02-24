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

// --- Claim ---

func TestBdStoreClaim(t *testing.T) {
	runner := fakeRunner(map[string]struct {
		out []byte
		err error
	}{
		`bd update --json --claim bd-abc-123 -a worker`: {
			out: []byte(`[{"id":"bd-abc-123","title":"test","status":"in_progress","issue_type":"task","created_at":"2025-01-15T10:30:00Z","assignee":"worker"}]`),
		},
	})
	s := beads.NewBdStore("/city", runner)
	if err := s.Claim("bd-abc-123", "worker"); err != nil {
		t.Fatal(err)
	}
}

func TestBdStoreClaimError(t *testing.T) {
	runner := func(_, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1")
	}
	s := beads.NewBdStore("/city", runner)
	err := s.Claim("nonexistent-999", "worker")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claiming bead") {
		t.Errorf("error = %q, want to contain 'claiming bead'", err)
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
