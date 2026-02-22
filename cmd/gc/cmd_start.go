package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newStartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "start [path]",
		Short: "Start the city (auto-initializes if needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doStart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doStart boots the city. If a path is given, operates there; otherwise uses
// cwd. If no city exists at the target, it auto-initializes one first via
// doInit, then prints "City started."
func doStart(args []string, stdout, stderr io.Writer) int {
	var dir string
	if len(args) > 0 {
		var err error
		dir, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	if _, err := findCity(dir); err != nil {
		// No city found — auto-init at dir.
		if code := doInit(fsys.OSFS{}, dir, stdout, stderr); code != 0 {
			return code
		}
	}

	fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout
	return 0
}
