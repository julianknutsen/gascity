package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"gc": func() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) },
	})
}

func TestTutorial01(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
	})
}

// --- gc version ---

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	code := run([]string{"version"}, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Errorf("run([version]) = %d, want 0", code)
	}
	out := stdout.String()
	// Default values when not built with ldflags.
	if !strings.Contains(out, "gc dev") {
		t.Errorf("stdout missing 'gc dev': %q", out)
	}
	if !strings.Contains(out, "commit:") {
		t.Errorf("stdout missing 'commit:': %q", out)
	}
	if !strings.Contains(out, "built:") {
		t.Errorf("stdout missing 'built:': %q", out)
	}
}

// --- findCity ---

func TestFindCity(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
			t.Fatal(err)
		}

		got, err := findCity(dir)
		if err != nil {
			t.Fatalf("findCity(%q) error: %v", dir, err)
		}
		if got != dir {
			t.Errorf("findCity(%q) = %q, want %q", dir, got, dir)
		}
	})

	t.Run("nested", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(dir, "sub", "deep")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}

		got, err := findCity(nested)
		if err != nil {
			t.Fatalf("findCity(%q) error: %v", nested, err)
		}
		if got != dir {
			t.Errorf("findCity(%q) = %q, want %q", nested, got, dir)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := t.TempDir() // no .gc/ inside
		_, err := findCity(dir)
		if err == nil {
			t.Fatal("findCity() should fail without .gc/")
		}
		if !strings.Contains(err.Error(), "not in a city directory") {
			t.Errorf("error = %q, want 'not in a city directory'", err)
		}
	})
}

// --- gc bd close ---

func TestBeadCloseMissingID(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadClose(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doBeadClose(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing bead ID") {
		t.Errorf("stderr = %q, want 'missing bead ID'", stderr.String())
	}
}

func TestBeadCloseNotFound(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadClose(store, events.Discard, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doBeadClose(gc-999) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestBeadCloseSuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Build a Tower of Hanoi app"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadClose(store, events.Discard, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadClose = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Closed bead: gc-1") {
		t.Errorf("stdout = %q, want 'Closed bead: gc-1'", stdout.String())
	}

	// Verify bead is actually closed.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "closed" {
		t.Errorf("bead status = %q, want %q", b.Status, "closed")
	}
}

// --- gc bd create ---

func TestBeadCreateMissingTitle(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadCreate(store, events.Discard, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doBeadCreate(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing title") {
		t.Errorf("stderr = %q, want 'missing title'", stderr.String())
	}
}

func TestBeadCreateSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadCreate(store, events.Discard, []string{"Build a Tower of Hanoi app"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadCreate = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Created bead: gc-1") {
		t.Errorf("stdout missing 'Created bead: gc-1': %q", out)
	}
	if !strings.Contains(out, "(status: open)") {
		t.Errorf("stdout missing '(status: open)': %q", out)
	}
}

// --- gc bd list ---

func TestBeadListEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadList(store, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	for _, col := range []string{"ID", "STATUS", "ASSIGNEE", "TITLE"} {
		if !strings.Contains(out, col) {
			t.Errorf("stdout missing header %q: %q", col, out)
		}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d: %q", len(lines), out)
	}
}

func TestBeadListSuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Build a Tower of Hanoi app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(beads.Bead{Title: "Add unit tests"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadList(store, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ID", "STATUS", "ASSIGNEE", "TITLE",
		"gc-1", "open", "\u2014", "Build a Tower of Hanoi app",
		"gc-2", "Add unit tests",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (header + 2 beads), got %d: %q", len(lines), out)
	}
}

// --- gc bd ready ---

func TestBeadReadyEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadReady(store, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadReady = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "TITLE") {
		t.Errorf("stdout missing header: %q", out)
	}
	// Should only have the header line, no data rows.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d: %q", len(lines), out)
	}
}

func TestBeadReadySuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Build a Tower of Hanoi app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(beads.Bead{Title: "Add unit tests"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadReady(store, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadReady = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ID", "STATUS", "TITLE",
		"gc-1", "open", "Build a Tower of Hanoi app",
		"gc-2", "Add unit tests",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (header + 2 beads), got %d: %q", len(lines), out)
	}
}

// --- gc bd show ---

func TestBeadShowMissingID(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadShow(store, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doBeadShow(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing bead ID") {
		t.Errorf("stderr = %q, want 'missing bead ID'", stderr.String())
	}
}

func TestBeadShowNotFound(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadShow(store, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doBeadShow(gc-999) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestBeadShowSuccess(t *testing.T) {
	store := beads.NewMemStore()

	// Create a bead first.
	_, err := store.Create(beads.Bead{Title: "Build a Tower of Hanoi app"})
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doBeadShow(store, []string{"gc-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doBeadShow = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ID:       gc-1",
		"Status:   open",
		"Type:     task",
		"Title:    Build a Tower of Hanoi app",
		"Created:",
		"Assignee: \u2014",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

// --- doAgentClaim ---

func TestDoAgentClaimSuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Print hello"}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doAgentClaim(store, events.Discard, "worker", "gc-1", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentClaim = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Claimed bead 'gc-1' for agent 'worker'") {
		t.Errorf("stdout = %q, want claim message", out)
	}

	// Verify bead state.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "in_progress" {
		t.Errorf("bead status = %q, want %q", b.Status, "in_progress")
	}
	if b.Assignee != "worker" {
		t.Errorf("bead assignee = %q, want %q", b.Assignee, "worker")
	}
}

func TestDoAgentClaimNotFound(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doAgentClaim(store, events.Discard, "worker", "gc-999", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentClaim = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestDoAgentClaimAlreadyClaimed(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "contested"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim("gc-1", "worker"); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := doAgentClaim(store, events.Discard, "builder", "gc-1", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentClaim = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already claimed") {
		t.Errorf("stderr = %q, want conflict message", stderr.String())
	}
}

// --- doAgentAttach ---

func TestAgentAttachStartsAndAttaches(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAttach = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Attaching to agent 'mayor'...") {
		t.Errorf("stdout = %q, want attach message", stdout.String())
	}

	// Verify call sequence: IsRunning → Start → Name → Attach.
	want := []string{"IsRunning", "Start", "Name", "Attach"}
	if len(f.Calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %+v", len(f.Calls), len(want), f.Calls)
	}
	for i, c := range f.Calls {
		if c.Method != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c.Method, want[i])
		}
	}
}

func TestAgentAttachExistingSession(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAttach = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Should skip Start: IsRunning → Name → Attach.
	want := []string{"IsRunning", "Name", "Attach"}
	if len(f.Calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %+v", len(f.Calls), len(want), f.Calls)
	}
	for i, c := range f.Calls {
		if c.Method != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c.Method, want[i])
		}
	}
}

func TestAgentAttachStartError(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.StartErr = fmt.Errorf("boom")

	var stderr bytes.Buffer
	code := doAgentAttach(f, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentAttach = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "starting session") {
		t.Errorf("stderr = %q, want 'starting session' error", stderr.String())
	}
}

func TestAgentAttachAttachError(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.AttachErr = fmt.Errorf("attach boom")

	var stderr bytes.Buffer
	code := doAgentAttach(f, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentAttach = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "attaching to session") {
		t.Errorf("stderr = %q, want 'attaching to session' error", stderr.String())
	}
}

// --- doRigAdd (with fsys.Fake) ---

func TestDoRigAddStatFails(t *testing.T) {
	f := fsys.NewFake()
	// rigPath doesn't exist in the fake → Stat returns not-exist error.

	var stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc rig add") {
		t.Errorf("stderr = %q, want 'gc rig add' prefix", stderr.String())
	}
}

func TestDoRigAddNotADirectory(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/projects/myapp"] = []byte("not a dir") // file, not directory

	var stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "is not a directory") {
		t.Errorf("stderr = %q, want 'is not a directory'", stderr.String())
	}
}

func TestDoRigAddMkdirFails(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true
	f.Files["/city/city.toml"] = []byte("[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n")
	rigDir := filepath.Join("/city", "rigs", "myapp")
	f.Errors[rigDir] = fmt.Errorf("permission denied")

	var stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "permission denied") {
		t.Errorf("stderr = %q, want 'permission denied'", stderr.String())
	}
}

func TestDoRigAddWriteTomlFails(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true
	f.Files["/city/city.toml"] = []byte("[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n")
	rigToml := filepath.Join("/city", "rigs", "myapp", "rig.toml")
	f.Errors[rigToml] = fmt.Errorf("disk full")

	var stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "disk full") {
		t.Errorf("stderr = %q, want 'disk full'", stderr.String())
	}
}

func TestDoRigAddWithGit(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	// Use real temp dirs so writeAllRoutes (which uses os.MkdirAll) works.
	cityPath := t.TempDir()
	rigPath := filepath.Join(t.TempDir(), "myapp")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"),
		[]byte("[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigAdd = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Detected git repo") {
		t.Errorf("stdout missing 'Detected git repo': %q", out)
	}
	if !strings.Contains(out, "Rig added.") {
		t.Errorf("stdout missing 'Rig added.': %q", out)
	}
}

