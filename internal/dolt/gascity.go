// Package dolt manages the Dolt SQL server for Gas City.
//
// This file maps Gas City directory conventions to the copied doltserver
// functions. All Gas City–specific paths live in .gc/ (city-scoped).
package dolt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GasCityConfig returns a dolt Config for the given city path.
// All state lives in .gc/ subdirectory (city-scoped, not global).
// Environment variables GC_DOLT_HOST, GC_DOLT_PORT, GC_DOLT_USER,
// GC_DOLT_PASSWORD override the defaults.
func GasCityConfig(cityPath string) *Config {
	gcDir := filepath.Join(cityPath, ".gc")
	config := &Config{
		TownRoot:       cityPath,
		Port:           DefaultPort,
		User:           DefaultUser,
		DataDir:        filepath.Join(gcDir, "dolt-data"),
		LogFile:        filepath.Join(gcDir, "dolt.log"),
		MaxConnections: DefaultMaxConnections,
	}

	if h := os.Getenv("GC_DOLT_HOST"); h != "" {
		config.Host = h
	}
	if p := os.Getenv("GC_DOLT_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			config.Port = port
		}
	}
	if u := os.Getenv("GC_DOLT_USER"); u != "" {
		config.User = u
	}
	if pw := os.Getenv("GC_DOLT_PASSWORD"); pw != "" {
		config.Password = pw
	}

	return config
}

// CityMetadata is the metadata.json content written to .beads/ for bd CLI
// to discover the dolt server connection.
type CityMetadata struct {
	Database     string `json:"database"`
	Backend      string `json:"backend"`
	DoltMode     string `json:"dolt_mode"`
	DoltDatabase string `json:"dolt_database"`
}

// InitCity sets up dolt for a Gas City instance:
//  1. EnsureDoltIdentity (copy git user.name/email if needed)
//  2. Create dolt-data dir
//  3. Start the dolt server
//  4. Run bd init --server (creates .beads/ and database on server)
//  5. Write .beads/metadata.json with Gas City fields
//
// Idempotent: skips steps already completed.
func InitCity(cityPath, cityName string, _ io.Writer) error {
	// 1. Ensure dolt identity.
	if err := EnsureDoltIdentity(); err != nil {
		return fmt.Errorf("dolt identity: %w", err)
	}

	config := GasCityConfig(cityPath)

	// 2. Create dolt-data dir.
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating dolt-data: %w", err)
	}

	// 3. Ensure the dolt server is running (idempotent — no error if already up).
	if err := EnsureRunning(cityPath); err != nil {
		return fmt.Errorf("starting dolt: %w", err)
	}

	// 4. Run bd init --server (creates .beads/ and database on server).
	if err := runBdInit(cityPath, cityName); err != nil {
		return fmt.Errorf("bd init: %w", err)
	}

	// 5. Write .beads/metadata.json with Gas City fields (overwrites bd's metadata).
	if err := writeCityMetadata(cityPath, cityName); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	return nil
}

// EnsureRunning starts the dolt server if not already running for this city.
// Returns an error if a different city's dolt server occupies the port.
// Called by gc start and gc init.
func EnsureRunning(cityPath string) error {
	running, _, err := IsRunningCity(cityPath)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	// No server for this city — but check if another city's server holds the port.
	config := GasCityConfig(cityPath)
	occupantPID := findDoltServerOnPort(config.Port)
	if occupantPID > 0 {
		return fmt.Errorf("port %d is occupied by another dolt server (PID %d); "+
			"kill it first: kill %d", config.Port, occupantPID, occupantPID)
	}

	return startCityServer(config, os.Stderr)
}

// StopCity stops the dolt server for the given city.
// Called by gc stop. Idempotent: returns nil if already stopped.
func StopCity(cityPath string) error {
	running, pid, err := IsRunningCity(cityPath)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding dolt process %d: %w", pid, err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("stopping dolt server (PID %d): %w", pid, err)
	}
	return nil
}

// IsRunningCity checks if a dolt server is running for the given city by
// querying the process table. Finds dolt on the configured port, then
// inspects its --data-dir to confirm it belongs to this city.
// No PID files — always queries live system state.
func IsRunningCity(cityPath string) (bool, int, error) {
	config := GasCityConfig(cityPath)
	pid := findDoltServerForDataDir(config.Port, config.DataDir)
	if pid > 0 {
		return true, pid, nil
	}
	return false, 0, nil
}

// findDoltServerForDataDir finds a dolt sql-server on the given port whose
// --data-dir matches the expected path. Returns the PID or 0 if not found.
func findDoltServerForDataDir(port int, expectedDataDir string) int {
	pid := findDoltServerOnPort(port)
	if pid == 0 {
		return 0
	}
	dataDir := doltProcessDataDir(pid)
	if dataDir == "" {
		return 0
	}
	// Normalize both paths for comparison.
	absExpected, err1 := filepath.Abs(expectedDataDir)
	absActual, err2 := filepath.Abs(dataDir)
	if err1 != nil || err2 != nil {
		return 0
	}
	if absExpected == absActual {
		return pid
	}
	return 0
}

