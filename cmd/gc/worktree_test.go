package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
)

// initTestRepo creates a git repo with one commit in a temp directory.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// gitRun runs a git command in dir, failing the test on error.
// Strips git env vars to prevent interference from pre-commit hooks.
func gitRun(t *testing.T, dir string, args ...string) {
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

func TestWorktreeDir(t *testing.T) {
	got := worktreeDir("/city", "my-rig", "worker")
	want := filepath.Join("/city", ".gc", "worktrees", "my-rig", "worker")
	if got != want {
		t.Errorf("worktreeDir = %q, want %q", got, want)
	}
}

func TestWorktreeBranch(t *testing.T) {
	b := worktreeBranch("worker")
	if !strings.HasPrefix(b, "gc/worker-") {
		t.Errorf("worktreeBranch = %q, want prefix %q", b, "gc/worker-")
	}
	// Should be unique.
	b2 := worktreeBranch("worker")
	if b == b2 {
		t.Errorf("two calls returned same branch: %q", b)
	}
}

func TestCreateAgentWorktree(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wtPath, branch, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("createAgentWorktree: %v", err)
	}

	// Verify path is correct.
	wantPath := worktreeDir(cityPath, "my-rig", "worker")
	if wtPath != wantPath {
		t.Errorf("wtPath = %q, want %q", wtPath, wantPath)
	}

	// Verify directory exists.
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree dir not created: %v", err)
	}

	// Verify branch name.
	if !strings.HasPrefix(branch, "gc/worker-") {
		t.Errorf("branch = %q, want prefix %q", branch, "gc/worker-")
	}
}

func TestCreateAgentWorktree_Idempotent(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wt1, br1, err := createAgentWorktree(repo, cityPath, "rig", "worker")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Second call should reuse, not fail.
	wt2, br2, err := createAgentWorktree(repo, cityPath, "rig", "worker")
	if err != nil {
		t.Fatalf("second create: %v", err)
	}

	if wt1 != wt2 {
		t.Errorf("paths differ: %q vs %q", wt1, wt2)
	}
	if br1 != br2 {
		t.Errorf("branches differ: %q vs %q (should reuse)", br1, br2)
	}
}

func TestRemoveAgentWorktree(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(repo, cityPath, "rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	removeAgentWorktree(repo, wtPath)

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after remove")
	}
}

func TestRemoveAgentWorktree_AlreadyGone(t *testing.T) {
	repo := initTestRepo(t)
	// Removing a non-existent worktree should not panic.
	removeAgentWorktree(repo, "/nonexistent/worktree")
}

func TestSetupBeadsRedirect(t *testing.T) {
	wtPath := t.TempDir()
	rigPath := "/home/user/my-rig"

	if err := setupBeadsRedirect(wtPath, rigPath); err != nil {
		t.Fatalf("setupBeadsRedirect: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(wtPath, ".beads", "redirect"))
	if err != nil {
		t.Fatalf("reading redirect file: %v", err)
	}

	want := filepath.Join(rigPath, ".beads")
	if string(data) != want {
		t.Errorf("redirect content = %q, want %q", data, want)
	}
}

func TestFindRigByDir(t *testing.T) {
	rigs := []config.Rig{
		{Name: "frontend", Path: "/home/user/frontend"},
		{Name: "backend", Path: "/home/user/backend"},
	}

	name, path, found := findRigByDir("/home/user/frontend", rigs)
	if !found {
		t.Fatal("expected to find rig for /home/user/frontend")
	}
	if name != "frontend" {
		t.Errorf("name = %q, want %q", name, "frontend")
	}
	if path != "/home/user/frontend" {
		t.Errorf("path = %q, want %q", path, "/home/user/frontend")
	}

	_, _, found = findRigByDir("/home/user/other", rigs)
	if found {
		t.Error("expected not found for unmatched dir")
	}
}

func TestFindRigByDir_TrailingSlash(t *testing.T) {
	rigs := []config.Rig{{Name: "fe", Path: "/home/user/frontend/"}}
	_, _, found := findRigByDir("/home/user/frontend", rigs)
	if !found {
		t.Error("expected to find rig with trailing slash cleaned")
	}
}

func TestCleanupWorktrees(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	// Create a worktree to clean up.
	_, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	cleanupWorktrees(cityPath, rigs)

	// Worktree dir should be gone.
	wtPath := worktreeDir(cityPath, "my-rig", "worker")
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after cleanup")
	}
}

func TestCleanupWorktrees_NoWorktrees(t *testing.T) {
	cityPath := t.TempDir()
	// Should not panic when no .gc/worktrees exists.
	cleanupWorktrees(cityPath, nil)
}
