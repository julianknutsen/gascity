package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/git"
)

// worktreeInfo holds information about a single agent worktree.
type worktreeInfo struct {
	RigName   string
	AgentName string
	Path      string
	Branch    string
	Dirty     bool
	Unpushed  bool
	Stashes   bool
}

func newWorktreeCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage agent worktrees",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc worktree: missing subcommand (list, clean)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc worktree: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newWorktreeListCmd(stdout, stderr),
		newWorktreeCleanCmd(stdout, stderr),
	)
	return cmd
}

func newWorktreeListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all agent worktrees with status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdWorktreeList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newWorktreeCleanCmd(stdout, stderr io.Writer) *cobra.Command {
	var all, force bool
	cmd := &cobra.Command{
		Use:   "clean [paths...]",
		Short: "Remove agent worktrees",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdWorktreeClean(args, all, force, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "clean all worktrees")
	cmd.Flags().BoolVar(&force, "force", false, "remove even dirty worktrees with uncommitted work")
	return cmd
}

// cmdWorktreeList is the CLI entry point for listing worktrees.
func cmdWorktreeList(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc worktree list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	infos, err := doWorktreeList(cityPath, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "gc worktree list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	for _, info := range infos {
		status := worktreeStatus(info)
		fmt.Fprintf(stdout, "%s/%s\t%s\t%s\t%s\n", //nolint:errcheck // best-effort stdout
			info.RigName, info.AgentName, info.Branch, status, info.Path)
	}
	return 0
}

// doWorktreeList walks the .gc/worktrees/ directory and returns info about
// each agent worktree. Reads the filesystem directly — no config dependency.
func doWorktreeList(cityPath string, _ io.Writer) ([]worktreeInfo, error) {
	wtRoot := filepath.Join(cityPath, ".gc", "worktrees")
	rigEntries, err := os.ReadDir(wtRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no worktrees directory
		}
		return nil, fmt.Errorf("reading worktrees dir: %w", err)
	}

	var infos []worktreeInfo
	for _, rigEntry := range rigEntries {
		if !rigEntry.IsDir() {
			continue
		}
		rigName := rigEntry.Name()
		agentEntries, err := os.ReadDir(filepath.Join(wtRoot, rigName))
		if err != nil {
			continue
		}
		for _, agentEntry := range agentEntries {
			if !agentEntry.IsDir() {
				continue
			}
			wtPath := filepath.Join(wtRoot, rigName, agentEntry.Name())
			g := git.New(wtPath)
			branch, _ := g.CurrentBranch()
			infos = append(infos, worktreeInfo{
				RigName:   rigName,
				AgentName: agentEntry.Name(),
				Path:      wtPath,
				Branch:    branch,
				Dirty:     g.HasUncommittedWork(),
				Unpushed:  g.HasUnpushedCommits(),
				Stashes:   g.HasStashes(),
			})
		}
	}
	return infos, nil
}

