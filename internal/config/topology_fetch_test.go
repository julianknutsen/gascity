package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

// initBareRepo creates a bare git repo with a topology.toml file.
// Returns the bare repo path.
func initBareRepo(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()

	// Create a working repo to populate, then bare-clone it.
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, name+".git")

	mustGit(t, "", "init", workDir)

	topoContent := `[topology]
name = "` + name + `"
version = "1.0.0"
schema = 1

[[agents]]
name = "worker"
`
	if err := os.MkdirAll(filepath.Join(workDir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "topology.toml"), []byte(topoContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "prompts", "worker.md"), []byte("you are a worker"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustGit(t, workDir, "add", "-A")
	mustGit(t, workDir, "commit", "-m", "initial")
	mustGit(t, workDir, "clone", "--bare", workDir, bareDir)

	return bareDir
}

// initBareRepoWithTag creates a bare repo with an initial commit tagged.
func initBareRepoWithTag(t *testing.T, name, tag string) string {
	t.Helper()
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, name+".git")

	mustGit(t, "", "init", workDir)

	topoContent := `[topology]
name = "` + name + `"
version = "` + tag + `"
schema = 1

[[agents]]
name = "worker"
`
	if err := os.WriteFile(filepath.Join(workDir, "topology.toml"), []byte(topoContent), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workDir, "add", "-A")
	mustGit(t, workDir, "commit", "-m", "release "+tag)
	mustGit(t, workDir, "tag", "-a", tag, "-m", "Release "+tag)

	mustGit(t, workDir, "clone", "--bare", workDir, bareDir)
	return bareDir
}

// initBareRepoWithBranch creates a bare repo with a named branch.
func initBareRepoWithBranch(t *testing.T, name, branch string) string {
	t.Helper()
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, name+".git")

	mustGit(t, "", "init", workDir)

	topoContent := `[topology]
name = "` + name + `"
version = "0.1.0"
schema = 1
`
	if err := os.WriteFile(filepath.Join(workDir, "topology.toml"), []byte(topoContent), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workDir, "add", "-A")
	mustGit(t, workDir, "commit", "-m", "initial on main")

	// Create branch with different content.
	mustGit(t, workDir, "checkout", "-b", branch)
	topoContent = `[topology]
name = "` + name + `"
version = "0.2.0-` + branch + `"
schema = 1
`
	if err := os.WriteFile(filepath.Join(workDir, "topology.toml"), []byte(topoContent), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workDir, "add", "-A")
	mustGit(t, workDir, "commit", "-m", "update on "+branch)

	mustGit(t, workDir, "clone", "--bare", workDir, bareDir)
	return bareDir
}

