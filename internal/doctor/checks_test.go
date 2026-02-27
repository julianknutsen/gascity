package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

// helper creates .gc/ and city.toml in a temp dir.
func setupCity(t *testing.T, tomlContent string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "city.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- CityStructureCheck ---

func TestCityStructureCheck_OK(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")
	c := &CityStructureCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestCityStructureCheck_MissingGC(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "city.toml"), []byte("[workspace]\nname = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &CityStructureCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

func TestCityStructureCheck_MissingToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := &CityStructureCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

// --- CityConfigCheck ---

func TestCityConfigCheck_OK(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")
	c := &CityConfigCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestCityConfigCheck_ParseError(t *testing.T) {
	dir := setupCity(t, "{{invalid toml")
	c := &CityConfigCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

func TestCityConfigCheck_NoName(t *testing.T) {
	dir := setupCity(t, "[workspace]\n")
	c := &CityConfigCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error; msg = %s", r.Status, r.Message)
	}
}

// --- ConfigValidCheck ---

func TestConfigValidCheck_OK(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents:    []config.Agent{{Name: "mayor"}},
	}
	c := NewConfigValidCheck(cfg)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestConfigValidCheck_BadAgent(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents:    []config.Agent{{Name: ""}}, // missing name
	}
	c := NewConfigValidCheck(cfg)
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

func TestConfigValidCheck_BadRig(t *testing.T) {
	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents:    []config.Agent{{Name: "mayor"}},
		Rigs:      []config.Rig{{Name: "rig1"}}, // missing path
	}
	c := NewConfigValidCheck(cfg)
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

// --- BinaryCheck ---

func TestBinaryCheck_Found(t *testing.T) {
	c := NewBinaryCheck("tmux", "", func(_ string) (string, error) {
		return "/usr/bin/tmux", nil
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestBinaryCheck_NotFound(t *testing.T) {
	c := NewBinaryCheck("tmux", "", func(_ string) (string, error) {
		return "", fmt.Errorf("not found")
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

func TestBinaryCheck_Skipped(t *testing.T) {
	c := NewBinaryCheck("bd", "skipped (GC_BEADS=file)", nil)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK (skipped)", r.Status)
	}
	if r.Message != "skipped (GC_BEADS=file)" {
		t.Errorf("message = %q, want skip message", r.Message)
	}
}

// --- AgentSessionsCheck ---

func TestAgentSessionsCheck_AllRunning(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}
	c := NewAgentSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestAgentSessionsCheck_Missing(t *testing.T) {
	sp := session.NewFake()
	// Don't start any sessions.

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}
	c := NewAgentSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusWarning {
		t.Errorf("status = %d, want Warning; msg = %s", r.Status, r.Message)
	}
}

func TestAgentSessionsCheck_SkipsSuspended(t *testing.T) {
	sp := session.NewFake()
	// Suspended agent has no session — that's fine.

	cfg := &config.City{
		Agents: []config.Agent{{Name: "worker", Suspended: true}},
	}
	c := NewAgentSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK (suspended skipped); msg = %s", r.Status, r.Message)
	}
}

// --- ZombieSessionsCheck ---

func TestZombieSessionsCheck_NoZombies(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor", ProcessNames: []string{"claude"}}},
	}
	c := NewZombieSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestZombieSessionsCheck_Found(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}
	sp.Zombies["gc-test-mayor"] = true

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor", ProcessNames: []string{"claude"}}},
	}
	c := NewZombieSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusWarning {
		t.Errorf("status = %d, want Warning; msg = %s", r.Status, r.Message)
	}
}

func TestZombieSessionsCheck_Fix(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}
	sp.Zombies["gc-test-mayor"] = true

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor", ProcessNames: []string{"claude"}}},
	}
	c := NewZombieSessionsCheck(cfg, "test", "", sp)
	if err := c.Fix(&CheckContext{}); err != nil {
		t.Fatalf("Fix() error: %v", err)
	}
	// After fix, session should be stopped.
	if sp.IsRunning("gc-test-mayor") {
		t.Error("zombie session still running after fix")
	}
}

func TestZombieSessionsCheck_SkipsNoProcessNames(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}
	sp.Zombies["gc-test-mayor"] = true // zombie but no process_names to check

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}}, // no ProcessNames
	}
	c := NewZombieSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK (no process_names = skip zombie check); msg = %s", r.Status, r.Message)
	}
}

// --- OrphanSessionsCheck ---

