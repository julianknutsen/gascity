// Package config handles loading and parsing city.toml configuration files.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/fsys"
)

// City is the top-level configuration for a Gas City instance.
type City struct {
	Workspace Workspace `toml:"workspace"`
	Agents    []Agent   `toml:"agents"`
}

// Workspace holds city-level metadata.
type Workspace struct {
	Name string `toml:"name"`
}

// Agent defines a configured agent in the city.
type Agent struct {
	Name         string `toml:"name"`
	Provider     string `toml:"provider"`
	StartCommand string `toml:"start_command"`
}

// Load reads and parses a city.toml file at the given path using the
// provided filesystem. All file I/O goes through fs for testability.
func Load(fs fsys.FS, path string) (*City, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading config %q: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes TOML data into a City config.
func Parse(data []byte) (*City, error) {
	var cfg City
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}
