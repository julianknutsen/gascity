package dolt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
)

func TestGasCityConfig_Paths(t *testing.T) {
	cityPath := "/home/user/bright-lights"
	config := GasCityConfig(cityPath)

	if config.TownRoot != cityPath {
		t.Errorf("TownRoot = %q, want %q", config.TownRoot, cityPath)
	}
	if want := filepath.Join(cityPath, ".gc", "dolt-data"); config.DataDir != want {
		t.Errorf("DataDir = %q, want %q", config.DataDir, want)
	}
	if want := filepath.Join(cityPath, ".gc", "dolt.log"); config.LogFile != want {
		t.Errorf("LogFile = %q, want %q", config.LogFile, want)
	}
	if config.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", config.Port, DefaultPort)
	}
	if config.User != DefaultUser {
		t.Errorf("User = %q, want %q", config.User, DefaultUser)
	}
	if config.MaxConnections != DefaultMaxConnections {
		t.Errorf("MaxConnections = %d, want %d", config.MaxConnections, DefaultMaxConnections)
	}
}

func TestGasCityConfig_EnvOverrides(t *testing.T) {
	t.Setenv("GC_DOLT_HOST", "remote.example.com")
	t.Setenv("GC_DOLT_PORT", "3308")
	t.Setenv("GC_DOLT_USER", "testuser")
	t.Setenv("GC_DOLT_PASSWORD", "secret")

	config := GasCityConfig("/tmp/test-city")

	if config.Host != "remote.example.com" {
		t.Errorf("Host = %q, want %q", config.Host, "remote.example.com")
	}
	if config.Port != 3308 {
		t.Errorf("Port = %d, want 3308", config.Port)
	}
	if config.User != "testuser" {
		t.Errorf("User = %q, want %q", config.User, "testuser")
	}
	if config.Password != "secret" {
		t.Errorf("Password = %q, want %q", config.Password, "secret")
	}
}

func TestGasCityConfig_InvalidPort(t *testing.T) {
	t.Setenv("GC_DOLT_PORT", "not-a-number")

	config := GasCityConfig("/tmp/test-city")

	if config.Port != DefaultPort {
		t.Errorf("Port = %d, want default %d when env is invalid", config.Port, DefaultPort)
	}
}

func TestParseDataDir(t *testing.T) {
	tests := []struct {
		cmdline string
		want    string
	}{
		{"dolt sql-server --port 3307 --data-dir /home/user/city/.gc/dolt-data --max-connections 50", "/home/user/city/.gc/dolt-data"},
		{"dolt sql-server --data-dir=/tmp/data", "/tmp/data"},
		{"dolt sql-server --port 3307", ""},
		{"", ""},
		{"dolt sql-server --data-dir /a/b", "/a/b"},
	}
	for _, tt := range tests {
		got := parseDataDir(tt.cmdline)
		if got != tt.want {
			t.Errorf("parseDataDir(%q) = %q, want %q", tt.cmdline, got, tt.want)
		}
	}
}

func TestFindDoltServerForDataDir_NoServer(t *testing.T) {
	// Use a port that's definitely not in use.
	pid := findDoltServerForDataDir(19999, "/nonexistent")
	if pid != 0 {
		t.Errorf("findDoltServerForDataDir on unused port returned PID %d, want 0", pid)
	}
}

func TestWriteCityMetadata(t *testing.T) {
	cityPath := t.TempDir()
	cityName := "bright-lights"

	if err := writeCityMetadata(cityPath, cityName); err != nil {
		t.Fatalf("writeCityMetadata() error = %v", err)
	}

	metadataPath := filepath.Join(cityPath, ".beads", "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("reading metadata.json: %v", err)
	}

	var meta CityMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("parsing metadata.json: %v", err)
	}

	if meta.Database != "dolt" {
		t.Errorf("database = %q, want %q", meta.Database, "dolt")
	}
	if meta.Backend != "dolt" {
		t.Errorf("backend = %q, want %q", meta.Backend, "dolt")
	}
	if meta.DoltMode != "server" {
		t.Errorf("dolt_mode = %q, want %q", meta.DoltMode, "server")
	}
	if meta.DoltDatabase != cityName {
		t.Errorf("dolt_database = %q, want %q", meta.DoltDatabase, cityName)
	}
}

