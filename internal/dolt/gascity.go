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
		PidFile:        filepath.Join(gcDir, "dolt.pid"),
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
//  3. Init database for the city
//  4. Start the dolt server
//  5. Write .beads/metadata.json for bd CLI
//  6. Run bd init --server
//
// Idempotent: skips steps already completed.
func InitCity(cityPath, cityName string, stderr io.Writer) error {
	// 1. Ensure dolt identity.
	if err := EnsureDoltIdentity(); err != nil {
		return fmt.Errorf("dolt identity: %w", err)
	}

	config := GasCityConfig(cityPath)

	// 2. Create dolt-data dir.
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return fmt.Errorf("creating dolt-data: %w", err)
	}

	// 3. Init database for the city (idempotent).
	if err := initCityDatabase(cityName, config); err != nil {
		return fmt.Errorf("init database %q: %w", cityName, err)
	}

	// 4. Start the dolt server.
	if err := startCityServer(config, stderr); err != nil {
		return fmt.Errorf("starting dolt: %w", err)
	}

	// 5. Write .beads/metadata.json for bd CLI.
	if err := writeCityMetadata(cityPath, cityName); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	// 6. Run bd init --server.
	if err := runBdInit(cityPath); err != nil {
		return fmt.Errorf("bd init: %w", err)
	}

	return nil
}

// EnsureRunning starts the dolt server if not already running.
// Called by gc start.
func EnsureRunning(cityPath string) error {
	running, _, err := IsRunningCity(cityPath)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	config := GasCityConfig(cityPath)
	return startCityServer(config, os.Stderr)
}

// StopCity stops the dolt server for the given city.
// Called by gc stop. Idempotent: returns nil if already stopped.
func StopCity(cityPath string) error {
	running, _, err := IsRunningCity(cityPath)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	return Stop(cityPath)
}

// IsRunningCity checks if a dolt server is running for the given city.
// Returns (running, pid, error).
func IsRunningCity(cityPath string) (bool, int, error) {
	return IsRunning(cityPath)
}

// initCityDatabase initializes a dolt database for the city.
// Idempotent: skips if database already exists on disk.
func initCityDatabase(cityName string, config *Config) error {
	rigDir := filepath.Join(config.DataDir, cityName)

	// Check if already exists on disk — idempotent.
	if _, err := os.Stat(filepath.Join(rigDir, ".dolt")); err == nil {
		return nil
	}

	// Create directory and init offline (server not running yet).
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		return fmt.Errorf("creating database directory: %w", err)
	}

	cmd := exec.Command("dolt", "init")
	cmd.Dir = rigDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initializing dolt database: %w\n%s", err, output)
	}

	return nil
}

// startCityServer starts the dolt sql-server process using a Gas City config.
// The standard Start() function uses DefaultConfig which puts state in daemon/.
// This function uses GasCityConfig paths (.gc/).
func startCityServer(config *Config, _ io.Writer) error {
	// Ensure directory for log/pid/lock files exists.
	logDir := filepath.Dir(config.LogFile)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	// Check if already running.
	running, pid, err := IsRunning(config.TownRoot)
	if err != nil {
		return fmt.Errorf("checking server status: %w", err)
	}
	if running {
		return fmt.Errorf("dolt server already running (PID %d)", pid)
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

	// Write PID file.
	if err := os.WriteFile(config.PidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Save state.
	stateFile := filepath.Join(filepath.Dir(config.PidFile), "dolt-state.json")
	state := &State{
		Running:   true,
		PID:       cmd.Process.Pid,
		Port:      config.Port,
		StartedAt: time.Now(),
		DataDir:   config.DataDir,
	}
	if data, marshalErr := json.MarshalIndent(state, "", "  "); marshalErr == nil {
		_ = atomicWriteFile(stateFile, append(data, '\n'), 0o644)
	}

	// Wait for server to accept connections with retry.
	for attempt := 0; attempt < 10; attempt++ {
		time.Sleep(500 * time.Millisecond)

		// Check if process is still alive.
		if err := cmd.Process.Signal(os.Signal(nil)); err != nil {
			break // Process exited — don't keep retrying.
		}

		if err := CheckServerReachable(config.TownRoot); err == nil {
			return nil // Server is ready.
		}
	}

	// Check one more time before giving up.
	if err := CheckServerReachable(config.TownRoot); err == nil {
		return nil
	}

	return fmt.Errorf("dolt server started (PID %d) but not accepting connections after 5s", cmd.Process.Pid)
}

// writeCityMetadata writes .beads/metadata.json at the city root for bd CLI.
func writeCityMetadata(cityPath, cityName string) error {
	beadsDir := filepath.Join(cityPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return fmt.Errorf("creating .beads dir: %w", err)
	}

	meta := CityMetadata{
		Database:     "dolt",
		Backend:      "dolt",
		DoltMode:     "server",
		DoltDatabase: cityName,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	metadataPath := filepath.Join(beadsDir, "metadata.json")
	return atomicWriteFile(metadataPath, append(data, '\n'), 0o644)
}

// runBdInit runs `bd init --server` in the city directory.
func runBdInit(cityPath string) error {
	if _, err := exec.LookPath("bd"); err != nil {
		return fmt.Errorf("bd not found in PATH (install beads or set GC_BEADS=file)")
	}

	cmd := exec.Command("bd", "init", "--server")
	cmd.Dir = cityPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd init --server failed: %s", out)
	}
	return nil
}