func TestDoRigAddWithoutGit(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")
	cityPath := t.TempDir()
	rigPath := filepath.Join(t.TempDir(), "myapp")
	if err := os.MkdirAll(rigPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"),
		[]byte("[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doRigAdd(fsys.OSFS{}, cityPath, rigPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigAdd = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "Detected git repo") {
		t.Errorf("stdout should not contain 'Detected git repo': %q", out)
	}
	if !strings.Contains(out, "Rig added.") {
		t.Errorf("stdout missing 'Rig added.': %q", out)
	}
}

// --- doRigList (with fsys.Fake) ---

func TestDoRigListConfigLoadFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors[filepath.Join("/city", "city.toml")] = fmt.Errorf("no such file")

	var stderr bytes.Buffer
	code := doRigList(f, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigList = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no such file") {
		t.Errorf("stderr = %q, want 'no such file'", stderr.String())
	}
}

func TestDoRigListSuccess(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/city.toml"] = []byte("[workspace]\nname = \"test-city\"\n\n[[agents]]\nname = \"mayor\"\n\n[[rigs]]\nname = \"alpha\"\npath = \"/projects/alpha\"\n\n[[rigs]]\nname = \"beta\"\npath = \"/projects/beta\"\n")

	var stdout, stderr bytes.Buffer
	code := doRigList(f, "/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doRigList = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "alpha:") {
		t.Errorf("stdout missing 'alpha:': %q", out)
	}
	if !strings.Contains(out, "beta:") {
		t.Errorf("stdout missing 'beta:': %q", out)
	}
}

// --- sessionName ---

func TestSessionName(t *testing.T) {
	got := sessionName("bright-lights", "mayor")
	want := "gc-bright-lights-mayor"
	if got != want {
		t.Errorf("sessionName = %q, want %q", got, want)
	}
}

// --- gc init (doInit with fsys.Fake) ---

func TestDoInitSuccess(t *testing.T) {
	f := fsys.NewFake()
	// No pre-existing files — doInit creates everything from scratch.

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/bright-lights", defaultWizardConfig(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing 'Welcome to Gas City!': %q", out)
	}
	if !strings.Contains(out, "Initialized city") {
		t.Errorf("stdout missing 'Initialized city': %q", out)
	}
	if !strings.Contains(out, "bright-lights") {
		t.Errorf("stdout missing city name: %q", out)
	}

	// Verify .gc/, rigs/, and prompts/ were created.
	if !f.Dirs[filepath.Join("/bright-lights", ".gc")] {
		t.Error(".gc/ not created")
	}
	if !f.Dirs[filepath.Join("/bright-lights", "rigs")] {
		t.Error("rigs/ not created")
	}
	if !f.Dirs[filepath.Join("/bright-lights", "prompts")] {
		t.Error("prompts/ not created")
	}

	// Verify prompt files were written.
	if _, ok := f.Files[filepath.Join("/bright-lights", "prompts", "mayor.md")]; !ok {
		t.Error("prompts/mayor.md not written")
	}
	if _, ok := f.Files[filepath.Join("/bright-lights", "prompts", "worker.md")]; !ok {
		t.Error("prompts/worker.md not written")
	}

	// Verify written config parses correctly.
	data := f.Files[filepath.Join("/bright-lights", "city.toml")]
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if cfg.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "bright-lights")
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Agents[0].PromptTemplate != "prompts/mayor.md" {
		t.Errorf("Agents[0].PromptTemplate = %q, want %q", cfg.Agents[0].PromptTemplate, "prompts/mayor.md")
	}
}

func TestDoInitWritesExpectedTOML(t *testing.T) {
	f := fsys.NewFake()

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/bright-lights", defaultWizardConfig(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}

	got := string(f.Files[filepath.Join("/bright-lights", "city.toml")])
	want := `[workspace]
name = "bright-lights"

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"
`
	if got != want {
		t.Errorf("city.toml content:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDoInitAlreadyInitialized(t *testing.T) {
	f := fsys.NewFake()
	// .gc/ already exists.
	f.Dirs[filepath.Join("/city", ".gc")] = true

	var stderr bytes.Buffer
	code := doInit(f, "/city", defaultWizardConfig(), &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doInit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already initialized") {
		t.Errorf("stderr = %q, want 'already initialized'", stderr.String())
	}
}

func TestDoInitMkdirGCFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors[filepath.Join("/city", ".gc")] = fmt.Errorf("permission denied")

	var stderr bytes.Buffer
	code := doInit(f, "/city", defaultWizardConfig(), &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doInit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "permission denied") {
		t.Errorf("stderr = %q, want 'permission denied'", stderr.String())
	}
}

func TestDoInitMkdirRigsFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors[filepath.Join("/city", "rigs")] = fmt.Errorf("disk full")

	var stderr bytes.Buffer
	code := doInit(f, "/city", defaultWizardConfig(), &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doInit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "disk full") {
		t.Errorf("stderr = %q, want 'disk full'", stderr.String())
	}
}

func TestDoInitWriteFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors[filepath.Join("/city", "city.toml")] = fmt.Errorf("read-only fs")

	var stderr bytes.Buffer
	code := doInit(f, "/city", defaultWizardConfig(), &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doInit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "read-only fs") {
		t.Errorf("stderr = %q, want 'read-only fs'", stderr.String())
	}
}

// --- runWizard ---

func TestRunWizardDefaults(t *testing.T) {
	// Two enters → default template (hello-world) + default agent (claude).
	stdin := strings.NewReader("\n\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if !wiz.interactive {
		t.Error("expected interactive = true")
	}
	if wiz.configName != "hello-world" {
		t.Errorf("configName = %q, want %q", wiz.configName, "hello-world")
	}
	if wiz.provider != "claude" {
		t.Errorf("provider = %q, want %q", wiz.provider, "claude")
	}
	// Verify both prompts were printed.
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City SDK!") {
		t.Errorf("stdout missing welcome message: %q", out)
	}
	if !strings.Contains(out, "Choose a config template:") {
		t.Errorf("stdout missing template prompt: %q", out)
	}
	if !strings.Contains(out, "Choose your coding agent:") {
		t.Errorf("stdout missing agent prompt: %q", out)
	}
}

func TestRunWizardNilStdin(t *testing.T) {
	var stdout bytes.Buffer
	wiz := runWizard(nil, &stdout)

	if wiz.interactive {
		t.Error("expected interactive = false for nil stdin")
	}
	if wiz.configName != "hello-world" {
		t.Errorf("configName = %q, want %q", wiz.configName, "hello-world")
	}
	if wiz.provider != "" {
		t.Errorf("provider = %q, want empty", wiz.provider)
	}
	// No prompts should be printed.
	if stdout.Len() > 0 {
		t.Errorf("unexpected stdout for nil stdin: %q", stdout.String())
	}
}

func TestRunWizardSelectGemini(t *testing.T) {
	// Default template + Gemini CLI.
	stdin := strings.NewReader("\nGemini CLI\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "gemini" {
		t.Errorf("provider = %q, want %q", wiz.provider, "gemini")
	}
}

func TestRunWizardSelectCodex(t *testing.T) {
	// Default template + Codex by number.
	stdin := strings.NewReader("\n2\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "codex" {
		t.Errorf("provider = %q, want %q", wiz.provider, "codex")
	}
}

func TestRunWizardCustomTemplate(t *testing.T) {
	// Select custom template → skips agent question, returns minimal config.
	stdin := strings.NewReader("2\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.configName != "custom" {
		t.Errorf("configName = %q, want %q", wiz.configName, "custom")
	}
	if wiz.provider != "" {
		t.Errorf("provider = %q, want empty for custom", wiz.provider)
	}
	if wiz.startCommand != "" {
		t.Errorf("startCommand = %q, want empty for custom", wiz.startCommand)
	}
	// Agent prompt should NOT appear.
	out := stdout.String()
	if strings.Contains(out, "Choose your coding agent:") {
		t.Errorf("stdout should not contain agent prompt for custom template: %q", out)
	}
}

func TestRunWizardSelectCursorByNumber(t *testing.T) {
	// Cursor is #4 in the order.
	stdin := strings.NewReader("\n4\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "cursor" {
		t.Errorf("provider = %q, want %q", wiz.provider, "cursor")
	}
}

func TestRunWizardSelectCopilotByName(t *testing.T) {
	stdin := strings.NewReader("\nGitHub Copilot\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "copilot" {
		t.Errorf("provider = %q, want %q", wiz.provider, "copilot")
	}
}

func TestRunWizardSelectByProviderKey(t *testing.T) {
	stdin := strings.NewReader("\namp\n")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "amp" {
		t.Errorf("provider = %q, want %q", wiz.provider, "amp")
	}
}

func TestRunWizardCustomCommand(t *testing.T) {
	// Default template + custom command (last option = len(providers)+1).
	customNum := len(config.BuiltinProviderOrder()) + 1
	stdin := strings.NewReader(fmt.Sprintf("\n%d\nmy-agent --auto --skip-confirm\n", customNum))
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	if wiz.provider != "" {
		t.Errorf("provider = %q, want empty for custom command", wiz.provider)
	}
	if wiz.startCommand != "my-agent --auto --skip-confirm" {
		t.Errorf("startCommand = %q, want %q", wiz.startCommand, "my-agent --auto --skip-confirm")
	}
}

