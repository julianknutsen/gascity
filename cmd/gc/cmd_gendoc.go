package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/docgen"
)

// newGenDocCmd creates the hidden "gc gen-doc" subcommand. It writes
// docs/reference/cli.md by walking the real command tree. Must be called
// from the repository root (go.mod must exist).
func newGenDocCmd(stdout, stderr io.Writer, root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:    "gen-doc",
		Short:  "Generate CLI reference documentation",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Verify repo root.
			if _, err := os.Stat("go.mod"); err != nil {
				fmt.Fprintf(stderr, "gen-doc: must run from repository root (go.mod not found)\n") //nolint:errcheck
				return errExit
			}

			// Ensure output directory exists.
			if err := os.MkdirAll("docs/reference", 0o755); err != nil {
				fmt.Fprintf(stderr, "gen-doc: creating docs/reference: %v\n", err) //nolint:errcheck
				return errExit
			}

			outPath := "docs/reference/cli.md"
			if err := docgen.WriteCLIMarkdown(outPath, root); err != nil {
				fmt.Fprintf(stderr, "gen-doc: %v\n", err) //nolint:errcheck
				return errExit
			}

			fmt.Fprintf(stdout, "Generated: %s\n", outPath) //nolint:errcheck
			return nil
		},
	}
}
