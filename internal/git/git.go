// Package git provides minimal Git worktree operations for agent isolation.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Worktree represents a single git worktree entry.
type Worktree struct {
	Path   string
	Head   string
	Branch string
}

// Git wraps git operations scoped to a working directory.
type Git struct {
	workDir string
}

// New returns a Git instance scoped to the given directory.
func New(workDir string) *Git {
	return &Git{workDir: workDir}
}

// IsRepo reports whether workDir is inside a git repository.
func (g *Git) IsRepo() bool {
	_, err := g.run("rev-parse", "--git-dir")
	return err == nil
}

// CurrentBranch returns the current branch name. Returns "HEAD" if detached.
func (g *Git) CurrentBranch() (string, error) {
	out, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// DefaultBranch returns the default branch name via the origin HEAD symref.
// Falls back to "main" if no remote is configured.
func (g *Git) DefaultBranch() (string, error) {
	out, err := g.run("symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main", nil
	}
	// Output is like "refs/remotes/origin/main"
	ref := strings.TrimSpace(out)
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:], nil
	}
	return ref, nil
}

// WorktreeAdd creates a new worktree at path with a new branch.
// Equivalent to: git worktree add -b <branch> <path>
func (g *Git) WorktreeAdd(path, branch string) error {
	_, err := g.run("worktree", "add", "-b", branch, path)
	if err != nil {
		return fmt.Errorf("adding worktree at %q branch %q: %w", path, branch, err)
	}
	return nil
}

// WorktreeRemove removes a worktree. If force is true, removes even with
// uncommitted changes.
func (g *Git) WorktreeRemove(path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(args...)
	if err != nil {
		return fmt.Errorf("removing worktree %q: %w", path, err)
	}
	return nil
}

// WorktreeList returns all worktrees in porcelain format.
func (g *Git) WorktreeList() ([]Worktree, error) {
	out, err := g.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	return parseWorktreeList(out), nil
}

// HasUncommittedWork reports whether the working directory has uncommitted
// changes (staged or unstaged) or untracked files. Used as a safety check
// before removing a worktree to avoid losing in-progress work.
func (g *Git) HasUncommittedWork() bool {
	out, err := g.run("status", "--porcelain")
	if err != nil {
		return true // assume dirty on error (safe default)
	}
	return strings.TrimSpace(out) != ""
}

// SubmoduleInit initializes and updates submodules recursively.
// No-op if the repo has no submodules. Best-effort — errors are returned
// but callers may choose to ignore them.
func (g *Git) SubmoduleInit() error {
	_, err := g.run("submodule", "update", "--init", "--recursive")
	if err != nil {
		return fmt.Errorf("initializing submodules: %w", err)
	}
	return nil
}

// WorktreePrune removes stale worktree entries.
func (g *Git) WorktreePrune() error {
	_, err := g.run("worktree", "prune")
	if err != nil {
		return fmt.Errorf("pruning worktrees: %w", err)
	}
	return nil
}

// gitEnvBlacklist lists git environment variables that must be stripped
// so subprocess git commands use the intended workDir, not a parent repo.
// This prevents leakage from pre-commit hooks or other git tooling.
var gitEnvBlacklist = map[string]bool{
	"GIT_DIR":                          true,
	"GIT_WORK_TREE":                    true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
}

// run executes a git command in the working directory. Git environment
// variables from the parent process are stripped to prevent interference
// (e.g., when called from a pre-commit hook context).
func (g *Git) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workDir
	// Build clean env: inherit everything except git-specific vars.
	for _, e := range os.Environ() {
		if k, _, ok := strings.Cut(e, "="); ok && gitEnvBlacklist[k] {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// parseWorktreeList parses git worktree list --porcelain output.
// Each worktree block is separated by a blank line and contains
// "worktree <path>", "HEAD <sha>", "branch refs/heads/<name>".
func parseWorktreeList(output string) []Worktree {
	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix.
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	// Handle last block if output doesn't end with blank line.
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees
}
