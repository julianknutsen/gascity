package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/git"
)

// worktreeDir returns the path for an agent's worktree under the city.
// Layout: <cityPath>/.gc/worktrees/<rigName>/<agentName>/
func worktreeDir(cityPath, rigName, agentName string) string {
	return filepath.Join(cityPath, ".gc", "worktrees", rigName, agentName)
}

// worktreeBranch generates a unique branch name for an agent worktree.
// Format: gc/<agentName>-<base36-nanos> — unique, sortable, namespaced.
func worktreeBranch(agentName string) string {
	nanos := time.Now().UnixNano()
	return fmt.Sprintf("gc/%s-%s", agentName, strconv.FormatInt(nanos, 36))
}

// createAgentWorktree creates a git worktree for an agent. Idempotent:
// if the worktree already exists (controller re-tick), reuses it.
// Returns (worktreePath, branchName, error).
func createAgentWorktree(repoDir, cityPath, rigName, agentName string) (string, string, error) {
	wtPath := worktreeDir(cityPath, rigName, agentName)

	// Already exists? Reuse it. Use Lstat to avoid following symlinks.
	if _, err := os.Lstat(wtPath); err == nil {
		wg := git.New(wtPath)
		branch, err := wg.CurrentBranch()
		if err != nil {
			return wtPath, "", nil // exists but can't read branch — still reuse
		}
		return wtPath, branch, nil
	}

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return "", "", fmt.Errorf("creating worktree parent dir: %w", err)
	}

	g := git.New(repoDir)
	branch := worktreeBranch(agentName)
	if err := g.WorktreeAdd(wtPath, branch); err != nil {
		return "", "", fmt.Errorf("creating worktree for agent %q: %w", agentName, err)
	}

	// Initialize submodules in the new worktree (no-op if none).
	wg := git.New(wtPath)
	_ = wg.SubmoduleInit() // best-effort — missing submodules are non-fatal

	return wtPath, branch, nil
}

// removeAgentWorktree removes a worktree and prunes stale entries.
// Idempotent and best-effort: never returns an error.
func removeAgentWorktree(repoDir, wtPath string) {
	if _, err := os.Lstat(wtPath); os.IsNotExist(err) {
		return // already gone
	}
	g := git.New(repoDir)
	if err := g.WorktreeRemove(wtPath, true); err != nil {
		// If git worktree remove fails (e.g., worktree already partially
		// cleaned), remove the directory and prune.
		os.RemoveAll(wtPath) //nolint:errcheck // best-effort cleanup
	}
	g.WorktreePrune() //nolint:errcheck // best-effort cleanup
}

// setupBeadsRedirect writes a redirect file so bead commands in the worktree
// find the shared beads in the rig directory. Creates <wtPath>/.beads/redirect
// with the absolute path to <rigPath>/.beads/.
func setupBeadsRedirect(wtPath, rigPath string) error {
	beadsDir := filepath.Join(wtPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return fmt.Errorf("creating .beads dir in worktree: %w", err)
	}
	target := filepath.Join(rigPath, ".beads")
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(target), 0o644); err != nil {
		return fmt.Errorf("writing beads redirect: %w", err)
	}
	return nil
}

// findRigByDir returns the rig name and path for a directory that matches
// a configured rig. Paths are cleaned before comparison.
func findRigByDir(dir string, rigs []config.Rig) (name, path string, found bool) {
	cleanDir := filepath.Clean(dir)
	for _, r := range rigs {
		if cleanDir == filepath.Clean(r.Path) {
			return r.Name, r.Path, true
		}
	}
	return "", "", false
}

// worktreeGitignoreMarker is the sentinel comment that marks the start of
// Gas City's managed block in .gitignore.
const worktreeGitignoreMarker = "# Gas City worktree infrastructure (do not edit this block)"

// worktreeGitignorePatterns are the patterns appended to .gitignore in agent
// worktrees. These cover all files that gc start installs into worktrees.
var worktreeGitignorePatterns = []string{
	worktreeGitignoreMarker,
	".beads/redirect",
	".beads/hooks/",
	".beads/formulas/",
	".gemini/",
	".opencode/",
	".github/copilot-instructions.md",
}

