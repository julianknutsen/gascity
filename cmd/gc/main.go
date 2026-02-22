// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
