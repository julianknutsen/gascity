package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newTopologyCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "topology",
		Short: "Manage remote topology sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newTopologyFetchCmd(stdout, stderr))
	cmd.AddCommand(newTopologyListCmd(stdout, stderr))
	return cmd
}

func newTopologyFetchCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch",
		Short: "Clone missing and update existing remote topologies",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doTopologyFetch(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doTopologyFetch clones missing topologies and updates existing ones.
func doTopologyFetch(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc topology fetch: %v\n", err) //nolint:errcheck
		return 1
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc topology fetch: %v\n", err) //nolint:errcheck
		return 1
	}

	if len(cfg.Topologies) == 0 {
		fmt.Fprintln(stdout, "No remote topologies configured.") //nolint:errcheck
		return 0
	}

	fmt.Fprintf(stdout, "Fetching %d topology source(s)...\n", len(cfg.Topologies)) //nolint:errcheck
	if err := config.FetchTopologies(cfg.Topologies, cityPath); err != nil {
		fmt.Fprintf(stderr, "gc topology fetch: %v\n", err) //nolint:errcheck
		return 1
	}

	// Write lockfile.
	lock, err := config.LockFromCache(cfg.Topologies, cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc topology fetch: building lock: %v\n", err) //nolint:errcheck
		return 1
	}
	if err := config.WriteLock(cityPath, lock); err != nil {
		fmt.Fprintf(stderr, "gc topology fetch: writing lock: %v\n", err) //nolint:errcheck
		return 1
	}

	for name := range cfg.Topologies {
		lt := lock.Topologies[name]
		commit := lt.Commit
		if len(commit) > 12 {
			commit = commit[:12]
		}
		fmt.Fprintf(stdout, "  %s: %s\n", name, commit) //nolint:errcheck
	}
	fmt.Fprintln(stdout, "Done.") //nolint:errcheck
	return 0
}

func newTopologyListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show remote topology sources and cache status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doTopologyList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doTopologyList shows configured topologies and their cache status.
func doTopologyList(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc topology list: %v\n", err) //nolint:errcheck
		return 1
	}

	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc topology list: %v\n", err) //nolint:errcheck
		return 1
	}

	if len(cfg.Topologies) == 0 {
		fmt.Fprintln(stdout, "No remote topologies configured.") //nolint:errcheck
		return 0
	}

	lock, _ := config.ReadLock(cityPath)

	for name, src := range cfg.Topologies {
		cached := "not cached"
		cachePath := config.TopologyCachePath(cityPath, name, src)
		fs := fsys.OSFS{}
		if _, statErr := fs.ReadFile(filepath.Join(cachePath, "topology.toml")); statErr == nil {
			cached = "cached"
		}

		ref := src.Ref
		if ref == "" {
			ref = "HEAD"
		}

		line := fmt.Sprintf("%-20s %-40s ref=%-12s %s", name, src.Source, ref, cached)

		if lt, ok := lock.Topologies[name]; ok && lt.Commit != "" {
			commit := lt.Commit
			if len(commit) > 12 {
				commit = commit[:12]
			}
			line += fmt.Sprintf("  commit=%s", commit)
		}

		fmt.Fprintln(stdout, line) //nolint:errcheck
	}
	return 0
}