// ensureWorktreeGitignore appends Gas City infrastructure patterns to the
// worktree's .gitignore. Idempotent: if the marker comment is already
// present, this is a no-op. Preserves existing .gitignore content.
func ensureWorktreeGitignore(wtPath string) error {
	gitignorePath := filepath.Join(wtPath, ".gitignore")

	// Read existing content (may not exist yet).
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	// Already managed? No-op.
	if strings.Contains(string(existing), worktreeGitignoreMarker) {
		return nil
	}

	// Build the block to append.
	block := "\n" + strings.Join(worktreeGitignorePatterns, "\n") + "\n"

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore for append: %w", err)
	}

	if _, err := f.WriteString(block); err != nil {
		f.Close() //nolint:errcheck // write already failed
		return fmt.Errorf("writing .gitignore patterns: %w", err)
	}
	return f.Close()
}

// syncWorktree runs git fetch + pull --rebase on a worktree before agent
// start. Non-fatal: logs warnings and continues on failure.
func syncWorktree(wtPath string, stderr io.Writer, agentName string) {
	g := git.New(wtPath)

	// Fetch latest from origin.
	if err := g.Fetch(); err != nil {
		fmt.Fprintf(stderr, "gc start: agent %q: pre_sync fetch: %v\n", agentName, err) //nolint:errcheck // best-effort stderr
		return
	}

	// Stash dirty tree if needed.
	stashed := false
	if g.HasUncommittedWork() {
		if err := g.Stash("gc-pre-sync-auto-stash"); err != nil {
			fmt.Fprintf(stderr, "gc start: agent %q: pre_sync stash: %v\n", agentName, err) //nolint:errcheck // best-effort stderr
			return
		}
		stashed = true
	}

	// Pull --rebase onto default branch.
	defaultBranch, _ := g.DefaultBranch()
	if err := g.PullRebase("origin", defaultBranch); err != nil {
		fmt.Fprintf(stderr, "gc start: agent %q: pre_sync pull: %v\n", agentName, err) //nolint:errcheck // best-effort stderr
		if stashed {
			_ = g.StashPop()
		}
		return
	}

	// Restore stashed work.
	if stashed {
		if err := g.StashPop(); err != nil {
			fmt.Fprintf(stderr, "gc start: agent %q: pre_sync stash pop: %v (stash preserved)\n", agentName, err) //nolint:errcheck // best-effort stderr
		}
	}
}

// cleanupWorktrees removes all agent worktrees under .gc/worktrees/ and
// prunes git worktree metadata. Worktrees with uncommitted work are skipped
// with a warning (safety check).
func cleanupWorktrees(cityPath string, rigs []config.Rig, stderr io.Writer) {
	wtRoot := filepath.Join(cityPath, ".gc", "worktrees")
	entries, err := os.ReadDir(wtRoot)
	if err != nil {
		return // no worktrees directory — nothing to clean
	}

	// Build rig path lookup.
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
			// Rig no longer in config — just remove the directory.
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
			// Safety check: skip worktrees with uncommitted work,
			// unpushed commits, or stashes.
			wg := git.New(wtPath)
			if wg.HasUncommittedWork() {
				fmt.Fprintf(stderr, "gc stop: worktree %s has uncommitted work (skipping removal)\n", wtPath) //nolint:errcheck // best-effort
				continue
			}
			if wg.HasUnpushedCommits() {
				fmt.Fprintf(stderr, "gc stop: worktree %s has unpushed commits (skipping removal)\n", wtPath) //nolint:errcheck // best-effort
				continue
			}
			if wg.HasStashes() {
				fmt.Fprintf(stderr, "gc stop: worktree %s has stashes (skipping removal)\n", wtPath) //nolint:errcheck // best-effort
				continue
			}
			removeAgentWorktree(repoDir, wtPath)
		}

		// Prune the rig's stale worktree references.
		g := git.New(repoDir)
		g.WorktreePrune() //nolint:errcheck // best-effort
	}

	// Remove the worktrees root if now empty.
	os.Remove(wtRoot) //nolint:errcheck // best-effort, fails if non-empty (fine)
}