// cmdWorktreeClean is the CLI entry point for cleaning worktrees.
func cmdWorktreeClean(args []string, all, force bool, stdout, stderr io.Writer) int {
	if all && len(args) > 0 {
		fmt.Fprintln(stderr, "gc worktree clean: --all and path arguments are mutually exclusive") //nolint:errcheck // best-effort stderr
		return 1
	}
	if !all && len(args) == 0 {
		fmt.Fprintln(stderr, "gc worktree clean: specify paths or use --all") //nolint:errcheck // best-effort stderr
		return 1
	}

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc worktree clean: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if all {
		cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
		if err != nil {
			fmt.Fprintf(stderr, "gc worktree clean: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		return doWorktreeCleanAll(cityPath, cfg.Rigs, force, stdout, stderr)
	}
	return doWorktreeClean(cityPath, args, force, stdout, stderr)
}

// doWorktreeClean removes specific worktrees by path. Skips dirty worktrees
// unless force is true.
func doWorktreeClean(cityPath string, paths []string, force bool, stdout, stderr io.Writer) int {
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc worktree clean: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Build rig path lookup.
	rigPaths := make(map[string]string, len(cfg.Rigs))
	for _, r := range cfg.Rigs {
		rigPaths[r.Name] = r.Path
	}

	exitCode := 0
	for _, wtPath := range paths {
		wtPath = filepath.Clean(wtPath)

		// Verify the path exists.
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			fmt.Fprintf(stderr, "gc worktree clean: %s: not found\n", wtPath) //nolint:errcheck // best-effort stderr
			exitCode = 1
			continue
		}

		// Safety check: skip worktrees with uncommitted work, unpushed
		// commits, or stashes unless forced.
		if !force {
			g := git.New(wtPath)
			if g.HasUncommittedWork() {
				fmt.Fprintf(stderr, "gc worktree clean: %s has uncommitted work (use --force to remove)\n", wtPath) //nolint:errcheck // best-effort stderr
				exitCode = 1
				continue
			}
			if g.HasUnpushedCommits() {
				fmt.Fprintf(stderr, "gc worktree clean: %s has unpushed commits (use --force to remove)\n", wtPath) //nolint:errcheck // best-effort stderr
				exitCode = 1
				continue
			}
			if g.HasStashes() {
				fmt.Fprintf(stderr, "gc worktree clean: %s has stashes (use --force to remove)\n", wtPath) //nolint:errcheck // best-effort stderr
				exitCode = 1
				continue
			}
		}

		// Determine the repo dir from the worktree path structure.
		// Path: <cityPath>/.gc/worktrees/<rigName>/<agentName>
		repoDir := repoForWorktree(wtPath, cityPath, rigPaths)
		if repoDir == "" {
			// Can't determine repo — just remove the directory.
			os.RemoveAll(wtPath)                        //nolint:errcheck // best-effort
			fmt.Fprintf(stdout, "Removed %s\n", wtPath) //nolint:errcheck // best-effort stdout
			continue
		}

		removeAgentWorktree(repoDir, wtPath)
		fmt.Fprintf(stdout, "Removed %s\n", wtPath) //nolint:errcheck // best-effort stdout
	}
	return exitCode
}

// doWorktreeCleanAll removes all worktrees. Delegates to cleanupWorktrees
// for non-forced mode. For forced mode, walks and removes everything.
func doWorktreeCleanAll(cityPath string, rigs []config.Rig, force bool, stdout, stderr io.Writer) int {
	if !force {
		// Reuse existing cleanupWorktrees which already skips dirty.
		cleanupWorktrees(cityPath, rigs, stderr)
		fmt.Fprintln(stdout, "Cleaned all worktrees (dirty worktrees preserved).") //nolint:errcheck // best-effort stdout
		return 0
	}

	// Force mode: remove everything regardless of dirty state.
	wtRoot := filepath.Join(cityPath, ".gc", "worktrees")
	entries, err := os.ReadDir(wtRoot)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(stdout, "No worktrees to clean.") //nolint:errcheck // best-effort stdout
			return 0
		}
		fmt.Fprintf(stderr, "gc worktree clean: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	rigPaths := make(map[string]string, len(rigs))
	for _, r := range rigs {
		rigPaths[r.Name] = r.Path
	}

	for _, rigEntry := range entries {
		if !rigEntry.IsDir() {
			continue
		}
		rigName := rigEntry.Name()
		repoDir, ok := rigPaths[rigName]
		if !ok {
			os.RemoveAll(filepath.Join(wtRoot, rigName)) //nolint:errcheck // best-effort
			continue
		}

		agentEntries, err := os.ReadDir(filepath.Join(wtRoot, rigName))
		if err != nil {
			continue
		}
		for _, agentEntry := range agentEntries {
			if !agentEntry.IsDir() {
				continue
			}
			wtPath := filepath.Join(wtRoot, rigName, agentEntry.Name())
			removeAgentWorktree(repoDir, wtPath)
		}

		// Prune stale worktree references.
		g := git.New(repoDir)
		g.WorktreePrune() //nolint:errcheck // best-effort
	}

	os.Remove(wtRoot)                                       //nolint:errcheck // best-effort, fails if non-empty (fine)
	fmt.Fprintln(stdout, "Cleaned all worktrees (forced).") //nolint:errcheck // best-effort stdout
	return 0
}

// repoForWorktree determines the git repo dir for a worktree path by
// extracting the rig name from the path structure and looking it up.
// Path layout: <cityPath>/.gc/worktrees/<rigName>/<agentName>
func repoForWorktree(wtPath, cityPath string, rigPaths map[string]string) string {
	wtRoot := filepath.Join(cityPath, ".gc", "worktrees")
	rel, err := filepath.Rel(wtRoot, wtPath)
	if err != nil {
		return ""
	}
	// rel is "<rigName>/<agentName>".
	rigName := filepath.Dir(rel)
	if rigName == "." {
		return ""
	}
	repoDir, ok := rigPaths[rigName]
	if !ok {
		return ""
	}
	return repoDir
}

// worktreeStatus returns a human-readable status string for a worktreeInfo.
// Combines multiple flags with "+": "dirty+unpushed", "dirty+stash", etc.
// Returns "clean" if no issues.
func worktreeStatus(info worktreeInfo) string {
	var parts []string
	if info.Dirty {
		parts = append(parts, "dirty")
	}
	if info.Unpushed {
		parts = append(parts, "unpushed")
	}
	if info.Stashes {
		parts = append(parts, "stash")
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, "+")
}
