package supervisor

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds machine-wide supervisor configuration loaded from
// ~/.gc/supervisor.toml (or $GC_HOME/supervisor.toml).
type Config struct {
	Supervisor Section `toml:"supervisor"`
}

// Section holds the [supervisor] table fields.
type Section struct {
	Port           int    `toml:"port,omitempty"`
	Bind           string `toml:"bind,omitempty"`
	PatrolInterval string `toml:"patrol_interval,omitempty"`
	AllowMutations bool   `toml:"allow_mutations,omitempty"`
}

// BindOrDefault returns the bind address, defaulting to "127.0.0.1".
func (s Section) BindOrDefault() string {
	if s.Bind == "" {
		return "127.0.0.1"
	}
	return s.Bind
}

// PortOrDefault returns the API port, defaulting to 8372.
func (s Section) PortOrDefault() int {
	if s.Port <= 0 {
		return 8372
	}
	return s.Port
}

// PatrolIntervalDuration returns the patrol interval as a time.Duration.
// Defaults to 10s on empty or unparseable values.
func (s Section) PatrolIntervalDuration() time.Duration {
	if s.PatrolInterval == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(s.PatrolInterval)
	if err != nil || d <= 0 {
		return 10 * time.Second
	}
	return d
}

// LoadConfig loads supervisor config from the given path. Returns a
// zero-value Config (with defaults) if the file doesn't exist.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// DefaultHome returns the default GC home directory (~/.gc). Respects
// the GC_HOME environment variable override.
func DefaultHome() string {
	if v := os.Getenv("GC_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".gc")
	}
	return filepath.Join(home, ".gc")
}

// RuntimeDir returns the directory for ephemeral runtime files (lock,
// socket). Uses $XDG_RUNTIME_DIR/gc if available, falls back to
// DefaultHome().
func RuntimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return filepath.Join(v, "gc")
	}
	return DefaultHome()
}

// RegistryPath returns the path to the cities.toml registry file.
func RegistryPath() string {
	return filepath.Join(DefaultHome(), "cities.toml")
}

// ConfigPath returns the path to the supervisor.toml config file.
func ConfigPath() string {
	return filepath.Join(DefaultHome(), "supervisor.toml")
}
