package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a git repo with one commit in a temp directory.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// runGit runs a git command in dir and fails the test on error.
// Strips git env vars to prevent interference from pre-commit hooks.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), out, err)
	}
}

func TestIsRepo(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)
	if !g.IsRepo() {
		t.Error("IsRepo() = false, want true")
	}

	notRepo := t.TempDir()
	g2 := New(notRepo)
	if g2.IsRepo() {
		t.Error("IsRepo() = true for non-repo, want false")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)
	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	// Default branch is typically "master" or "main" depending on git config.
	if branch == "" {
		t.Error("CurrentBranch returned empty string")
	}
}

func TestDefaultBranch_NoRemote(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)
	branch, err := g.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("DefaultBranch() = %q, want %q (fallback)", branch, "main")
	}
}

func TestWorktreeAdd(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := g.WorktreeAdd(wtPath, "feature-1"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	// Verify the worktree directory exists.
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree dir not created: %v", err)
	}

	// Verify it's on the right branch.
	wg := New(wtPath)
	branch, err := wg.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch in worktree: %v", err)
	}
	if branch != "feature-1" {
		t.Errorf("worktree branch = %q, want %q", branch, "feature-1")
	}
}

func TestWorktreeRemove(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := g.WorktreeAdd(wtPath, "to-remove"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	if err := g.WorktreeRemove(wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}

	// Directory should be gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove")
	}
}

func TestWorktreeRemoveForce(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := g.WorktreeAdd(wtPath, "dirty-wt"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	// Create an uncommitted file to make the worktree dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Force remove should succeed even with dirty worktree.
	if err := g.WorktreeRemove(wtPath, true); err != nil {
		t.Fatalf("WorktreeRemove(force): %v", err)
	}
}

func TestWorktreeList(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := g.WorktreeAdd(wtPath, "listed"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	worktrees, err := g.WorktreeList()
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}

	// Should have at least 2: the main repo and the worktree.
	if len(worktrees) < 2 {
		t.Fatalf("len(worktrees) = %d, want >= 2", len(worktrees))
	}

	// Find our worktree.
	var found bool
	for _, wt := range worktrees {
		if wt.Path == wtPath {
			found = true
			if wt.Branch != "listed" {
				t.Errorf("worktree branch = %q, want %q", wt.Branch, "listed")
			}
		}
	}
	if !found {
		t.Errorf("worktree at %q not found in list", wtPath)
	}
}

func TestWorktreePrune(t *testing.T) {
	repo := initTestRepo(t)
	g := New(repo)

	// Prune on a clean repo should not fail.
	if err := g.WorktreePrune(); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
}

func TestParseWorktreeList(t *testing.T) {
	output := `worktree /home/user/repo
HEAD abc123
branch refs/heads/main

worktree /home/user/repo-wt
HEAD def456
branch refs/heads/feature-1

`
	wts := parseWorktreeList(output)
	if len(wts) != 2 {
		t.Fatalf("len(worktrees) = %d, want 2", len(wts))
	}
	if wts[0].Path != "/home/user/repo" {
		t.Errorf("wts[0].Path = %q, want %q", wts[0].Path, "/home/user/repo")
	}
	if wts[0].Branch != "main" {
		t.Errorf("wts[0].Branch = %q, want %q", wts[0].Branch, "main")
	}
	if wts[1].Path != "/home/user/repo-wt" {
		t.Errorf("wts[1].Path = %q, want %q", wts[1].Path, "/home/user/repo-wt")
	}
	if wts[1].Branch != "feature-1" {
		t.Errorf("wts[1].Branch = %q, want %q", wts[1].Branch, "feature-1")
	}
	if wts[1].Head != "def456" {
		t.Errorf("wts[1].Head = %q, want %q", wts[1].Head, "def456")
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	wts := parseWorktreeList("")
	if len(wts) != 0 {
		t.Errorf("len(worktrees) = %d, want 0", len(wts))
	}
}