func TestOrphanSessionsCheck_NoOrphans(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}
	c := NewOrphanSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestOrphanSessionsCheck_Found(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := sp.Start("gc-test-stale-worker", session.Config{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}
	c := NewOrphanSessionsCheck(cfg, "test", "", sp)
	r := c.Run(&CheckContext{})
	if r.Status != StatusWarning {
		t.Errorf("status = %d, want Warning; msg = %s", r.Status, r.Message)
	}
}

func TestOrphanSessionsCheck_Fix(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-test-mayor", session.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := sp.Start("gc-test-stale-worker", session.Config{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.City{
		Agents: []config.Agent{{Name: "mayor"}},
	}
	c := NewOrphanSessionsCheck(cfg, "test", "", sp)
	if err := c.Fix(&CheckContext{}); err != nil {
		t.Fatalf("Fix() error: %v", err)
	}
	if sp.IsRunning("gc-test-stale-worker") {
		t.Error("orphan session still running after fix")
	}
	if !sp.IsRunning("gc-test-mayor") {
		t.Error("legitimate session was killed by fix")
	}
}

// --- BeadsStoreCheck ---

func TestBeadsStoreCheck_OK(t *testing.T) {
	dir := t.TempDir()
	// Create a file store.
	store, err := beads.OpenFileStore(fsys.OSFS{}, filepath.Join(dir, "beads.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Create a bead so List returns something.
	if _, err := store.Create(beads.Bead{Title: "test"}); err != nil {
		t.Fatal(err)
	}

	c := NewBeadsStoreCheck(dir, func(cityPath string) (beads.Store, error) {
		return beads.OpenFileStore(fsys.OSFS{}, filepath.Join(cityPath, "beads.json"))
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestBeadsStoreCheck_OpenError(t *testing.T) {
	c := NewBeadsStoreCheck("/nonexistent", func(_ string) (beads.Store, error) {
		return nil, fmt.Errorf("open failed")
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

// --- DoltServerCheck ---

func TestDoltServerCheck_Skipped(t *testing.T) {
	c := NewDoltServerCheck("/tmp", true)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK (skipped)", r.Status)
	}
}

// --- EventsLogCheck ---

func TestEventsLogCheck_OK(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gcDir, "events.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &EventsLogCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestEventsLogCheck_Missing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := &EventsLogCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusWarning {
		t.Errorf("status = %d, want Warning; msg = %s", r.Status, r.Message)
	}
}

// --- ControllerCheck ---

func TestControllerCheck_Running(t *testing.T) {
	c := NewControllerCheck("/tmp", true)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK", r.Status)
	}
	if r.Message != "controller running (sessions managed)" {
		t.Errorf("message = %q", r.Message)
	}
}

func TestControllerCheck_NotRunning(t *testing.T) {
	c := NewControllerCheck("/tmp", false)
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK", r.Status)
	}
	if r.Message != "controller not running (one-shot mode)" {
		t.Errorf("message = %q", r.Message)
	}
}

// --- RigPathCheck ---

func TestRigPathCheck_OK(t *testing.T) {
	dir := t.TempDir()
	c := NewRigPathCheck(config.Rig{Name: "myrig", Path: dir})
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestRigPathCheck_Missing(t *testing.T) {
	c := NewRigPathCheck(config.Rig{Name: "myrig", Path: "/nonexistent/path"})
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

// --- RigGitCheck ---

func TestRigGitCheck_OK(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := NewRigGitCheck(config.Rig{Name: "myrig", Path: dir})
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestRigGitCheck_NotGit(t *testing.T) {
	dir := t.TempDir()
	c := NewRigGitCheck(config.Rig{Name: "myrig", Path: dir})
	r := c.Run(&CheckContext{})
	if r.Status != StatusWarning {
		t.Errorf("status = %d, want Warning; msg = %s", r.Status, r.Message)
	}
}

// --- RigBeadsCheck ---

func TestRigBeadsCheck_OK(t *testing.T) {
	dir := t.TempDir()
	c := NewRigBeadsCheck(config.Rig{Name: "myrig", Path: dir}, func(rigPath string) (beads.Store, error) {
		return beads.OpenFileStore(fsys.OSFS{}, filepath.Join(rigPath, "beads.json"))
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestRigBeadsCheck_Error(t *testing.T) {
	c := NewRigBeadsCheck(config.Rig{Name: "myrig", Path: "/nonexistent"}, func(_ string) (beads.Store, error) {
		return nil, fmt.Errorf("store failed")
	})
	r := c.Run(&CheckContext{})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error", r.Status)
	}
}

// --- IsControllerRunning ---

func TestIsControllerRunning_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No lock file, no controller.
	if IsControllerRunning(dir) {
		t.Error("expected false when no lock exists")
	}
}

func TestIsControllerRunning_UnlockedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create lock file but don't hold the lock.
	if err := os.WriteFile(filepath.Join(dir, ".gc", "controller.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if IsControllerRunning(dir) {
		t.Error("expected false when lock file exists but not locked")
	}
}

// --- TopologyCacheCheck ---

func TestTopologyCacheCheck_OK(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".gc", "topologies", "gastown")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "topology.toml"), []byte("[topology]\nname=\"gastown\"\nschema=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewTopologyCacheCheck(map[string]config.TopologySource{
		"gastown": {Source: "https://example.com/gastown"},
	}, dir)
	ctx := &CheckContext{CityPath: dir}
	r := c.Run(ctx)
	if r.Status != StatusOK {
		t.Errorf("status = %v, want OK: %s", r.Status, r.Message)
	}
}

func TestTopologyCacheCheck_Missing(t *testing.T) {
	dir := t.TempDir()
	// No cache created.

	c := NewTopologyCacheCheck(map[string]config.TopologySource{
		"gastown": {Source: "https://example.com/gastown"},
	}, dir)
	ctx := &CheckContext{CityPath: dir}
	r := c.Run(ctx)
	if r.Status != StatusError {
		t.Errorf("status = %v, want Error: %s", r.Status, r.Message)
	}
	if r.FixHint == "" {
		t.Error("expected fix hint")
	}
}

func TestTopologyCacheCheck_WithPath(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".gc", "topologies", "mono", "packages", "topo")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "topology.toml"), []byte("[topology]\nname=\"mono\"\nschema=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewTopologyCacheCheck(map[string]config.TopologySource{
		"mono": {Source: "https://example.com/mono", Path: "packages/topo"},
	}, dir)
	ctx := &CheckContext{CityPath: dir}
	r := c.Run(ctx)
	if r.Status != StatusOK {
		t.Errorf("status = %v, want OK: %s", r.Status, r.Message)
	}
}

// --- WorktreeCheck ---

func TestWorktreeCheckNoWorktrees(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")
	c := &WorktreeCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestWorktreeCheckAllValid(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")

	// Create a worktree dir with a valid .git file pointing to a real target.
	wtDir := filepath.Join(dir, ".gc", "worktrees", "myrig", "agent1")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a real target directory that the gitdir points to.
	gitTarget := filepath.Join(dir, ".git", "worktrees", "agent1")
	if err := os.MkdirAll(gitTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write .git file (this is how git worktrees work: .git is a file, not a dir).
	gitContent := fmt.Sprintf("gitdir: %s\n", gitTarget)
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte(gitContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &WorktreeCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK; msg = %s", r.Status, r.Message)
	}
}

func TestWorktreeCheckBroken(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")

	// Create a worktree with a .git file pointing to a nonexistent path.
	wtDir := filepath.Join(dir, ".gc", "worktrees", "myrig", "agent1")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitContent := "gitdir: /nonexistent/.git/worktrees/agent1\n"
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte(gitContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &WorktreeCheck{}
	r := c.Run(&CheckContext{CityPath: dir})
	if r.Status != StatusError {
		t.Errorf("status = %d, want Error; msg = %s", r.Status, r.Message)
	}
	if len(r.Details) != 1 {
		t.Errorf("details = %v, want 1 broken entry", r.Details)
	}
}

func TestWorktreeCheckFix(t *testing.T) {
	dir := setupCity(t, "[workspace]\nname = \"test\"\n")

	// Create a broken worktree.
	wtDir := filepath.Join(dir, ".gc", "worktrees", "myrig", "agent1")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitContent := "gitdir: /nonexistent/.git/worktrees/agent1\n"
	if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte(gitContent), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &WorktreeCheck{}
	ctx := &CheckContext{CityPath: dir}

	// Verify it's broken first.
	r := c.Run(ctx)
	if r.Status != StatusError {
		t.Fatalf("status = %d, want Error before fix", r.Status)
	}

	// Fix should remove the broken directory.
	if err := c.Fix(ctx); err != nil {
		t.Fatalf("Fix() error: %v", err)
	}

	// After fix, the worktree dir should be gone.
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after Fix()")
	}

	// Re-run should be OK.
	r = c.Run(ctx)
	if r.Status != StatusOK {
		t.Errorf("status = %d, want OK after fix; msg = %s", r.Status, r.Message)
	}
}
