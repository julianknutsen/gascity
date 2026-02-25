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
