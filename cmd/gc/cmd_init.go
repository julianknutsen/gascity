package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newInitCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a starter city.toml with a default agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdInit(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdInit writes a starter city.toml with a default mayor agent.
// Requires being inside a city (.gc/ must exist from gc start).
func cmdInit(args []string, stdout, stderr io.Writer) int {
	_ = args
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "gc init: not in a city directory; run 'gc start <path>' first") //nolint:errcheck // best-effort stderr
		return 1
	}
	return doInit(fsys.OSFS{}, cityPath, stdout, stderr)
}

// doInit is the pure logic for "gc init". It reads the existing city.toml,
// checks that no agents are already configured, and writes a starter config
// with a default mayor agent. Accepts an injected FS for testability.
func doInit(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")

	data, err := fs.ReadFile(tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cfg, err := config.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if len(cfg.Agents) > 0 {
		fmt.Fprintln(stderr, "gc init: city.toml already has agents configured") //nolint:errcheck // best-effort stderr
		return 1
	}

	cityName := filepath.Base(cityPath)
	content := fmt.Sprintf("[workspace]\nname = %q\n\n[[agents]]\nname = \"mayor\"\n", cityName)
	if err := fs.WriteFile(tomlPath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Initialized city %q with default mayor agent.\n", cityName) //nolint:errcheck // best-effort stdout
	return 0
}
