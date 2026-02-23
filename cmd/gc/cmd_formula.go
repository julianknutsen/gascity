package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/formula"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newFormulaCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "formula",
		Short: "Manage formulas (multi-step workflow templates)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc formula: missing subcommand (list, show)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc formula: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newFormulaListCmd(stdout, stderr),
		newFormulaShowCmd(stdout, stderr),
	)
	return cmd
}

func newFormulaListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available formulas",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdFormulaList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newFormulaShowCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show details of a formula",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdFormulaShow(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdFormulaList is the CLI entry point for listing formulas.
func cmdFormulaList(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc formula list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc formula list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc formula list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doFormulaList(fsys.OSFS{}, filepath.Join(cityPath, cfg.FormulasDir()), stdout, stderr)
}

// doFormulaList scans a directory for *.formula.toml files and prints their
// names. Accepts an injected filesystem and directory for testability.
func doFormulaList(fs fsys.FS, formulasDir string, stdout, _ io.Writer) int {
	entries, err := fs.ReadDir(formulasDir)
	if err != nil {
		fmt.Fprintln(stdout, "No formulas found.") //nolint:errcheck // best-effort stdout
		return 0
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".formula.toml") {
			name := strings.TrimSuffix(e.Name(), ".formula.toml")
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		fmt.Fprintln(stdout, "No formulas found.") //nolint:errcheck // best-effort stdout
		return 0
	}

	for _, name := range names {
		fmt.Fprintln(stdout, name) //nolint:errcheck // best-effort stdout
	}
	return 0
}

// cmdFormulaShow is the CLI entry point for showing a formula.
func cmdFormulaShow(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc formula show: missing formula name") //nolint:errcheck // best-effort stderr
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc formula show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc formula show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc formula show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doFormulaShow(fsys.OSFS{}, filepath.Join(cityPath, cfg.FormulasDir()), args[0], stdout, stderr)
}

// doFormulaShow parses and validates a formula, then prints its details.
// Accepts an injected filesystem and directory for testability.
func doFormulaShow(fs fsys.FS, formulasDir, name string, stdout, stderr io.Writer) int {
	path := filepath.Join(formulasDir, name+".formula.toml")
	data, err := fs.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "gc formula show: formula %q not found\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	f, err := formula.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "gc formula show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := formula.Validate(f); err != nil {
		fmt.Fprintf(stderr, "gc formula show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("Formula:     %s", f.Name))
	w(fmt.Sprintf("Description: %s", f.Description))
	w(fmt.Sprintf("Steps:       %d", len(f.Steps)))
	w("")
	for i, s := range f.Steps {
		needs := ""
		if len(s.Needs) > 0 {
			needs = fmt.Sprintf("  (needs: %s)", strings.Join(s.Needs, ", "))
		}
		w(fmt.Sprintf("  %d. %s — %s%s", i+1, s.ID, s.Title, needs))
	}
	return 0
}
