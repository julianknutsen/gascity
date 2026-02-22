package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/steveyegge/gascity/internal/beads"
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

// --- run ---

func TestRunNoArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run(nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no command specified") {
		t.Errorf("stderr = %q, want 'no command specified'", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"blorp"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([blorp]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown command "blorp"`) {
		t.Errorf("stderr = %q, want 'unknown command'", stderr.String())
	}
}

// --- gc start ---

func TestStartNoPath(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"start"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([start]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing city path") {
		t.Errorf("stderr = %q, want 'missing city path'", stderr.String())
	}
}

func TestStartSuccess(t *testing.T) {
	cityDir := filepath.Join(t.TempDir(), "my-city")

	var stdout, stderr bytes.Buffer
	code := run([]string{"start", cityDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([start dir]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Welcome to Gas City!") {
		t.Errorf("stdout missing welcome message: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "city.toml") {
		t.Errorf("stdout missing city.toml reference: %q", stdout.String())
	}

	// Verify directory structure.
	for _, sub := range []string{".gc", "rigs"} {
		p := filepath.Join(cityDir, sub)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("%s: %v", sub, err)
		} else if !fi.IsDir() {
			t.Errorf("%s: not a directory", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(cityDir, "city.toml")); err != nil {
		t.Errorf("city.toml: %v", err)
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

// --- gc rig ---

func TestRigNoSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"rig"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing subcommand") {
		t.Errorf("stderr = %q, want 'missing subcommand'", stderr.String())
	}
}

func TestRigUnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"rig", "blorp"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig blorp]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown subcommand "blorp"`) {
		t.Errorf("stderr = %q, want 'unknown subcommand'", stderr.String())
	}
}

// --- gc rig add ---

func TestRigAddMissingPath(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stderr bytes.Buffer
	code := run([]string{"rig", "add"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig add]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing path") {
		t.Errorf("stderr = %q, want 'missing path'", stderr.String())
	}
}

func TestRigAddNotInCity(t *testing.T) {
	dir := t.TempDir() // no .gc/
	t.Chdir(dir)

	var stderr bytes.Buffer
	code := run([]string{"rig", "add", dir}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig add]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not in a city directory") {
		t.Errorf("stderr = %q, want 'not in a city directory'", stderr.String())
	}
}

func TestRigAddPathNotExist(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stderr bytes.Buffer
	code := run([]string{"rig", "add", filepath.Join(city, "nonexistent")}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig add nonexistent]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no such file") {
		t.Errorf("stderr = %q, want 'no such file'", stderr.String())
	}
}

func TestRigAddNotADirectory(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	// Create a file (not a directory).
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := run([]string{"rig", "add", f}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig add file]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "is not a directory") {
		t.Errorf("stderr = %q, want 'is not a directory'", stderr.String())
	}
}

func TestRigAddSuccess(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	// Create a project directory with .git.
	project := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"rig", "add", project}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([rig add project]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Adding rig 'my-project'...") {
		t.Errorf("stdout missing 'Adding rig': %q", out)
	}
	if !strings.Contains(out, "Detected git repo") {
		t.Errorf("stdout missing 'Detected git repo': %q", out)
	}
	if !strings.Contains(out, "Rig added.") {
		t.Errorf("stdout missing 'Rig added.': %q", out)
	}

	// Verify rig.toml was created.
	rigToml := filepath.Join(city, "rigs", "my-project", "rig.toml")
	data, err := os.ReadFile(rigToml)
	if err != nil {
		t.Fatalf("reading rig.toml: %v", err)
	}
	if !strings.Contains(string(data), project) {
		t.Errorf("rig.toml = %q, want path %q", data, project)
	}
}

func TestRigAddNoGit(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	// Create a project directory without .git.
	project := filepath.Join(t.TempDir(), "no-git-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"rig", "add", project}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([rig add project]) = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, "Detected git repo") {
		t.Errorf("stdout should not contain 'Detected git repo' for non-git dir: %q", out)
	}
	if !strings.Contains(out, "Rig added.") {
		t.Errorf("stdout missing 'Rig added.': %q", out)
	}
}

// --- gc rig list ---

func TestRigListNotInCity(t *testing.T) {
	dir := t.TempDir() // no .gc/
	t.Chdir(dir)

	var stderr bytes.Buffer
	code := run([]string{"rig", "list"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([rig list]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not in a city directory") {
		t.Errorf("stderr = %q, want 'not in a city directory'", stderr.String())
	}
}

func TestRigListEmpty(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stdout, stderr bytes.Buffer
	code := run([]string{"rig", "list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([rig list]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Rigs in") {
		t.Errorf("stdout missing 'Rigs in': %q", out)
	}
	cityName := filepath.Base(city)
	if !strings.Contains(out, cityName+":") {
		t.Errorf("stdout missing city name entry %q: %q", cityName+":", out)
	}
	if !strings.Contains(out, "Agents: [mayor]") {
		t.Errorf("stdout missing 'Agents: [mayor]': %q", out)
	}
}

func TestRigListSuccess(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs", "my-rig"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stdout, stderr bytes.Buffer
	code := run([]string{"rig", "list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([rig list]) = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Rigs in") {
		t.Errorf("stdout missing 'Rigs in': %q", out)
	}
	cityName := filepath.Base(city)
	if !strings.Contains(out, cityName+":") {
		t.Errorf("stdout missing city name entry %q: %q", cityName+":", out)
	}
	if !strings.Contains(out, "Agents: [mayor]") {
		t.Errorf("stdout missing 'Agents: [mayor]': %q", out)
	}
	if !strings.Contains(out, "my-rig:") {
		t.Errorf("stdout missing 'my-rig:': %q", out)
	}
	if !strings.Contains(out, "Agents: []") {
		t.Errorf("stdout missing 'Agents: []' for rig entry: %q", out)
	}
}

// --- gc bead ---

func TestBeadNoSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"bead"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([bead]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing subcommand") {
		t.Errorf("stderr = %q, want 'missing subcommand'", stderr.String())
	}
}

func TestBeadUnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"bead", "blorp"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([bead blorp]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown subcommand "blorp"`) {
		t.Errorf("stderr = %q, want 'unknown subcommand'", stderr.String())
	}
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
