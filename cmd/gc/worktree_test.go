package main

import (
	"bytes"
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

// initTestRepoWithRemote creates a git repo backed by a bare remote so that
// HasUnpushedCommits returns false. Returns the clone directory.
func initTestRepoWithRemote(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")

	clone := t.TempDir()
	gitRun(t, clone, "clone", bare, ".")
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "commit", "--allow-empty", "-m", "init")
	gitRun(t, clone, "push", "origin", "HEAD")
	return clone
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
	repo := initTestRepoWithRemote(t)
	cityPath := t.TempDir()

	// Create a worktree to clean up.
	_, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	var stderr bytes.Buffer
	cleanupWorktrees(cityPath, rigs, &stderr)

	// Worktree dir should be gone.
	wtPath := worktreeDir(cityPath, "my-rig", "worker")
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after cleanup")
	}
}

func TestCleanupWorktrees_NoWorktrees(t *testing.T) {
	cityPath := t.TempDir()
	var stderr bytes.Buffer
	// Should not panic when no .gc/worktrees exists.
	cleanupWorktrees(cityPath, nil, &stderr)
}

func TestCleanupWorktrees_SkipsUnpushed(t *testing.T) {
	// Create a bare remote and clone it.
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")

	clone := t.TempDir()
	gitRun(t, clone, "clone", bare, ".")
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "commit", "--allow-empty", "-m", "init")
	gitRun(t, clone, "push", "origin", "HEAD")

	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(clone, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make local-only commit in worktree.
	gitRun(t, wtPath, "config", "user.email", "test@test.com")
	gitRun(t, wtPath, "config", "user.name", "Test")
	gitRun(t, wtPath, "commit", "--allow-empty", "-m", "local work")

	rigs := []config.Rig{{Name: "my-rig", Path: clone}}
	var stderr bytes.Buffer
	cleanupWorktrees(cityPath, rigs, &stderr)

	// Worktree should still exist (not removed).
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree with unpushed commits was removed, want preserved")
	}
	if !strings.Contains(stderr.String(), "unpushed commits") {
		t.Errorf("stderr = %q, want warning about unpushed commits", stderr.String())
	}
}

func TestCleanupWorktrees_SkipsStash(t *testing.T) {
	// Use a repo with a remote so HasUnpushedCommits doesn't trigger first.
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")

	clone := t.TempDir()
	gitRun(t, clone, "clone", bare, ".")
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "commit", "--allow-empty", "-m", "init")
	gitRun(t, clone, "push", "origin", "HEAD")

	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(clone, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Create a file and stash it in the worktree.
	if err := os.WriteFile(filepath.Join(wtPath, "stash-me.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, wtPath, "add", "stash-me.txt")
	gitRun(t, wtPath, "stash")

	rigs := []config.Rig{{Name: "my-rig", Path: clone}}
	var stderr bytes.Buffer
	cleanupWorktrees(cityPath, rigs, &stderr)

	// Worktree should still exist (not removed).
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree with stashes was removed, want preserved")
	}
	if !strings.Contains(stderr.String(), "stashes") {
		t.Errorf("stderr = %q, want warning about stashes", stderr.String())
	}
}

func TestCleanupWorktrees_SkipsDirty(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make the worktree dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}

	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	var stderr bytes.Buffer
	cleanupWorktrees(cityPath, rigs, &stderr)

	// Worktree should still exist (not removed).
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("dirty worktree was removed, want preserved")
	}

	// Should have warning on stderr.
	if !strings.Contains(stderr.String(), "uncommitted work") {
		t.Errorf("stderr = %q, want warning about uncommitted work", stderr.String())
	}
}