func TestRunWizardEOFStdin(t *testing.T) {
	stdin := strings.NewReader("")
	var stdout bytes.Buffer
	wiz := runWizard(stdin, &stdout)

	// EOF means default for both questions.
	if wiz.configName != "hello-world" {
		t.Errorf("configName = %q, want %q", wiz.configName, "hello-world")
	}
	if wiz.provider != "claude" {
		t.Errorf("provider = %q, want %q", wiz.provider, "claude")
	}
}

func TestDoInitWithWizardConfig(t *testing.T) {
	f := fsys.NewFake()
	wiz := wizardConfig{
		interactive: true,
		configName:  "hello-world",
		provider:    "claude",
	}

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/bright-lights", wiz, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	// Verify output message.
	out := stdout.String()
	if !strings.Contains(out, "Created hello-world config") {
		t.Errorf("stdout missing wizard message: %q", out)
	}

	// Verify written config has two agents and provider.
	data := f.Files[filepath.Join("/bright-lights", "city.toml")]
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if cfg.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", cfg.Workspace.Provider, "claude")
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg.Agents[1].Name, "worker")
	}
	// Verify provider appears in TOML.
	if !strings.Contains(string(data), `provider = "claude"`) {
		t.Errorf("city.toml missing provider:\n%s", data)
	}
}

