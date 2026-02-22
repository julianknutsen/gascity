package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newInitCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a new city",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdInit(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdInit initializes a new city at the given path (or cwd if no path given).
// Creates .gc/, rigs/, and a full city.toml with a default mayor agent.
func cmdInit(args []string, stdout, stderr io.Writer) int {
	var cityPath string
	if len(args) > 0 {
		var err error
		cityPath, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		cityPath, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	return doInit(fsys.OSFS{}, cityPath, stdout, stderr)
}

// doInit is the pure logic for "gc init". It creates the city directory
// structure (.gc/, rigs/) and writes a full city.toml with a default mayor
// agent. Errors if .gc/ already exists. Accepts an injected FS for testability.
func doInit(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	gcDir := filepath.Join(cityPath, ".gc")

	// Check if already initialized.
	if _, err := fs.Stat(gcDir); err == nil {
		fmt.Fprintln(stderr, "gc init: already initialized") //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create directory structure.
	if err := fs.MkdirAll(gcDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.MkdirAll(filepath.Join(cityPath, "rigs"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Write full city.toml.
	cityName := filepath.Base(cityPath)
	content := fmt.Sprintf("[workspace]\nname = %q\n\n[[agents]]\nname = \"mayor\"\n", cityName)
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := fs.WriteFile(tomlPath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintln(stdout, "Welcome to Gas City!")                                     //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "Initialized city %q with default mayor agent.\n", cityName) //nolint:errcheck // best-effort stdout
	return 0
}