func TestSyncWorktree_Clean(t *testing.T) {
	// Create a bare remote, clone it, create a worktree.
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")

	clone := t.TempDir()
	gitRun(t, clone, "clone", bare, ".")
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "commit", "--allow-empty", "-m", "init")
	gitRun(t, clone, "push", "origin", "HEAD")

	cityPath := t.TempDir()
	wtPath, _, err := createAgentWorktree(clone, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make an upstream change via a second clone.
	clone2 := t.TempDir()
	gitRun(t, clone2, "clone", bare, ".")
	gitRun(t, clone2, "config", "user.email", "test@test.com")
	gitRun(t, clone2, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(clone2, "upstream.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, clone2, "add", "upstream.txt")
	gitRun(t, clone2, "commit", "-m", "upstream change")
	gitRun(t, clone2, "push", "origin", "HEAD")

	var stderr bytes.Buffer
	syncWorktree(wtPath, &stderr, "worker")

	// The upstream file should now exist in the worktree.
	if _, err := os.Stat(filepath.Join(wtPath, "upstream.txt")); err != nil {
		t.Errorf("upstream.txt not found after sync: %v", err)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestSyncWorktree_DirtyStashRestore(t *testing.T) {
	// Create a bare remote, clone, worktree.
	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")

	clone := t.TempDir()
	gitRun(t, clone, "clone", bare, ".")
	gitRun(t, clone, "config", "user.email", "test@test.com")
	gitRun(t, clone, "config", "user.name", "Test")
	gitRun(t, clone, "commit", "--allow-empty", "-m", "init")
	gitRun(t, clone, "push", "origin", "HEAD")

	cityPath := t.TempDir()
	wtPath, _, err := createAgentWorktree(clone, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make worktree dirty.
	localFile := filepath.Join(wtPath, "local-wip.txt")
	if err := os.WriteFile(localFile, []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	syncWorktree(wtPath, &stderr, "worker")

	// Local WIP file should still exist after sync (stash + pop).
	if _, err := os.Stat(localFile); err != nil {
		t.Errorf("local-wip.txt lost after sync: %v", err)
	}
}

func TestSyncWorktree_NoRemoteWarning(t *testing.T) {
	// Repo with no remote — fetch should fail gracefully.
	repo := initTestRepo(t)
	cityPath := t.TempDir()
	wtPath, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var stderr bytes.Buffer
	syncWorktree(wtPath, &stderr, "worker")

	// Should warn about fetch failure, not panic.
	if !strings.Contains(stderr.String(), "pre_sync fetch") {
		t.Errorf("stderr = %q, want warning about pre_sync fetch", stderr.String())
	}
}

func TestEnsureWorktreeGitignore(t *testing.T) {
	dir := t.TempDir()

	if err := ensureWorktreeGitignore(dir); err != nil {
		t.Fatalf("ensureWorktreeGitignore: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, worktreeGitignoreMarker) {
		t.Error("missing marker comment in .gitignore")
	}
	for _, pat := range []string{".beads/redirect", ".beads/hooks/", ".gemini/", ".opencode/"} {
		if !strings.Contains(content, pat) {
			t.Errorf("missing pattern %q in .gitignore", pat)
		}
	}
}

func TestEnsureWorktreeGitignore_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := ensureWorktreeGitignore(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ensureWorktreeGitignore(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	// Marker should appear exactly once.
	count := strings.Count(string(data), worktreeGitignoreMarker)
	if count != 1 {
		t.Errorf("marker appears %d times, want 1", count)
	}
}

func TestEnsureWorktreeGitignore_PreservesExisting(t *testing.T) {
	dir := t.TempDir()

	// Write pre-existing .gitignore content.
	existing := "node_modules/\n*.log\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureWorktreeGitignore(dir); err != nil {
		t.Fatalf("ensureWorktreeGitignore: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	content := string(data)
	// Existing content should still be there.
	if !strings.Contains(content, "node_modules/") {
		t.Error("existing pattern 'node_modules/' was lost")
	}
	if !strings.Contains(content, "*.log") {
		t.Error("existing pattern '*.log' was lost")
	}
	// New patterns should also be there.
	if !strings.Contains(content, worktreeGitignoreMarker) {
		t.Error("missing marker comment after append")
	}
}
