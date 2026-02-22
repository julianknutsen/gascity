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

// --- gc bead close ---

func TestBeadCloseMissingID(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadClose(store, nil, &bytes.Buffer{}, &stderr)
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
	code := doBeadClose(store, []string{"gc-999"}, &bytes.Buffer{}, &stderr)
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
	code := doBeadClose(store, []string{"gc-1"}, &stdout, &stderr)
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

// --- gc bead create ---

func TestBeadCreateMissingTitle(t *testing.T) {
	var stderr bytes.Buffer
	store := beads.NewMemStore()
	code := doBeadCreate(store, nil, &bytes.Buffer{}, &stderr)
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
	code := doBeadCreate(store, []string{"Build a Tower of Hanoi app"}, &stdout, &stderr)
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

// --- gc bead list ---

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

// --- gc bead ready ---

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

// --- gc bead show ---

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

// --- detectProvider ---

func TestDetectProviderClaude(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := detectProvider(lookPath)
	if err != nil {
		t.Fatalf("detectProvider: %v", err)
	}
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("cmd = %q, want claude command", cmd)
	}
}

func TestDetectProviderFallbackToCodex(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "codex" {
			return "/usr/bin/codex", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := detectProvider(lookPath)
	if err != nil {
		t.Fatalf("detectProvider: %v", err)
	}
	if !strings.Contains(cmd, "codex") {
		t.Errorf("cmd = %q, want codex command", cmd)
	}
}

func TestDetectProviderNoneFound(t *testing.T) {
	lookPath := func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	_, err := detectProvider(lookPath)
	if err == nil {
		t.Fatal("expected error when no provider found")
	}
	if !strings.Contains(err.Error(), "no supported agent CLI found") {
		t.Errorf("error = %q, want 'no supported agent CLI found'", err)
	}
}

// --- resolveProvider ---

func TestResolveProviderClaude(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := resolveProvider("claude", lookPath)
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("cmd = %q, want claude command", cmd)
	}
}

func TestResolveProviderCodex(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "codex" {
			return "/usr/bin/codex", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := resolveProvider("codex", lookPath)
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	if cmd != "codex --dangerously-bypass-approvals-and-sandbox" {
		t.Errorf("cmd = %q, want codex command", cmd)
	}
}

func TestResolveProviderNotInPath(t *testing.T) {
	lookPath := func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	_, err := resolveProvider("codex", lookPath)
	if err == nil {
		t.Fatal("expected error when provider not in PATH")
	}
	if !strings.Contains(err.Error(), `provider "codex" not found in PATH`) {
		t.Errorf("error = %q, want 'provider not found in PATH'", err)
	}
}

func TestResolveProviderUnknown(t *testing.T) {
	lookPath := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	_, err := resolveProvider("vim", lookPath)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), `unknown provider "vim"`) {
		t.Errorf("error = %q, want 'unknown provider'", err)
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
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true
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
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true
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
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true
	f.Dirs[filepath.Join("/projects/myapp", ".git")] = true

	var stdout, stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &stdout, &stderr)
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
	f := fsys.NewFake()
	f.Dirs["/projects/myapp"] = true

	var stdout, stderr bytes.Buffer
	code := doRigAdd(f, "/city", "/projects/myapp", &stdout, &stderr)
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

func TestDoRigListReadDirFails(t *testing.T) {
	f := fsys.NewFake()
	f.Errors[filepath.Join("/city", "rigs")] = fmt.Errorf("no such directory")

	var stderr bytes.Buffer
	code := doRigList(f, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doRigList = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no such directory") {
		t.Errorf("stderr = %q, want 'no such directory'", stderr.String())
	}
}

func TestDoRigListSuccess(t *testing.T) {
	f := fsys.NewFake()
	f.Dirs[filepath.Join("/city", "rigs", "alpha")] = true
	f.Dirs[filepath.Join("/city", "rigs", "beta")] = true

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
	code := doInit(f, "/bright-lights", &stdout, &stderr)
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

	// Verify .gc/ and rigs/ were created.
	if !f.Dirs[filepath.Join("/bright-lights", ".gc")] {
		t.Error(".gc/ not created")
	}
	if !f.Dirs[filepath.Join("/bright-lights", "rigs")] {
		t.Error("rigs/ not created")
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
}

func TestDoInitWritesExpectedTOML(t *testing.T) {
	f := fsys.NewFake()

	var stdout, stderr bytes.Buffer
	code := doInit(f, "/bright-lights", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doInit = %d, want 0; stderr: %s", code, stderr.String())
	}

	got := string(f.Files[filepath.Join("/bright-lights", "city.toml")])
	want := `[workspace]
name = "bright-lights"

[[agents]]
name = "mayor"
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
	code := doInit(f, "/city", &bytes.Buffer{}, &stderr)
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
	code := doInit(f, "/city", &bytes.Buffer{}, &stderr)
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
	code := doInit(f, "/city", &bytes.Buffer{}, &stderr)
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
	code := doInit(f, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doInit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "read-only fs") {
		t.Errorf("stderr = %q, want 'read-only fs'", stderr.String())
	}
}

// --- gc stop (doStop with agent.Fake) ---

func TestDoStopOneAgentRunning(t *testing.T) {
	f := agent.NewFake("mayor", "gc-bright-lights-mayor")
	f.Running = true

	var stdout, stderr bytes.Buffer
	code := doStop([]agent.Agent{f}, &stdout, &stderr)
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
	code := doStop(nil, &stdout, &stderr)
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
	code := doStop([]agent.Agent{f}, &stdout, &stderr)
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
	code := doStop([]agent.Agent{mayor, worker}, &stdout, &stderr)
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

// --- resolveAgentCommand ---

func TestResolveAgentCommandExplicit(t *testing.T) {
	agent := &config.Agent{Name: "mayor", StartCommand: "my-custom-cli --flag"}
	lookPath := func(string) (string, error) { return "", fmt.Errorf("not found") }
	cmd, err := resolveAgentCommand(agent, lookPath)
	if err != nil {
		t.Fatalf("resolveAgentCommand: %v", err)
	}
	if cmd != "my-custom-cli --flag" {
		t.Errorf("cmd = %q, want %q", cmd, "my-custom-cli --flag")
	}
}

func TestResolveAgentCommandProvider(t *testing.T) {
	agent := &config.Agent{Name: "mayor", Provider: "claude"}
	lookPath := func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := resolveAgentCommand(agent, lookPath)
	if err != nil {
		t.Fatalf("resolveAgentCommand: %v", err)
	}
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("cmd = %q, want claude command", cmd)
	}
}

func TestResolveAgentCommandAutoDetect(t *testing.T) {
	agent := &config.Agent{Name: "mayor"}
	lookPath := func(name string) (string, error) {
		if name == "codex" {
			return "/usr/bin/codex", nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
	cmd, err := resolveAgentCommand(agent, lookPath)
	if err != nil {
		t.Fatalf("resolveAgentCommand: %v", err)
	}
	if !strings.Contains(cmd, "codex") {
		t.Errorf("cmd = %q, want codex command", cmd)
	}
}

func TestResolveAgentCommandNoProvider(t *testing.T) {
	agent := &config.Agent{Name: "mayor"}
	lookPath := func(string) (string, error) { return "", fmt.Errorf("not found") }
	_, err := resolveAgentCommand(agent, lookPath)
	if err == nil {
		t.Fatal("expected error when no provider found")
	}
	if !strings.Contains(err.Error(), "no supported agent CLI found") {
		t.Errorf("error = %q, want 'no supported agent CLI found'", err)
	}
}

func TestResolveAgentCommandStartCommandWinsOverProvider(t *testing.T) {
	agent := &config.Agent{Name: "mayor", StartCommand: "custom-cmd", Provider: "claude"}
	lookPath := func(string) (string, error) { return "", fmt.Errorf("not found") }
	cmd, err := resolveAgentCommand(agent, lookPath)
	if err != nil {
		t.Fatalf("resolveAgentCommand: %v", err)
	}
	// start_command should win even though provider is set.
	if cmd != "custom-cmd" {
		t.Errorf("cmd = %q, want %q", cmd, "custom-cmd")
	}
}

// --- doStartAgents ---

func TestDoStartAgentsSuccess(t *testing.T) {
	f := agent.NewFake("mayor", "gc-bright-lights-mayor")

	var stdout, stderr bytes.Buffer
	code := doStartAgents([]agent.Agent{f}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStartAgents = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Started agent 'mayor' (session: gc-bright-lights-mayor)") {
		t.Errorf("stdout missing start message: %q", out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}

	// Verify call sequence: IsRunning → Start → Name → SessionName.
	want := []string{"IsRunning", "Start", "Name", "SessionName"}
	if len(f.Calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %+v", len(f.Calls), len(want), f.Calls)
	}
	for i, c := range f.Calls {
		if c.Method != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c.Method, want[i])
		}
	}
}

func TestDoStartAgentsAlreadyRunning(t *testing.T) {
	f := agent.NewFake("mayor", "gc-bright-lights-mayor")
	f.Running = true

	var stdout, stderr bytes.Buffer
	code := doStartAgents([]agent.Agent{f}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStartAgents = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	// Should NOT contain "Started agent" since it was already running.
	if strings.Contains(out, "Started agent") {
		t.Errorf("stdout should not contain 'Started agent' for already-running session: %q", out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}

	// Only IsRunning should have been called — no Start.
	if len(f.Calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(f.Calls), f.Calls)
	}
	if f.Calls[0].Method != "IsRunning" {
		t.Errorf("call 0: got %q, want %q", f.Calls[0].Method, "IsRunning")
	}
}

func TestDoStartAgentsMultiple(t *testing.T) {
	mayor := agent.NewFake("mayor", "gc-city-mayor")
	worker := agent.NewFake("worker", "gc-city-worker")

	var stdout, stderr bytes.Buffer
	code := doStartAgents([]agent.Agent{mayor, worker}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStartAgents = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Started agent 'mayor'") {
		t.Errorf("stdout missing 'Started agent mayor': %q", out)
	}
	if !strings.Contains(out, "Started agent 'worker'") {
		t.Errorf("stdout missing 'Started agent worker': %q", out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}
}

func TestDoStartAgentsStartError(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.StartErr = fmt.Errorf("boom")

	var stdout, stderr bytes.Buffer
	code := doStartAgents([]agent.Agent{f}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStartAgents = %d, want 0 (errors are non-fatal); stderr: %s", code, stderr.String())
	}

	// Error is reported to stderr.
	if !strings.Contains(stderr.String(), "starting mayor") {
		t.Errorf("stderr = %q, want 'starting mayor' error", stderr.String())
	}
	// Should still print "City started."
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.': %q", stdout.String())
	}
}

func TestDoStartAgentsEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doStartAgents(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doStartAgents = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "City started.") {
		t.Errorf("stdout missing 'City started.': %q", stdout.String())
	}
}
