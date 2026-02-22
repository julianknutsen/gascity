package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newStartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "start <path>",
		Short: "Initialize a new city at the given path",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdStart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdStart initializes a new city at the given path, creating the directory
// structure (.gc/, rigs/) and a minimal city.toml.
func cmdStart(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc start: missing city path") //nolint:errcheck // best-effort stderr
		return 1
	}
	return doStart(fsys.OSFS{}, args[0], stdout, stderr)
}

// doStart is the pure logic for "gc start". It creates the city directory
// structure and writes a minimal city.toml. Accepts an injected FS for
// testability.
func doStart(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	// Create directory structure.
	if err := fs.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.MkdirAll(filepath.Join(cityPath, "rigs"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Write minimal city.toml.
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := fs.WriteFile(tomlPath, []byte("# city.toml — Gas City configuration\n"), 0o644); err != nil {
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
