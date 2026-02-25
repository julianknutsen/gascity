package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newConfigCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate city configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigShowCmd(stdout, stderr))
	return cmd
}

func newConfigShowCmd(stdout, stderr io.Writer) *cobra.Command {
	var validate bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Dump the resolved city configuration as TOML",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doConfigShow(validate, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&validate, "validate", false, "validate config and exit (0 = valid, 1 = errors)")
	return cmd
}

// doConfigShow loads city.toml and dumps the resolved config or validates it.
func doConfigShow(validate bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc config show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc config show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Run validation.
	var validationErrors []string
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		validationErrors = append(validationErrors, err.Error())
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	if err := config.ValidateRigs(cfg.Rigs, cityName); err != nil {
		validationErrors = append(validationErrors, err.Error())
	}

	if validate {
		if len(validationErrors) > 0 {
			for _, e := range validationErrors {
				fmt.Fprintf(stderr, "gc config show: %s\n", e) //nolint:errcheck // best-effort stderr
			}
			return 1
		}
		fmt.Fprintln(stdout, "Config valid.") //nolint:errcheck // best-effort stdout
		return 0
	}

	// Print validation warnings even in show mode.
	for _, e := range validationErrors {
		fmt.Fprintf(stderr, "gc config show: warning: %s\n", e) //nolint:errcheck // best-effort stderr
	}

	data, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc config show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprint(stdout, string(data)) //nolint:errcheck // best-effort stdout
	return 0
}
