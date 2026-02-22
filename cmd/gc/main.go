// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the gc CLI with the given args, writing output to stdout and
// errors to stderr. Returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc: no command specified") //nolint:errcheck // best-effort stderr
		return 1
	}

	switch args[0] {
	case "start":
		return cmdStart(args[1:], stdout, stderr)
	case "rig":
		return cmdRig(args[1:], stdout, stderr)
	case "bead":
		return cmdBead(args[1:], stdout, stderr)
	case "agent":
		return cmdAgent(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc: unknown command %q\n", args[0]) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// cmdStart initializes a new city at the given path, creating the directory
// structure (.gc/, rigs/) and a minimal city.toml.
func cmdStart(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc start: missing city path") //nolint:errcheck // best-effort stderr
		return 1
	}

	cityPath := args[0]

	// Create directory structure.
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := os.MkdirAll(filepath.Join(cityPath, "rigs"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Write minimal city.toml.
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := os.WriteFile(tomlPath, []byte("# city.toml — Gas City configuration\n"), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout

	w("Welcome to Gas City!")
	w("To configure your new city, add a `city.toml` file.")
	w("")
	w("To get started with one of the built-in configurations, use `gc init`.")
	w("")
	w("To add a rig (project), use `gc rig add <path>`.")
	w("")
	w("For help, use `gc help`.")
	return 0
}

// findCity walks dir upward looking for a directory containing .gc/.
// Returns the city root path or an error.
func findCity(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".gc")); err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a city directory (no .gc/ found)")
		}
		dir = parent
	}
}

// openCityStore locates the city root from the current directory and opens a
// FileStore at .gc/beads.json. On error it writes to stderr and returns nil
// plus an exit code.
func openCityStore(stderr io.Writer, cmdName string) (beads.Store, int) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	store, err := beads.OpenFileStore(filepath.Join(cityPath, ".gc", "beads.json"))
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	return store, 0
}