func TestDoInitWithCustomCommand(t *testing.T) {
	f := fsys.NewFake()
	wiz := wizardConfig{
		interactive:  true,
		configName:   "hello-world",
		startCommand: "my-agent --auto",
	}

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/bright-lights", wiz, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Verify written config has start_command and no provider.
	data := f.Files[filepath.Join("/bright-lights", "city.toml")]
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if cfg.Workspace.StartCommand != "my-agent --auto" {
		t.Errorf("Workspace.StartCommand = %q, want %q", cfg.Workspace.StartCommand, "my-agent --auto")
	}
	if cfg.Workspace.Provider != "" {
		t.Errorf("Workspace.Provider = %q, want empty", cfg.Workspace.Provider)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
}

func TestDoInitWithCustomTemplate(t *testing.T) {
	f := fsys.NewFake()
	wiz := wizardConfig{
		interactive: true,
		configName:  "custom",
	}

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/my-city", wiz, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Custom template → DefaultCity (one mayor, no provider).
	data := f.Files[filepath.Join("/my-city", "city.toml")]
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Workspace.Provider != "" {
		t.Errorf("Workspace.Provider = %q, want empty", cfg.Workspace.Provider)
	}
}

// --- cmdInitFromTOMLFile ---

func TestCmdInitFromTOMLFileSuccess(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	t.Setenv("GC_DOLT", "skip")

	// Use real temp dirs since cmdInitFromTOMLFile calls initBeads which
	// uses real filesystem via beadsProvider.
	dir := t.TempDir()
	cityPath := filepath.Join(dir, "bright-lights")
	if err := os.MkdirAll(cityPath, 0o755); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(dir, "my-config.toml")
	tomlContent := []byte(`[workspace]
name = "placeholder"
provider = "claude"

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"

[[agents]]
name = "worker"

[agents.pool]
min = 0
max = 5
check = "echo 3"
`)
	if err := os.WriteFile(src, tomlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := cmdInitFromTOMLFile(fsys.OSFS{}, src, cityPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("cmdInitFromTOMLFile = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing welcome: %q", out)
	}
	if !strings.Contains(out, "bright-lights") {
		t.Errorf("stdout missing city name: %q", out)
	}
	if !strings.Contains(out, "my-config.toml") {
		t.Errorf("stdout missing source filename: %q", out)
	}

	// Verify city.toml was written with updated name.
	data, err := os.ReadFile(filepath.Join(cityPath, "city.toml"))
	if err != nil {
		t.Fatalf("reading city.toml: %v", err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if cfg.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q (should be overridden)", cfg.Workspace.Name, "bright-lights")
	}
	if cfg.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", cfg.Workspace.Provider, "claude")
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg.Agents[1].Name, "worker")
	}
	if cfg.Agents[1].Pool == nil {
		t.Fatal("Agents[1].Pool is nil, want non-nil")
	}
	if cfg.Agents[1].Pool.Max != 5 {
		t.Errorf("Agents[1].Pool.Max = %d, want 5", cfg.Agents[1].Pool.Max)
	}
}

func TestCmdInitFromTOMLFileNotFound(t *testing.T) {
	f := fsys.NewFake()
	var stderr bytes.Buffer
	code := cmdInitFromTOMLFile(f, "/nonexistent.toml", "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "reading") {
		t.Errorf("stderr = %q, want reading error", stderr.String())
	}
}

func TestCmdInitFromTOMLFileInvalidTOML(t *testing.T) {
	f := fsys.NewFake()
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(src, []byte("[[[invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := cmdInitFromTOMLFile(f, src, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "parsing") {
		t.Errorf("stderr = %q, want parsing error", stderr.String())
	}
}

func TestCmdInitFromTOMLFileAlreadyInitialized(t *testing.T) {
	f := fsys.NewFake()
	f.Dirs[filepath.Join("/city", ".gc")] = true

	dir := t.TempDir()
	src := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(src, []byte("[workspace]\nname = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := cmdInitFromTOMLFile(f, src, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already initialized") {
		t.Errorf("stderr = %q, want 'already initialized'", stderr.String())
	}
}

// --- gc stop (doStop with agent.Fake) ---

func TestDoStopOneAgentRunning(t *testing.T) {
	f := agent.NewFake("mayor", "gc-bright-lights-mayor")
	f.Running = true

	var stdout, stderr bytes.Buffer
	code := doStop([]agent.Agent{f}, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStop = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Stopped agent 'mayor'") {
		t.Errorf("stdout missing 'Stopped agent': %q", out)
	}
	if !strings.Contains(out, "gc-bright-lights-mayor") {
		t.Errorf("stdout missing session name: %q", out)
	}
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("stdout missing 'City stopped.': %q", out)
	}
}

func TestDoStopNoAgents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doStop(nil, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStop = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("stdout missing 'City stopped.': %q", out)
	}
	// Should not contain any "Stopped agent" messages.
	if strings.Contains(out, "Stopped agent") {
		t.Errorf("stdout should not contain 'Stopped agent' with no agents: %q", out)
	}
}

func TestDoStopAgentNotRunning(t *testing.T) {
	f := agent.NewFake("mayor", "gc-bright-lights-mayor")
	// Running defaults to false — agent is not running.

	var stdout, stderr bytes.Buffer
	code := doStop([]agent.Agent{f}, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStop = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("stdout missing 'City stopped.': %q", out)
	}
	// Should not contain "Stopped agent" since session wasn't running.
	if strings.Contains(out, "Stopped agent") {
		t.Errorf("stdout should not contain 'Stopped agent' for non-running session: %q", out)
	}
}

func TestDoStopMultipleAgents(t *testing.T) {
	mayor := agent.NewFake("mayor", "gc-city-mayor")
	mayor.Running = true
	worker := agent.NewFake("worker", "gc-city-worker")
	worker.Running = true

	var stdout, stderr bytes.Buffer
	code := doStop([]agent.Agent{mayor, worker}, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStop = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Stopped agent 'mayor'") {
		t.Errorf("stdout missing 'Stopped agent mayor': %q", out)
	}
	if !strings.Contains(out, "Stopped agent 'worker'") {
		t.Errorf("stdout missing 'Stopped agent worker': %q", out)
	}
	if !strings.Contains(out, "City stopped.") {
		t.Errorf("stdout missing 'City stopped.': %q", out)
	}
}

func TestDoStopStopError(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true
	f.StopErr = fmt.Errorf("session stuck")

	var stdout, stderr bytes.Buffer
	code := doStop([]agent.Agent{f}, events.Discard, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStop = %d, want 0 (errors are non-fatal); stderr: %s", code, stderr.String())
	}
	// Error reported to stderr.
	if !strings.Contains(stderr.String(), "session stuck") {
		t.Errorf("stderr = %q, want 'session stuck' error", stderr.String())
	}
	// Should still print "City stopped."
	if !strings.Contains(stdout.String(), "City stopped.") {
		t.Errorf("stdout missing 'City stopped.': %q", stdout.String())
	}
}

// --- doAgentAdd (with fsys.Fake) ---

func TestDoAgentAddSuccess(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doAgentAdd(f, "/city", "worker", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAdd = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Added agent 'worker'") {
		t.Errorf("stdout = %q, want 'Added agent'", stdout.String())
	}

	// Verify the written config has both agents.
	written := f.Files[filepath.Join("/city", "city.toml")]
	got, err := config.Parse(written)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(got.Agents))
	}
	if got.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", got.Agents[0].Name, "mayor")
	}
	if got.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", got.Agents[1].Name, "worker")
	}
}

func TestDoAgentAddDuplicate(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stderr bytes.Buffer
	code := doAgentAdd(f, "/city", "mayor", "", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("stderr = %q, want 'already exists'", stderr.String())
	}
}