// gitEnvBlacklist lists git environment variables stripped from test
// commands to prevent the parent project's hooks and repo state from
// leaking into temp test repos (e.g., GIT_DIR set by pre-commit hooks).
var testGitEnvBlacklist = map[string]bool{
	"GIT_DIR":                          true,
	"GIT_WORK_TREE":                    true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	// Prepend -c core.hooksPath= to disable hooks inherited from the
	// parent project config (core.hooksPath leaks via local git config).
	fullArgs := append([]string{"-c", "core.hooksPath="}, args...)
	cmd := exec.Command("git", fullArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Build clean env: strip git-specific vars that leak from the parent
	// project's pre-commit hook context.
	for _, e := range os.Environ() {
		if k, _, ok := strings.Cut(e, "="); ok && testGitEnvBlacklist[k] {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	cmd.Env = append(cmd.Env,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), string(out), err)
	}
}

func TestCloneTopology(t *testing.T) {
	bare := initBareRepo(t, "test-topo")
	cacheDir := filepath.Join(t.TempDir(), "cached")

	if err := cloneTopology(bare, cacheDir, ""); err != nil {
		t.Fatalf("cloneTopology: %v", err)
	}

	// Verify topology.toml exists in cache.
	if _, err := os.Stat(filepath.Join(cacheDir, "topology.toml")); err != nil {
		t.Errorf("topology.toml not found in cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "prompts", "worker.md")); err != nil {
		t.Errorf("prompts/worker.md not found in cache: %v", err)
	}
}

func TestCloneTopology_WithTag(t *testing.T) {
	bare := initBareRepoWithTag(t, "tagged", "v1.0.0")
	cacheDir := filepath.Join(t.TempDir(), "cached")

	if err := cloneTopology(bare, cacheDir, "v1.0.0"); err != nil {
		t.Fatalf("cloneTopology with tag: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, "topology.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `version = "v1.0.0"`) {
		t.Errorf("expected tagged version, got: %s", data)
	}
}

func TestCloneTopology_WithBranch(t *testing.T) {
	bare := initBareRepoWithBranch(t, "branched", "develop")
	cacheDir := filepath.Join(t.TempDir(), "cached")

	if err := cloneTopology(bare, cacheDir, "develop"); err != nil {
		t.Fatalf("cloneTopology with branch: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, "topology.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "develop") {
		t.Errorf("expected branch version, got: %s", data)
	}
}

func TestUpdateTopology(t *testing.T) {
	// Create bare repo, clone it, then add a commit to the bare repo
	// (via a temporary worktree), and verify update fetches it.
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	bareDir := filepath.Join(dir, "test.git")

	mustGit(t, "", "init", workDir)
	if err := os.WriteFile(filepath.Join(workDir, "topology.toml"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workDir, "add", "-A")
	mustGit(t, workDir, "commit", "-m", "v1")
	mustGit(t, workDir, "clone", "--bare", workDir, bareDir)

	// Clone into cache.
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := cloneTopology(bareDir, cacheDir, ""); err != nil {
		t.Fatal(err)
	}

	// Push a new commit to bare repo.
	pushDir := filepath.Join(dir, "push")
	mustGit(t, "", "clone", bareDir, pushDir)
	if err := os.WriteFile(filepath.Join(pushDir, "topology.toml"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, pushDir, "add", "-A")
	mustGit(t, pushDir, "commit", "-m", "v2")
	mustGit(t, pushDir, "push")

	// Update cache.
	if err := updateTopology(cacheDir, ""); err != nil {
		t.Fatalf("updateTopology: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, "topology.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2" {
		t.Errorf("expected v2 after update, got: %s", data)
	}
}

func TestFetchTopologies_ClonesMissing(t *testing.T) {
	bare := initBareRepo(t, "remote-topo")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	topos := map[string]TopologySource{
		"myremote": {Source: bare},
	}

	if err := FetchTopologies(topos, cityRoot); err != nil {
		t.Fatalf("FetchTopologies: %v", err)
	}

	// Verify cache exists.
	topoFile := filepath.Join(cityRoot, ".gc", "topologies", "myremote", "topology.toml")
	if _, err := os.Stat(topoFile); err != nil {
		t.Errorf("expected cache to exist: %v", err)
	}
}

func TestFetchTopologies_SkipsExisting(t *testing.T) {
	bare := initBareRepo(t, "skip-test")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	topos := map[string]TopologySource{
		"cached": {Source: bare},
	}

	// First fetch.
	if err := FetchTopologies(topos, cityRoot); err != nil {
		t.Fatal(err)
	}

	// Second fetch should not error (skips clone, does update).
	if err := FetchTopologies(topos, cityRoot); err != nil {
		t.Fatalf("second FetchTopologies should succeed: %v", err)
	}
}

func TestFetchTopologies_InvalidSource(t *testing.T) {
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	topos := map[string]TopologySource{
		"bad": {Source: "/nonexistent/repo.git"},
	}

	err := FetchTopologies(topos, cityRoot)
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention topology name, got: %v", err)
	}
}

func TestLockfile_RoundTrip(t *testing.T) {
	cityRoot := t.TempDir()
	lock := &TopologyLock{
		Topologies: map[string]LockedTopology{
			"gastown": {
				Source: "https://github.com/example/gastown",
				Ref:    "v1.0.0",
				Commit: "abc123def456",
				Hash:   "sha256:e3b0c44",
			},
		},
	}

	if err := WriteLock(cityRoot, lock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	got, err := ReadLock(cityRoot)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}

	if len(got.Topologies) != 1 {
		t.Fatalf("expected 1 topology, got %d", len(got.Topologies))
	}
	lt := got.Topologies["gastown"]
	if lt.Source != "https://github.com/example/gastown" {
		t.Errorf("Source = %q, want gastown URL", lt.Source)
	}
	if lt.Ref != "v1.0.0" {
		t.Errorf("Ref = %q, want v1.0.0", lt.Ref)
	}
	if lt.Commit != "abc123def456" {
		t.Errorf("Commit = %q, want abc123def456", lt.Commit)
	}
	if lt.Hash != "sha256:e3b0c44" {
		t.Errorf("Hash = %q, want sha256:e3b0c44", lt.Hash)
	}
}

func TestReadLock_MissingFile(t *testing.T) {
	cityRoot := t.TempDir()
	lock, err := ReadLock(cityRoot)
	if err != nil {
		t.Fatalf("ReadLock on missing file: %v", err)
	}
	if len(lock.Topologies) != 0 {
		t.Errorf("expected empty topologies, got %d", len(lock.Topologies))
	}
}

func TestLockFromCache(t *testing.T) {
	bare := initBareRepo(t, "locktest")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	topos := map[string]TopologySource{
		"locktest": {Source: bare},
	}

	if err := FetchTopologies(topos, cityRoot); err != nil {
		t.Fatal(err)
	}

	lock, err := LockFromCache(topos, cityRoot)
	if err != nil {
		t.Fatalf("LockFromCache: %v", err)
	}

	lt, ok := lock.Topologies["locktest"]
	if !ok {
		t.Fatal("expected locktest in lock")
	}
	if lt.Source != bare {
		t.Errorf("Source = %q, want %q", lt.Source, bare)
	}
	if lt.Commit == "" {
		t.Error("Commit should not be empty")
	}
	if !strings.HasPrefix(lt.Hash, "sha256:") {
		t.Errorf("Hash should start with sha256:, got %q", lt.Hash)
	}
}

func TestTopologyCachePath(t *testing.T) {
	got := TopologyCachePath("/city", "gastown", TopologySource{Source: "url"})
	want := "/city/.gc/topologies/gastown"
	if got != want {
		t.Errorf("TopologyCachePath = %q, want %q", got, want)
	}

	got = TopologyCachePath("/city", "mono", TopologySource{Source: "url", Path: "packages/topo"})
	want = "/city/.gc/topologies/mono/packages/topo"
	if got != want {
		t.Errorf("TopologyCachePath with Path = %q, want %q", got, want)
	}
}

func TestFetchTopologies_WithRefTag(t *testing.T) {
	bare := initBareRepoWithTag(t, "tagref", "v2.0.0")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	topos := map[string]TopologySource{
		"tagref": {Source: bare, Ref: "v2.0.0"},
	}

	if err := FetchTopologies(topos, cityRoot); err != nil {
		t.Fatalf("FetchTopologies with ref: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cityRoot, ".gc", "topologies", "tagref", "topology.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `version = "v2.0.0"`) {
		t.Errorf("expected tagged content, got: %s", data)
	}
}

func TestFetchRemoteInclude(t *testing.T) {
	bare := initBareRepo(t, "inc-test")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// First fetch: clone.
	cacheDir, err := fetchRemoteInclude(bare, "", cityRoot)
	if err != nil {
		t.Fatalf("fetchRemoteInclude: %v", err)
	}

	// Verify topology.toml exists in the cache.
	if _, err := os.Stat(filepath.Join(cacheDir, "topology.toml")); err != nil {
		t.Errorf("topology.toml not in cache: %v", err)
	}

	// Cache path should be under _inc/.
	if !strings.Contains(cacheDir, filepath.Join(".gc", "topologies", "_inc")) {
		t.Errorf("cacheDir = %q, want under _inc/", cacheDir)
	}

	// Idempotent: second fetch succeeds (updates existing).
	cacheDir2, err := fetchRemoteInclude(bare, "", cityRoot)
	if err != nil {
		t.Fatalf("second fetchRemoteInclude: %v", err)
	}
	if cacheDir2 != cacheDir {
		t.Errorf("cache path changed: %q → %q", cacheDir, cacheDir2)
	}
}

func TestFetchRemoteInclude_WithRef(t *testing.T) {
	bare := initBareRepoWithTag(t, "inc-tagged", "v1.0.0")
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	cacheDir, err := fetchRemoteInclude(bare, "v1.0.0", cityRoot)
	if err != nil {
		t.Fatalf("fetchRemoteInclude: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cacheDir, "topology.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `version = "v1.0.0"`) {
		t.Errorf("expected tagged content, got: %s", data)
	}
}

func TestLoadTopology_RemoteInclude(t *testing.T) {
	// Create a bare repo as the "remote" topology.
	bare := initBareRepo(t, "remote-maint")

	// Set up a city root and parent topology that includes the bare repo
	// via a remote URL. We pre-clone the bare repo into the _inc cache
	// to simulate what fetchRemoteInclude does, then verify loadTopology
	// picks up the included agents.
	cityRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityRoot, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate the include cache (simulates fetchRemoteInclude).
	cacheName := includeCacheName(bare)
	cacheDir := filepath.Join(cityRoot, ".gc", topologyCacheDir, includeCacheDir, cacheName)
	if err := cloneTopology(bare, cacheDir, ""); err != nil {
		t.Fatalf("pre-clone: %v", err)
	}

	// Use the cache dir as a local include (since it's now a local clone).
	// This tests the full flow: loadTopology reads the included topology.
	parentToml := `[topology]
name = "parent"
schema = 1
includes = ["` + cacheDir + `"]

[[agents]]
name = "boss"
`
	topoDir := filepath.Join(cityRoot, "topologies", "parent")
	if err := os.MkdirAll(topoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(topoDir, "topology.toml"), []byte(parentToml), 0o644); err != nil {
		t.Fatal(err)
	}

	agents, _, _, _, err := loadTopology(
		fsys.OSFS{},
		filepath.Join(topoDir, "topology.toml"),
		topoDir,
		cityRoot, "", nil)
	if err != nil {
		t.Fatalf("loadTopology with remote include: %v", err)
	}

	// Should have 2 agents: worker (from remote include) + boss (parent).
	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(agents))
	}
	if agents[0].Name != "worker" {
		t.Errorf("agents[0].Name = %q, want worker (from remote)", agents[0].Name)
	}
	if agents[1].Name != "boss" {
		t.Errorf("agents[1].Name = %q, want boss (from parent)", agents[1].Name)
	}
}