// cmdRig dispatches rig subcommands (add, list).
func cmdRig(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc rig: missing subcommand (add, list)") //nolint:errcheck // best-effort stderr
		return 1
	}
	switch args[0] {
	case "add":
		return cmdRigAdd(args[1:], stdout, stderr)
	case "list":
		return cmdRigList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc rig: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// cmdRigAdd registers an external project directory as a rig in the city.
func cmdRigAdd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc rig add: missing path") //nolint:errcheck // best-effort stderr
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	rigPath, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fi, err := os.Stat(rigPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if !fi.IsDir() {
		fmt.Fprintf(stderr, "gc rig add: %s is not a directory\n", rigPath) //nolint:errcheck // best-effort stderr
		return 1
	}

	name := filepath.Base(rigPath)

	// Check for git repo.
	_, gitErr := os.Stat(filepath.Join(rigPath, ".git"))
	hasGit := gitErr == nil

	// Create rig directory and write rig.toml.
	rigDir := filepath.Join(cityPath, "rigs", name)
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rigToml := fmt.Sprintf("[rig]\npath = %q\n", rigPath)
	if err := os.WriteFile(filepath.Join(rigDir, "rig.toml"), []byte(rigToml), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("Adding rig '%s'...", name))
	if hasGit {
		w(fmt.Sprintf("  Detected git repo at %s", rigPath))
	}
	w("  Configured AGENTS.md and GEMINI.md with beads integration")
	w("  Assigned default agent: mayor")
	w("Rig added.")
	return 0
}

// cmdRigList lists all registered rigs in the current city.
func cmdRigList(args []string, stdout, stderr io.Writer) int {
	_ = args // no arguments used yet
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc rig list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc rig list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	entries, err := os.ReadDir(filepath.Join(cityPath, "rigs"))
	if err != nil {
		fmt.Fprintf(stderr, "gc rig list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	cityName := filepath.Base(cityPath)
	w("")
	w(fmt.Sprintf("Rigs in %s:", cityPath))
	w("")
	w(fmt.Sprintf("  %s:", cityName))
	w("    Agents: [mayor]")
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		w("")
		w(fmt.Sprintf("  %s:", e.Name()))
		w("    Agents: []")
	}
	return 0
}

// cmdBead dispatches bead subcommands (close, create, list, ready, show).
func cmdBead(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead: missing subcommand (close, create, list, ready, show)") //nolint:errcheck // best-effort stderr
		return 1
	}
	switch args[0] {
	case "close":
		return cmdBeadClose(args[1:], stdout, stderr)
	case "create":
		return cmdBeadCreate(args[1:], stdout, stderr)
	case "list":
		return cmdBeadList(args[1:], stdout, stderr)
	case "ready":
		return cmdBeadReady(args[1:], stdout, stderr)
	case "show":
		return cmdBeadShow(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc bead: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// cmdBeadClose is the CLI entry point for closing a bead. It opens a
// FileStore in the current city and delegates to doBeadClose.
func cmdBeadClose(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead close")
	if store == nil {
		return code
	}
	return doBeadClose(store, args, stdout, stderr)
}

// doBeadClose closes a bead by ID. Accepts an injected store for testability.
func doBeadClose(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead close: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := store.Close(args[0]); err != nil {
		fmt.Fprintf(stderr, "gc bead close: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Closed bead: %s\n", args[0]) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadCreate is the CLI entry point for bead creation. It opens a
// FileStore in the current city and delegates to doBeadCreate.
func cmdBeadCreate(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead create")
	if store == nil {
		return code
	}
	return doBeadCreate(store, args, stdout, stderr)
}

// doBeadCreate creates a bead with the given title. Accepts an injected
// store for testability.
func doBeadCreate(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead create: missing title") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Create(beads.Bead{Title: args[0]})
	if err != nil {
		fmt.Fprintf(stderr, "gc bead create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Created bead: %s  (status: %s)\n", b.ID, b.Status) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadList is the CLI entry point for listing all beads. It opens a
// FileStore in the current city and delegates to doBeadList.
func cmdBeadList(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead list")
	if store == nil {
		return code
	}
	return doBeadList(store, args, stdout, stderr)
}

// doBeadList lists all beads in a tab-aligned table. Accepts an injected
// store for testability.
func doBeadList(store beads.Store, args []string, stdout, stderr io.Writer) int {
	_ = args // no arguments used yet
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tASSIGNEE\tTITLE") //nolint:errcheck // best-effort stdout
	for _, b := range all {
		assignee := b.Assignee
		if assignee == "" {
			assignee = "\u2014"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", b.ID, b.Status, assignee, b.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadReady is the CLI entry point for listing ready beads. It opens a
// FileStore in the current city and delegates to doBeadReady.
func cmdBeadReady(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead ready")
	if store == nil {
		return code
	}
	return doBeadReady(store, args, stdout, stderr)
}

// doBeadReady lists all open beads in a tab-aligned table. Accepts an
// injected store for testability.
func doBeadReady(store beads.Store, args []string, stdout, stderr io.Writer) int {
	_ = args // no arguments used yet
	ready, err := store.Ready()
	if err != nil {
		fmt.Fprintf(stderr, "gc bead ready: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tTITLE") //nolint:errcheck // best-effort stdout
	for _, b := range ready {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", b.ID, b.Status, b.Title) //nolint:errcheck // best-effort stdout
	}
	tw.Flush() //nolint:errcheck // best-effort stdout
	return 0
}

// cmdBeadShow is the CLI entry point for showing a bead. It opens a
// FileStore in the current city and delegates to doBeadShow.
func cmdBeadShow(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc bead show")
	if store == nil {
		return code
	}
	return doBeadShow(store, args, stdout, stderr)
}

// doBeadShow displays a bead's details. Accepts an injected store for
// testability.
func doBeadShow(store beads.Store, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc bead show: missing bead ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Get(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc bead show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("ID:       %s", b.ID))
	w(fmt.Sprintf("Status:   %s", b.Status))
	w(fmt.Sprintf("Type:     %s", b.Type))
	w(fmt.Sprintf("Title:    %s", b.Title))
	w(fmt.Sprintf("Created:  %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
	assignee := b.Assignee
	if assignee == "" {
		assignee = "\u2014"
	}
	w(fmt.Sprintf("Assignee: %s", assignee))
	return 0
}

// cmdAgent dispatches agent subcommands (attach).
func cmdAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent: missing subcommand (attach)") //nolint:errcheck // best-effort stderr
		return 1
	}
	switch args[0] {
	case "attach":
		return cmdAgentAttach(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc agent: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// detectProvider scans PATH for known agent CLI binaries and returns the
// shell command to run. Checks claude, codex, gemini in order.
func detectProvider() (string, error) {
	providers := []struct {
		bin string
		cmd string
	}{
		{"claude", "claude --dangerously-skip-permissions"},
		{"codex", "codex --dangerously-bypass-approvals-and-sandbox"},
		{"gemini", "gemini --approval-mode yolo"},
	}
	for _, p := range providers {
		if _, err := exec.LookPath(p.bin); err == nil {
			return p.cmd, nil
		}
	}
	return "", fmt.Errorf("no supported agent CLI found in PATH (looked for: claude, codex, gemini)")
}

// cmdAgentAttach is the CLI entry point for attaching to an agent session.
// It detects the agent CLI provider, creates a real tmux provider, and
// delegates to doAgentAttach.
func cmdAgentAttach(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent attach: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	command, err := detectProvider()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent attach: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	sp := sessiontmux.NewProvider()
	return doAgentAttach(sp, args[0], command, stdout, stderr)
}

// doAgentAttach is the pure logic for "gc agent attach <name>".
// It is idempotent: starts the session if not already running, then attaches.
func doAgentAttach(sp session.Provider, name string, command string, stdout, stderr io.Writer) int {
	if !sp.IsRunning(name) {
		if err := sp.Start(name, session.Config{Command: command}); err != nil {
			fmt.Fprintf(stderr, "gc agent attach: starting session: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	fmt.Fprintf(stdout, "Attaching to agent '%s'...\n", name) //nolint:errcheck // best-effort stdout

	if err := sp.Attach(name); err != nil {
		fmt.Fprintf(stderr, "gc agent attach: attaching to session: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}