func TestDoAgentAddLoadFails(t *testing.T) {
	f := fsys.NewFake()
	// No city.toml → load fails.

	var stderr bytes.Buffer
	code := doAgentAdd(f, "/city", "worker", "", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc agent add") {
		t.Errorf("stderr = %q, want 'gc agent add' prefix", stderr.String())
	}
}

// --- doAgentList (with fsys.Fake) ---

func TestDoAgentListSuccess(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	cfg.Agents = append(cfg.Agents, config.Agent{Name: "worker"})
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doAgentList(f, "/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bright-lights:") {
		t.Errorf("stdout missing 'bright-lights:': %q", out)
	}
	if !strings.Contains(out, "  mayor") {
		t.Errorf("stdout missing '  mayor': %q", out)
	}
	if !strings.Contains(out, "  worker") {
		t.Errorf("stdout missing '  worker': %q", out)
	}
}

func TestDoAgentListSingleAgent(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doAgentList(f, "/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentList = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bright-lights:") {
		t.Errorf("stdout missing 'bright-lights:': %q", out)
	}
	if !strings.Contains(out, "  mayor") {
		t.Errorf("stdout missing '  mayor': %q", out)
	}
}

func TestDoAgentListLoadFails(t *testing.T) {
	f := fsys.NewFake()
	// No city.toml → load fails.

	var stderr bytes.Buffer
	code := doAgentList(f, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentList = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc agent list") {
		t.Errorf("stderr = %q, want 'gc agent list' prefix", stderr.String())
	}
}

// --- doAgentClaimed ---

