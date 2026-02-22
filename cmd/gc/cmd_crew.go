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

func newCrewCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crew",
		Short: "Manage workspace crew members",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc crew: missing subcommand (add, list)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc crew: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newCrewAddCmd(stdout, stderr),
		newCrewListCmd(stdout, stderr),
	)
	return cmd
}

func newCrewAddCmd(stdout, stderr io.Writer) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add --name <name>",
		Short: "Add a crew member to the workspace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdCrewAdd(name, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name of the crew member")
	return cmd
}

func newCrewListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workspace crew members",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdCrewList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdCrewAdd is the CLI entry point for adding a crew member. It locates
// the city root and delegates to doCrewAdd.
func cmdCrewAdd(name string, stdout, stderr io.Writer) int {
	if name == "" {
		fmt.Fprintln(stderr, "gc crew add: missing --name flag") //nolint:errcheck // best-effort stderr
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc crew add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc crew add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doCrewAdd(fsys.OSFS{}, cityPath, name, stdout, stderr)
}

// doCrewAdd is the pure logic for "gc crew add". It loads city.toml,
// checks for duplicates, appends the new agent, and writes back.
// Accepts an injected FS for testability.
func doCrewAdd(fs fsys.FS, cityPath, name string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc crew add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	for _, a := range cfg.Agents {
		if a.Name == name {
			fmt.Fprintf(stderr, "gc crew add: agent %q already exists\n", name) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	cfg.Agents = append(cfg.Agents, config.Agent{Name: name})
	content, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc crew add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.WriteFile(tomlPath, content, 0o644); err != nil {
		fmt.Fprintf(stderr, "gc crew add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Added crew member '%s'\n", name) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdCrewList is the CLI entry point for listing crew members. It locates
// the city root and delegates to doCrewList.
func cmdCrewList(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc crew list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc crew list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doCrewList(fsys.OSFS{}, cityPath, stdout, stderr)
}

// doCrewList is the pure logic for "gc crew list". It loads city.toml
// and prints the city name header followed by agent names.
// Accepts an injected FS for testability.
func doCrewList(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc crew list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "%s:\n", cfg.Workspace.Name) //nolint:errcheck // best-effort stdout
	for _, a := range cfg.Agents {
		fmt.Fprintf(stdout, "  %s\n", a.Name) //nolint:errcheck // best-effort stdout
	}
	return 0
}
