package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newRigCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rig",
		Short: "Manage rigs (projects)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc rig: missing subcommand (add, list)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc rig: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newRigAddCmd(stdout, stderr),
		newRigListCmd(stdout, stderr),
	)
	return cmd
}

func newRigAddCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "add <path>",
		Short: "Register a project as a rig",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdRigAdd(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newRigListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered rigs",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdRigList(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
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
	return doRigAdd(fsys.OSFS{}, cityPath, rigPath, stdout, stderr)
}

// doRigAdd is the pure logic for "gc rig add". It validates the rig path,
// adds the rig to city.toml, creates the rigs/ directory, writes rig.toml,
// initializes beads, and generates routes. Accepts an injected FS for testability.
func doRigAdd(fs fsys.FS, cityPath, rigPath string, stdout, stderr io.Writer) int {
	fi, err := fs.Stat(rigPath)
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
	_, gitErr := fs.Stat(filepath.Join(rigPath, ".git"))
	hasGit := gitErr == nil

	// Load existing config to add the rig.
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: loading config: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Check for duplicate rig name.
	for _, r := range cfg.Rigs {
		if r.Name == name {
			fmt.Fprintf(stderr, "gc rig add: rig %q already registered\n", name) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	// Derive prefix.
	prefix := deriveBeadsPrefix(name)

	// Add rig to config.
	cfg.Rigs = append(cfg.Rigs, config.Rig{
		Name: name,
		Path: rigPath,
		// Prefix omitted in config — derived at runtime.
	})

	// Write updated city.toml.
	data, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc rig add: marshaling config: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.WriteFile(tomlPath, data, 0o644); err != nil {
		fmt.Fprintf(stderr, "gc rig add: writing config: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create rig directory and write rig.toml (backward compat).
	rigDir := filepath.Join(cityPath, "rigs", name)
	if err := fs.MkdirAll(rigDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rigToml := fmt.Sprintf("[rig]\npath = %q\n", rigPath)
	if err := fs.WriteFile(filepath.Join(rigDir, "rig.toml"), []byte(rigToml), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc rig add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("Adding rig '%s'...", name))
	if hasGit {
		w(fmt.Sprintf("  Detected git repo at %s", rigPath))
	}
	w(fmt.Sprintf("  Prefix: %s", prefix))

	// Initialize beads for the rig (if bd provider).
	if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
		if err := dolt.InitRigBeads(rigPath, prefix); err != nil {
			fmt.Fprintf(stderr, "gc rig add: init beads: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		w("  Initialized beads database")
	}

	// Generate routes for all rigs (HQ + all configured rigs).
	allRigs := collectRigRoutes(cityPath, cfg)
	if err := writeAllRoutes(allRigs); err != nil {
		fmt.Fprintf(stderr, "gc rig add: writing routes: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	w("  Generated routes.jsonl for cross-rig routing")

	w("Rig added.")
	return 0
}

// collectRigRoutes builds the list of all rig routes (HQ + configured rigs)
// for route generation. Resolves prefixes: uses configured prefix if set,
// otherwise derives from name.
func collectRigRoutes(cityPath string, cfg *config.City) []rigRoute {
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	hqPrefix := deriveBeadsPrefix(cityName)

	rigs := []rigRoute{{Prefix: hqPrefix, AbsDir: cityPath}}
	for _, r := range cfg.Rigs {
		prefix := r.Prefix
		if prefix == "" {
			prefix = deriveBeadsPrefix(r.Name)
		}
		rigs = append(rigs, rigRoute{Prefix: prefix, AbsDir: r.Path})
	}
	return rigs
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
	return doRigList(fsys.OSFS{}, cityPath, stdout, stderr)
}

// doRigList is the pure logic for "gc rig list". It reads rigs from city.toml
// and prints each with its prefix and beads status. Accepts an injected FS for
// testability.
func doRigList(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(fs, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc rig list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	hqPrefix := deriveBeadsPrefix(cityName)

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w("")
	w(fmt.Sprintf("Rigs in %s:", cityPath))

	// HQ rig (the city itself).
	hqBeads := rigBeadsStatus(fs, cityPath)
	w("")
	w(fmt.Sprintf("  %s (HQ):", cityName))
	w(fmt.Sprintf("    Prefix: %s", hqPrefix))
	w(fmt.Sprintf("    Beads:  %s", hqBeads))

	// Configured rigs.
	for _, r := range cfg.Rigs {
		prefix := r.Prefix
		if prefix == "" {
			prefix = deriveBeadsPrefix(r.Name)
		}
		beads := rigBeadsStatus(fs, r.Path)
		w("")
		w(fmt.Sprintf("  %s:", r.Name))
		w(fmt.Sprintf("    Path:   %s", r.Path))
		w(fmt.Sprintf("    Prefix: %s", prefix))
		w(fmt.Sprintf("    Beads:  %s", beads))
	}
	return 0
}

// rigBeadsStatus returns a human-readable beads status for a directory.
func rigBeadsStatus(fs fsys.FS, dir string) string {
	metaPath := filepath.Join(dir, ".beads", "metadata.json")
	if _, err := fs.Stat(metaPath); err == nil {
		return "initialized"
	}
	return "not initialized"
}