func TestDoAgentClaimedMissingAgent(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doAgentClaimed(store, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentClaimed(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing agent name") {
		t.Errorf("stderr = %q, want 'missing agent name'", stderr.String())
	}
}

func TestDoAgentClaimedNotFound(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doAgentClaimed(store, []string{"worker"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentClaimed(worker) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestDoAgentClaimedSuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Print hello"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim("gc-1", "worker"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doAgentClaimed(store, []string{"worker"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentClaimed = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ID:       gc-1",
		"Status:   in_progress",
		"Title:    Print hello",
		"Assignee: worker",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

// --- readPromptFile ---

func TestReadPromptFileEmptyPath(t *testing.T) {
	f := fsys.NewFake()
	got := readPromptFile(f, "/city", "")
	if got != "" {
		t.Errorf("readPromptFile(empty) = %q, want empty", got)
	}
}

func TestReadPromptFileMissing(t *testing.T) {
	f := fsys.NewFake()
	got := readPromptFile(f, "/city", "prompts/mayor.md")
	if got != "" {
		t.Errorf("readPromptFile(missing) = %q, want empty", got)
	}
}

func TestReadPromptFileSuccess(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/mayor.md"] = []byte("You are the mayor.")
	got := readPromptFile(f, "/city", "prompts/mayor.md")
	if got != "You are the mayor." {
		t.Errorf("readPromptFile = %q, want %q", got, "You are the mayor.")
	}
}

// --- doAgentAdd with --prompt-template ---

// --- mergeEnv ---

func TestMergeEnvNil(t *testing.T) {
	got := mergeEnv(nil, nil)
	if got != nil {
		t.Errorf("mergeEnv(nil, nil) = %v, want nil", got)
	}
}

func TestMergeEnvSingle(t *testing.T) {
	got := mergeEnv(map[string]string{"A": "1"})
	if got["A"] != "1" {
		t.Errorf("got[A] = %q, want %q", got["A"], "1")
	}
}

func TestMergeEnvOverride(t *testing.T) {
	got := mergeEnv(
		map[string]string{"A": "base", "B": "keep"},
		map[string]string{"A": "override", "C": "new"},
	)
	if got["A"] != "override" {
		t.Errorf("got[A] = %q, want %q (later map wins)", got["A"], "override")
	}
	if got["B"] != "keep" {
		t.Errorf("got[B] = %q, want %q", got["B"], "keep")
	}
	if got["C"] != "new" {
		t.Errorf("got[C] = %q, want %q", got["C"], "new")
	}
}

func TestMergeEnvProviderEnvFlowsThrough(t *testing.T) {
	// Simulate what cmd_start does: provider env + GC_AGENT.
	providerEnv := map[string]string{"OPENCODE_PERMISSION": `{"*":"allow"}`}
	got := mergeEnv(providerEnv, map[string]string{"GC_AGENT": "worker"})
	if got["OPENCODE_PERMISSION"] != `{"*":"allow"}` {
		t.Errorf("provider env lost: %v", got)
	}
	if got["GC_AGENT"] != "worker" {
		t.Errorf("GC_AGENT lost: %v", got)
	}
}

// --- resolveAgentChoice ---

func TestResolveAgentChoiceEmpty(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	got := resolveAgentChoice("", order, builtins, len(order)+1)
	if got != order[0] {
		t.Errorf("resolveAgentChoice('') = %q, want %q", got, order[0])
	}
}

func TestResolveAgentChoiceByNumber(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	got := resolveAgentChoice("2", order, builtins, len(order)+1)
	if got != order[1] {
		t.Errorf("resolveAgentChoice('2') = %q, want %q", got, order[1])
	}
}

func TestResolveAgentChoiceByDisplayName(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	got := resolveAgentChoice("Gemini CLI", order, builtins, len(order)+1)
	if got != "gemini" {
		t.Errorf("resolveAgentChoice('Gemini CLI') = %q, want %q", got, "gemini")
	}
}

func TestResolveAgentChoiceByKey(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	got := resolveAgentChoice("amp", order, builtins, len(order)+1)
	if got != "amp" {
		t.Errorf("resolveAgentChoice('amp') = %q, want %q", got, "amp")
	}
}

func TestResolveAgentChoiceOutOfRange(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	customNum := len(order) + 1

	for _, input := range []string{"0", "-1", "99", fmt.Sprintf("%d", customNum)} {
		got := resolveAgentChoice(input, order, builtins, customNum)
		if got != "" {
			t.Errorf("resolveAgentChoice(%q) = %q, want empty", input, got)
		}
	}
}

func TestResolveAgentChoiceUnknown(t *testing.T) {
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()
	got := resolveAgentChoice("vim", order, builtins, len(order)+1)
	if got != "" {
		t.Errorf("resolveAgentChoice('vim') = %q, want empty", got)
	}
}

func TestDoAgentAddWithPromptTemplate(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doAgentAdd(f, "/city", "worker", "prompts/worker.md", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAdd = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Verify the written config has the prompt_template.
	written := f.Files[filepath.Join("/city", "city.toml")]
	got, err := config.Parse(written)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(got.Agents))
	}
	if got.Agents[1].PromptTemplate != "prompts/worker.md" {
		t.Errorf("Agents[1].PromptTemplate = %q, want %q", got.Agents[1].PromptTemplate, "prompts/worker.md")
	}
}

// --- doAgentUnclaim ---

func TestDoAgentUnclaimSuccess(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "Print hello"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim("gc-1", "worker"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doAgentUnclaim(store, events.Discard, "worker", "gc-1", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentUnclaim = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Unclaimed bead 'gc-1' from agent 'worker'") {
		t.Errorf("stdout = %q, want unclaim message", out)
	}

	// Verify bead state.
	b, err := store.Get("gc-1")
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "open" {
		t.Errorf("bead status = %q, want %q", b.Status, "open")
	}
	if b.Assignee != "" {
		t.Errorf("bead assignee = %q, want empty", b.Assignee)
	}
}

func TestDoAgentUnclaimNotFound(t *testing.T) {
	store := beads.NewMemStore()

	var stderr bytes.Buffer
	code := doAgentUnclaim(store, events.Discard, "worker", "gc-999", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentUnclaim = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bead not found") {
		t.Errorf("stderr = %q, want 'bead not found'", stderr.String())
	}
}

func TestDoAgentUnclaimWrongAgent(t *testing.T) {
	store := beads.NewMemStore()
	if _, err := store.Create(beads.Bead{Title: "contested"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Claim("gc-1", "worker"); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := doAgentUnclaim(store, events.Discard, "builder", "gc-1", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentUnclaim = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already claimed") {
		t.Errorf("stderr = %q, want conflict message", stderr.String())
	}
}

// --- doAgentNudge ---

func TestDoAgentNudgeSuccess(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")

	var stdout, stderr bytes.Buffer
	code := doAgentNudge(f, "wake up", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentNudge = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Nudged agent 'mayor'") {
		t.Errorf("stdout = %q, want nudge message", out)
	}

	// Verify the Fake recorded the nudge call.
	var found bool
	for _, c := range f.Calls {
		if c.Method == "Nudge" {
			found = true
			if c.Message != "wake up" {
				t.Errorf("Nudge Message = %q, want %q", c.Message, "wake up")
			}
		}
	}
	if !found {
		t.Error("Nudge call not recorded on agent fake")
	}
}

func TestDoAgentNudgeBrokenProvider(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.NudgeErr = fmt.Errorf("session unavailable")

	var stderr bytes.Buffer
	code := doAgentNudge(f, "wake up", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentNudge = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "session unavailable") {
		t.Errorf("stderr = %q, want 'session unavailable'", stderr.String())
	}
}
