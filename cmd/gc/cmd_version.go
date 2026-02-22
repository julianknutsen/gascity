package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Build metadata — injected via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func newVersionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print gc version information",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintf(stdout, "gc %s (commit: %s, built: %s)\n", version, commit, date) //nolint:errcheck // best-effort stdout
		},
	}
}
