package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
)

func TestDoWorktreeList(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	// Create two worktrees.
	wt1, br1, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-1")
	if err != nil {
		t.Fatalf("create worker-1: %v", err)
	}
	_, br2, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-2")
	if err != nil {
		t.Fatalf("create worker-2: %v", err)
	}

	// Make worker-1 dirty.
	if err := os.WriteFile(filepath.Join(wt1, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	infos, err := doWorktreeList(cityPath, &stderr)
	if err != nil {
		t.Fatalf("doWorktreeList: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(infos))
	}

	// Build a lookup by agent name.
	byAgent := make(map[string]worktreeInfo, len(infos))
	for _, info := range infos {
		byAgent[info.AgentName] = info
	}

	w1 := byAgent["worker-1"]
	if w1.RigName != "my-rig" {
		t.Errorf("worker-1 rig = %q, want %q", w1.RigName, "my-rig")
	}
	if w1.Branch != br1 {
		t.Errorf("worker-1 branch = %q, want %q", w1.Branch, br1)
	}
	if !w1.Dirty {
		t.Error("worker-1 should be dirty")
	}

	w2 := byAgent["worker-2"]
	if w2.Branch != br2 {
		t.Errorf("worker-2 branch = %q, want %q", w2.Branch, br2)
	}
	if w2.Dirty {
		t.Error("worker-2 should be clean")
	}
}

func TestDoWorktreeList_Empty(t *testing.T) {
	cityPath := t.TempDir()
	var stderr bytes.Buffer
	infos, err := doWorktreeList(cityPath, &stderr)
	if err != nil {
		t.Fatalf("doWorktreeList: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d worktrees, want 0", len(infos))
	}
}

func TestDoWorktreeClean_SinglePath(t *testing.T) {
	repo := initTestRepoWithRemote(t)
	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Write city.toml so doWorktreeClean can load config.
	writeCityToml(t, cityPath, repo, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wtPath}, false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doWorktreeClean returned %d; stderr: %s", code, stderr.String())
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree dir still exists after clean")
	}
	if !strings.Contains(stdout.String(), "Removed") {
		t.Errorf("stdout = %q, want 'Removed' message", stdout.String())
	}
}

func TestDoWorktreeClean_MultiplePaths(t *testing.T) {
	repo := initTestRepoWithRemote(t)
	cityPath := t.TempDir()

	wt1, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-1")
	if err != nil {
		t.Fatalf("create worker-1: %v", err)
	}
	wt2, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-2")
	if err != nil {
		t.Fatalf("create worker-2: %v", err)
	}

	writeCityToml(t, cityPath, repo, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wt1, wt2}, false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doWorktreeClean returned %d; stderr: %s", code, stderr.String())
	}

	for _, wt := range []string{wt1, wt2} {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists after clean", wt)
		}
	}
}

func TestDoWorktreeClean_SkipsDirty(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeCityToml(t, cityPath, repo, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wtPath}, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doWorktreeClean returned %d, want 1 (dirty skip)", code)
	}

	// Worktree should still exist.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("dirty worktree was removed, want preserved")
	}

	if !strings.Contains(stderr.String(), "uncommitted work") {
		t.Errorf("stderr = %q, want warning about uncommitted work", stderr.String())
	}
}

func TestDoWorktreeClean_SkipsUnpushed(t *testing.T) {
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

	writeCityToml(t, cityPath, clone, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wtPath}, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doWorktreeClean returned %d, want 1 (unpushed skip)", code)
	}

	// Worktree should still exist.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree with unpushed commits was removed, want preserved")
	}
	if !strings.Contains(stderr.String(), "unpushed commits") {
		t.Errorf("stderr = %q, want warning about unpushed commits", stderr.String())
	}
}

func TestDoWorktreeClean_SkipsStash(t *testing.T) {
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

	writeCityToml(t, cityPath, clone, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wtPath}, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doWorktreeClean returned %d, want 1 (stash skip)", code)
	}

	// Worktree should still exist.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree with stashes was removed, want preserved")
	}
	if !strings.Contains(stderr.String(), "stashes") {
		t.Errorf("stderr = %q, want warning about stashes", stderr.String())
	}
}

func TestDoWorktreeClean_ForceDirty(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wtPath, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Make dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeCityToml(t, cityPath, repo, "my-rig")

	var stdout, stderr bytes.Buffer
	code := doWorktreeClean(cityPath, []string{wtPath}, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doWorktreeClean --force returned %d; stderr: %s", code, stderr.String())
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("dirty worktree should be removed with --force")
	}
}

func TestDoWorktreeCleanAll(t *testing.T) {
	repo := initTestRepoWithRemote(t)
	cityPath := t.TempDir()

	wt1, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-1")
	if err != nil {
		t.Fatalf("create worker-1: %v", err)
	}
	wt2, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-2")
	if err != nil {
		t.Fatalf("create worker-2: %v", err)
	}

	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	var stdout, stderr bytes.Buffer
	code := doWorktreeCleanAll(cityPath, rigs, false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doWorktreeCleanAll returned %d; stderr: %s", code, stderr.String())
	}

	for _, wt := range []string{wt1, wt2} {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists after clean --all", wt)
		}
	}
}

func TestDoWorktreeCleanAll_Force(t *testing.T) {
	repo := initTestRepo(t)
	cityPath := t.TempDir()

	wt1, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-1")
	if err != nil {
		t.Fatalf("create worker-1: %v", err)
	}
	wt2, _, err := createAgentWorktree(repo, cityPath, "my-rig", "worker-2")
	if err != nil {
		t.Fatalf("create worker-2: %v", err)
	}

	// Make worker-1 dirty.
	if err := os.WriteFile(filepath.Join(wt1, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}

	rigs := []config.Rig{{Name: "my-rig", Path: repo}}
	var stdout, stderr bytes.Buffer
	code := doWorktreeCleanAll(cityPath, rigs, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doWorktreeCleanAll --force returned %d; stderr: %s", code, stderr.String())
	}

	for _, wt := range []string{wt1, wt2} {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists after clean --all --force", wt)
		}
	}
}

func TestRepoForWorktree(t *testing.T) {
	rigPaths := map[string]string{
		"my-rig": "/home/user/my-rig",
	}

	got := repoForWorktree("/city/.gc/worktrees/my-rig/worker", "/city", rigPaths)
	if got != "/home/user/my-rig" {
		t.Errorf("repoForWorktree = %q, want %q", got, "/home/user/my-rig")
	}

	// Unknown rig.
	got = repoForWorktree("/city/.gc/worktrees/unknown/worker", "/city", rigPaths)
	if got != "" {
		t.Errorf("repoForWorktree for unknown rig = %q, want empty", got)
	}
}

// writeCityToml writes a minimal city.toml with one rig for testing.
func writeCityToml(t *testing.T, cityPath, repoPath, rigName string) { //nolint:unparam // test helper kept parameterized for clarity
	t.Helper()
	gcDir := filepath.Join(cityPath, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `[workspace]
name = "test-city"

[[rigs]]
name = "` + rigName + `"
path = "` + repoPath + `"
`
	if err := os.WriteFile(filepath.Join(cityPath, "city.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}