func TestWriteCityMetadata_Idempotent(t *testing.T) {
	cityPath := t.TempDir()
	cityName := "test-city"

	// Write twice.
	if err := writeCityMetadata(cityPath, cityName); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeCityMetadata(cityPath, cityName); err != nil {
		t.Fatalf("second write: %v", err)
	}

	// Verify content is correct after second write.
	data, err := os.ReadFile(filepath.Join(cityPath, ".beads", "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta CityMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.DoltDatabase != cityName {
		t.Errorf("dolt_database = %q, want %q", meta.DoltDatabase, cityName)
	}
}

func TestWriteCityMetadata_CreatesBeadsDir(t *testing.T) {
	cityPath := t.TempDir()

	if err := writeCityMetadata(cityPath, "test"); err != nil {
		t.Fatalf("writeCityMetadata() error = %v", err)
	}

	beadsDir := filepath.Join(cityPath, ".beads")
	fi, err := os.Stat(beadsDir)
	if err != nil {
		t.Fatalf(".beads dir not created: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".beads is not a directory")
	}
}

func TestRunBdInit_Idempotent(t *testing.T) {
	cityPath := t.TempDir()

	// Pre-create .beads/metadata.json to simulate already-initialized state.
	beadsDir := filepath.Join(cityPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"backend":"dolt"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The runner should never be called (idempotent — already initialized).
	neverCalled := func(_, _ string, _ ...string) ([]byte, error) {
		t.Fatal("runner should not be called for idempotent init")
		return nil, nil
	}
	store := beads.NewBdStore(cityPath, neverCalled)

	// Should be a no-op (idempotent) — doesn't need bd installed.
	if err := runBdInit(store, cityPath, "test", "localhost", 3307); err != nil {
		t.Errorf("runBdInit() error = %v, want nil (idempotent)", err)
	}
}

// Regression: Bug 2 — patchMetadataConnection must NOT set dolt_database.
// The dolt_database field is owned by bd init (set via -p flag). If
// patchMetadataConnection overwrites it, rigs get the wrong database name.
func TestPatchMetadataConnection_PreservesDoltDatabase(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate bd init having written metadata with its own dolt_database.
	existing := `{"dolt_database":"fe","issue_prefix":"fe"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := patchMetadataConnection(dir); err != nil {
		t.Fatalf("patchMetadataConnection() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}

	// dolt_database must be preserved (owned by bd).
	if meta["dolt_database"] != "fe" {
		t.Errorf("dolt_database = %v, want %q (bd's value must be preserved)", meta["dolt_database"], "fe")
	}
	// issue_prefix must be preserved (owned by bd).
	if meta["issue_prefix"] != "fe" {
		t.Errorf("issue_prefix = %v, want %q (bd's value must be preserved)", meta["issue_prefix"], "fe")
	}
	// Connection fields must be set.
	if meta["database"] != "dolt" {
		t.Errorf("database = %v, want %q", meta["database"], "dolt")
	}
	if meta["backend"] != "dolt" {
		t.Errorf("backend = %v, want %q", meta["backend"], "dolt")
	}
	if meta["dolt_mode"] != "server" {
		t.Errorf("dolt_mode = %v, want %q", meta["dolt_mode"], "server")
	}
}

func TestPatchMetadataConnection_CreatesBeadsDir(t *testing.T) {
	dir := t.TempDir()

	if err := patchMetadataConnection(dir); err != nil {
		t.Fatalf("patchMetadataConnection() error = %v", err)
	}

	// Verify .beads dir was created.
	fi, err := os.Stat(filepath.Join(dir, ".beads"))
	if err != nil {
		t.Fatalf(".beads dir not created: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".beads is not a directory")
	}

	// Verify metadata.json has connection fields but no dolt_database.
	data, err := os.ReadFile(filepath.Join(dir, ".beads", "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["database"] != "dolt" {
		t.Errorf("database = %v, want %q", meta["database"], "dolt")
	}
	// dolt_database should NOT be set by patchMetadataConnection.
	if _, ok := meta["dolt_database"]; ok {
		t.Errorf("dolt_database should not be set by patchMetadataConnection, got %v", meta["dolt_database"])
	}
}

func TestEnsureRunning_remote(t *testing.T) {
	cityPath := t.TempDir()

	// Unreachable remote host — EnsureRunning should return an error.
	t.Setenv("GC_DOLT_HOST", "192.0.2.1") // TEST-NET-1, not routable
	t.Setenv("GC_DOLT_PORT", "39999")

	err := EnsureRunning(cityPath)
	if err == nil {
		t.Error("EnsureRunning() should fail for unreachable remote host")
	}
	if !strings.Contains(err.Error(), "remote dolt") {
		t.Errorf("error should mention 'remote dolt', got: %v", err)
	}
}

func TestIsRunningCity_remote(t *testing.T) {
	cityPath := t.TempDir()

	// Unreachable remote → not running, no error.
	t.Setenv("GC_DOLT_HOST", "192.0.2.1") // TEST-NET-1, not routable
	t.Setenv("GC_DOLT_PORT", "39999")

	running, pid, err := IsRunningCity(cityPath)
	if err != nil {
		t.Fatalf("IsRunningCity() unexpected error: %v", err)
	}
	if running {
		t.Error("IsRunningCity() = true for unreachable remote, want false")
	}
	if pid != 0 {
		t.Errorf("pid = %d, want 0 for unreachable remote", pid)
	}
}

func TestStopCity_remote_noop(t *testing.T) {
	cityPath := t.TempDir()

	// Remote host — StopCity should be a no-op.
	t.Setenv("GC_DOLT_HOST", "remote.example.com")
	t.Setenv("GC_DOLT_PORT", "3307")

	err := StopCity(cityPath)
	if err != nil {
		t.Errorf("StopCity() = %v for remote, want nil (no-op)", err)
	}
}

func TestInitRigBeads_Idempotent(t *testing.T) {
	rigPath := t.TempDir()

	// Pre-create .beads/metadata.json to simulate already-initialized state.
	beadsDir := filepath.Join(rigPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_database":"fe"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The runner should never be called (idempotent — already initialized).
	neverCalled := func(_, _ string, _ ...string) ([]byte, error) {
		t.Fatal("runner should not be called for idempotent init")
		return nil, nil
	}
	store := beads.NewBdStore(rigPath, neverCalled)

	// Should be a no-op (idempotent) — doesn't need bd installed.
	if err := InitRigBeads(store, rigPath, "fe", "localhost", 3307); err != nil {
		t.Errorf("InitRigBeads() error = %v, want nil (idempotent)", err)
	}
}