// doltProcessDataDir extracts the --data-dir argument from a running dolt
// process's command line via ps.
func doltProcessDataDir(pid int) string {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseDataDir(strings.TrimSpace(string(output)))
}

// parseDataDir extracts the --data-dir value from a dolt command line string.
func parseDataDir(cmdline string) string {
	fields := strings.Fields(cmdline)
	for i, f := range fields {
		if f == "--data-dir" && i+1 < len(fields) {
			return fields[i+1]
		}
		if strings.HasPrefix(f, "--data-dir=") {
			return strings.TrimPrefix(f, "--data-dir=")
		}
	}
	return ""
}

// startCityServer starts the dolt sql-server process using a Gas City config.
// No PID or state files are written — all detection is via process table queries.
func startCityServer(config *Config, _ io.Writer) error {
	// Ensure directory for log file exists.
	logDir := filepath.Dir(config.LogFile)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	// Ensure data directory exists.
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Open log file.
	logFile, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	// Build dolt sql-server command.
	args := []string{
		"sql-server",
		"--port", strconv.Itoa(config.Port),
		"--data-dir", config.DataDir,
	}
	if config.MaxConnections > 0 {
		args = append(args, "--max-connections", strconv.Itoa(config.MaxConnections))
	}
	cmd := exec.Command("dolt", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("starting dolt server: %w", err)
	}

	_ = logFile.Close()

	// Wait for server to accept connections with retry.
	processExited := false
	for attempt := 0; attempt < 30; attempt++ {
		time.Sleep(500 * time.Millisecond)

		// Check if process is still alive.
		if err := cmd.Process.Signal(os.Signal(nil)); err != nil {
			processExited = true
			break // Process exited — don't keep retrying.
		}

		if err := CheckServerReachable(config.TownRoot); err == nil {
			return nil // Server is ready.
		}
	}

	// Check one more time before giving up — another server may be handling the port.
	if err := CheckServerReachable(config.TownRoot); err == nil {
		return nil
	}

	if processExited {
		return fmt.Errorf("dolt server (PID %d) exited immediately; check logs: %s", cmd.Process.Pid, config.LogFile)
	}
	return fmt.Errorf("dolt server started (PID %d) but not accepting connections after 15s", cmd.Process.Pid)
}

// writeCityMetadata patches .beads/metadata.json at the city root with Gas City
// dolt server fields. Merges into existing metadata (preserving bd's fields like
// issue_prefix, jsonl_export, etc.) rather than overwriting.
func writeCityMetadata(cityPath, cityName string) error {
	beadsDir := filepath.Join(cityPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return fmt.Errorf("creating .beads dir: %w", err)
	}

	metadataPath := filepath.Join(beadsDir, "metadata.json")

	// Load existing metadata (preserve bd's fields).
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &existing) // best effort
	}

	// Patch Gas City dolt fields. dolt_database is owned by bd init —
	// only set it as a fallback when bd hasn't run yet (no existing value).
	existing["database"] = "dolt"
	existing["backend"] = "dolt"
	existing["dolt_mode"] = "server"
	if existing["dolt_database"] == nil || existing["dolt_database"] == "" {
		existing["dolt_database"] = cityName
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	return atomicWriteFile(metadataPath, append(data, '\n'), 0o644)
}

// runBdInit runs `bd init --server` in the city directory, then explicitly
// sets the issue_prefix config (required for bd create to work).
// Idempotent: skips if .beads/metadata.json already exists.
func runBdInit(cityPath, cityName string) error {
	// Idempotent: skip if already initialized.
	if _, err := os.Stat(filepath.Join(cityPath, ".beads", "metadata.json")); err == nil {
		return nil
	}

	if _, err := exec.LookPath("bd"); err != nil {
		return fmt.Errorf("bd not found in PATH (install beads or set GC_BEADS=file)")
	}

	cmd := exec.Command("bd", "init", "--server", "-p", cityName, "--skip-hooks")
	cmd.Dir = cityPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd init --server failed: %s", out)
	}

	// Remove AGENTS.md written by bd init — Gas City manages its own
	// agent prompts via prompt_template, so bd's AGENTS.md is unwanted.
	os.Remove(filepath.Join(cityPath, "AGENTS.md")) //nolint:errcheck // best-effort cleanup

	// Explicitly set issue_prefix (bd init --prefix may not persist it).
	// Without this, bd create fails with "issue_prefix config is missing".
	prefixCmd := exec.Command("bd", "config", "set", "issue_prefix", cityName)
	prefixCmd.Dir = cityPath
	if out, err := prefixCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd config set issue_prefix failed: %s", out)
	}

	return nil
}
