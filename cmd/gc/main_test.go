package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
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
	var stdout bytes.Buffer
	code := run(nil, &stdout, &bytes.Buffer{})
	if code != 0 {
		t.Errorf("run(nil) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "Available Commands") {
		t.Errorf("stdout missing help text: %q", stdout.String())
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

// --- gc start ---

func TestStartAutoInit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"start"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([start]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing welcome: %q", out)
	}
	if !strings.Contains(out, "Initialized city") {
		t.Errorf("stdout missing 'Initialized city': %q", out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}

	// Verify directory structure.
	for _, sub := range []string{".gc", "rigs"} {
		p := filepath.Join(dir, sub)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("%s: %v", sub, err)
		} else if !fi.IsDir() {
			t.Errorf("%s: not a directory", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "city.toml")); err != nil {
		t.Errorf("city.toml: %v", err)
	}
}

func TestStartExistingCity(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(city, "rigs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"
	if err := os.WriteFile(filepath.Join(city, "city.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stdout, stderr bytes.Buffer
	code := run([]string{"start"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([start]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}
	// Should NOT contain welcome/init output since city already exists.
	if strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout should not contain welcome for existing city: %q", out)
	}
}

func TestStartWithPath(t *testing.T) {
	cityDir := filepath.Join(t.TempDir(), "my-city")

	var stdout, stderr bytes.Buffer
	code := run([]string{"start", cityDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([start path]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing welcome: %q", out)
	}
	if !strings.Contains(out, "my-city") {
		t.Errorf("stdout missing city name: %q", out)
	}
	if !strings.Contains(out, "City started.") {
		t.Errorf("stdout missing 'City started.': %q", out)
	}

	// Verify directory structure at path.
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

// --- gc agent ---

func TestAgentNoSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"agent"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([agent]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing subcommand") {
		t.Errorf("stderr = %q, want 'missing subcommand'", stderr.String())
	}
}

func TestAgentUnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"agent", "blorp"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([agent blorp]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown subcommand "blorp"`) {
		t.Errorf("stderr = %q, want 'unknown subcommand'", stderr.String())
	}
}

func TestAgentAttachMissingName(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"agent", "attach"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([agent attach]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing agent name") {
		t.Errorf("stderr = %q, want 'missing agent name'", stderr.String())
	}
}

// --- doAgentAttach ---

func TestAgentAttachStartsAndAttaches(t *testing.T) {
	f := session.NewFake()

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, "mayor", "claude --dangerously-skip-permissions", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAttach = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Attaching to agent 'mayor'...") {
		t.Errorf("stdout = %q, want attach message", stdout.String())
	}

	// Verify call sequence: IsRunning → Start → Attach.
	want := []string{"IsRunning", "Start", "Attach"}
	if len(f.Calls) != len(want) {
		t.Fatalf("got %d calls, want %d: %+v", len(f.Calls), len(want), f.Calls)
	}
	for i, c := range f.Calls {
		if c.Method != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c.Method, want[i])
		}
		if c.Name != "mayor" {
			t.Errorf("call %d: name = %q, want %q", i, c.Name, "mayor")
		}
	}

	// Verify the command was passed through to Start.
	startCall := f.Calls[1]
	if startCall.Config.Command != "claude --dangerously-skip-permissions" {
		t.Errorf("Start Config.Command = %q, want %q", startCall.Config.Command, "claude --dangerously-skip-permissions")
	}
}

func TestAgentAttachExistingSession(t *testing.T) {
	f := session.NewFake()
	// Pre-create the session so IsRunning returns true.
	_ = f.Start("mayor", session.Config{})
	f.Calls = nil // reset spy

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, "mayor", "claude --dangerously-skip-permissions", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doAgentAttach = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Should skip Start: IsRunning → Attach only.
	want := []string{"IsRunning", "Attach"}
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
	f := session.NewFake()
	// Make Start fail by pre-creating the session (duplicate name error).
	_ = f.Start("mayor", session.Config{})
	_ = f.Stop("mayor") // remove it so IsRunning returns false
	f.Calls = nil

	// Re-create to cause the "already exists" error on the next Start.
	_ = f.Start("mayor", session.Config{})
	f.Calls = nil

	// IsRunning=false (we'll trick it), Start=error.
	// Actually, Fake's IsRunning checks the map. The session exists, so
	// IsRunning=true and Start won't be called. Let me use a different approach:
	// Stop first, then Start will succeed. We need to inject an error differently.
	// The cleanest way: stop it, then pre-create so Start fails on duplicate.
	// But that means IsRunning=true. Instead, let's just verify the error path
	// by having Start fail when the session doesn't exist. We need a custom fake.

	// Simpler: use a wrapper that forces Start to fail.
	var stderr bytes.Buffer
	code := doAgentAttach(&startErrorProvider{}, "mayor", "claude --dangerously-skip-permissions", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doAgentAttach = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "starting session") {
		t.Errorf("stderr = %q, want 'starting session' error", stderr.String())
	}
}

func TestAgentAttachAttachError(t *testing.T) {
	var stderr bytes.Buffer
	code := doAgentAttach(&attachErrorProvider{}, "mayor", "claude --dangerously-skip-permissions", &bytes.Buffer{}, &stderr)
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

// --- gc stop (doStop with session.Fake) ---

func TestDoStopOneAgentRunning(t *testing.T) {
	f := session.NewFake()
	_ = f.Start("gc-bright-lights-mayor", session.Config{})
	f.Calls = nil // reset spy

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}

	var stdout, stderr bytes.Buffer
	code := doStop(f, cfg, "bright-lights", &stdout, &stderr)
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
	f := session.NewFake()
	cfg := &config.City{}

	var stdout, stderr bytes.Buffer
	code := doStop(f, cfg, "empty-city", &stdout, &stderr)
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
	f := session.NewFake()
	// Don't start any sessions — agent is not running.
	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}

	var stdout, stderr bytes.Buffer
	code := doStop(f, cfg, "bright-lights", &stdout, &stderr)
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
	f := session.NewFake()
	_ = f.Start("gc-city-mayor", session.Config{})
	_ = f.Start("gc-city-worker", session.Config{})
	f.Calls = nil

	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
		},
	}

	var stdout, stderr bytes.Buffer
	code := doStop(f, cfg, "city", &stdout, &stderr)
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

// --- gc agent attach: no agents configured ---

func TestAgentAttachNoAgentsConfigured(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Bare city.toml with no agents.
	if err := os.WriteFile(filepath.Join(city, "city.toml"), []byte("# bare config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stderr bytes.Buffer
	code := run([]string{"agent", "attach", "mayor"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([agent attach mayor]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no agents configured") {
		t.Errorf("stderr = %q, want 'no agents configured'", stderr.String())
	}
	if !strings.Contains(stderr.String(), "gc init") {
		t.Errorf("stderr = %q, want 'gc init' guidance", stderr.String())
	}
}

func TestAgentAttachAgentNotFound(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[workspace]\nname = \"test\"\n\n[[agents]]\nname = \"mayor\"\n"
	if err := os.WriteFile(filepath.Join(city, "city.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stderr bytes.Buffer
	code := run([]string{"agent", "attach", "nonexistent"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([agent attach nonexistent]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `agent "nonexistent" not found`) {
		t.Errorf("stderr = %q, want agent not found error", stderr.String())
	}
}

// --- gc stop: integration via run() ---

func TestStopNotInCity(t *testing.T) {
	dir := t.TempDir() // no .gc/
	t.Chdir(dir)

	var stderr bytes.Buffer
	code := run([]string{"stop"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([stop]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not in a city directory") {
		t.Errorf("stderr = %q, want 'not in a city directory'", stderr.String())
	}
}

// --- gc init: integration via run() ---

func TestInitInCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([init]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing 'Welcome to Gas City!': %q", out)
	}
	if !strings.Contains(out, "Initialized city") {
		t.Errorf("stdout missing 'Initialized city': %q", out)
	}

	// Verify config was written correctly.
	data, err := os.ReadFile(filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("reading city.toml: %v", err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("parsing city.toml: %v", err)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "mayor" {
		t.Errorf("config agents = %+v, want [mayor]", cfg.Agents)
	}
}

func TestInitWithPath(t *testing.T) {
	cityPath := filepath.Join(t.TempDir(), "bright-lights")

	var stdout, stderr bytes.Buffer
	code := run([]string{"init", cityPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run([init path]) = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Welcome to Gas City!") {
		t.Errorf("stdout missing 'Welcome to Gas City!': %q", out)
	}
	if !strings.Contains(out, "bright-lights") {
		t.Errorf("stdout missing city name: %q", out)
	}

	// Verify directory structure.
	for _, sub := range []string{".gc", "rigs"} {
		p := filepath.Join(cityPath, sub)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("%s: %v", sub, err)
		} else if !fi.IsDir() {
			t.Errorf("%s: not a directory", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(cityPath, "city.toml")); err != nil {
		t.Errorf("city.toml: %v", err)
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	city := t.TempDir()
	if err := os.MkdirAll(filepath.Join(city, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(city)

	var stderr bytes.Buffer
	code := run([]string{"init"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([init]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already initialized") {
		t.Errorf("stderr = %q, want 'already initialized'", stderr.String())
	}
}

// --- test helpers for error injection ---

type startErrorProvider struct{}

func (p *startErrorProvider) Start(string, session.Config) error { return fmt.Errorf("boom") }
func (p *startErrorProvider) Stop(string) error                  { return nil }
func (p *startErrorProvider) IsRunning(string) bool              { return false }
func (p *startErrorProvider) Attach(string) error                { return nil }

type attachErrorProvider struct{}

func (p *attachErrorProvider) Start(string, session.Config) error { return nil }
func (p *attachErrorProvider) Stop(string) error                  { return nil }
func (p *attachErrorProvider) IsRunning(string) bool              { return true }
func (p *attachErrorProvider) Attach(string) error                { return fmt.Errorf("attach boom") }
