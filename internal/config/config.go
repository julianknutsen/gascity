// Package config handles loading and parsing city.toml configuration files.
package config

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/fsys"
)

// City is the top-level configuration for a Gas City instance.
type City struct {
	Workspace Workspace               `toml:"workspace"`
	Providers map[string]ProviderSpec `toml:"providers,omitempty"`
	Agents    []Agent                 `toml:"agents"`
	Rigs      []Rig                   `toml:"rigs,omitempty"`
	Beads     BeadsConfig             `toml:"beads,omitempty"`
	Dolt      DoltConfig              `toml:"dolt,omitempty"`
	Formulas  FormulasConfig          `toml:"formulas,omitempty"`
	Daemon    DaemonConfig            `toml:"daemon,omitempty"`
}

// Rig defines an external project registered in the city.
type Rig struct {
	Name   string `toml:"name"`
	Path   string `toml:"path"`
	Prefix string `toml:"prefix,omitempty"`
}

// EffectivePrefix returns the bead ID prefix for this rig. Uses the
// explicit Prefix if set, otherwise derives one from the Name.
func (r *Rig) EffectivePrefix() string {
	if r.Prefix != "" {
		return r.Prefix
	}
	return DeriveBeadsPrefix(r.Name)
}

// DeriveBeadsPrefix computes a short bead ID prefix from a rig/city name.
// Ported from gastown/internal/rig/manager.go:deriveBeadsPrefix.
//
// Algorithm:
//  1. Strip -py, -go suffixes
//  2. Split on - or _
//  3. If single word, try splitting compound word (camelCase, etc.)
//  4. If 2+ parts: first letter of each part
//  5. If 1 part and ≤3 chars: use as-is
//  6. If 1 part and >3 chars: first 2 chars
func DeriveBeadsPrefix(name string) string {
	name = strings.TrimSuffix(name, "-py")
	name = strings.TrimSuffix(name, "-go")

	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	if len(parts) == 1 {
		parts = splitCompoundWord(parts[0])
	}

	if len(parts) >= 2 {
		var prefix strings.Builder
		for _, p := range parts {
			if len(p) > 0 {
				prefix.WriteByte(p[0])
			}
		}
		return strings.ToLower(prefix.String())
	}

	if len(name) <= 3 {
		return strings.ToLower(name)
	}
	return strings.ToLower(name[:2])
}

// splitCompoundWord splits a camelCase or PascalCase word into parts.
// e.g. "myFrontend" → ["my", "Frontend"], "GasCity" → ["Gas", "City"]
func splitCompoundWord(word string) []string {
	if word == "" {
		return []string{word}
	}
	var parts []string
	start := 0
	runes := []rune(word)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	parts = append(parts, string(runes[start:]))
	if len(parts) <= 1 {
		return []string{word}
	}
	return parts
}

// Workspace holds city-level metadata and optional defaults that apply
// to all agents unless overridden per-agent.
type Workspace struct {
	Name         string `toml:"name"`
	Provider     string `toml:"provider,omitempty"`
	StartCommand string `toml:"start_command,omitempty"`
}

// BeadsConfig holds bead store settings.
type BeadsConfig struct {
	Provider string `toml:"provider,omitempty"` // "bd" (default) or "file"
}

// DoltConfig holds optional dolt server overrides.
// When present in city.toml, these override the defaults.
type DoltConfig struct {
	Port int    `toml:"port,omitempty"` // default 3307
	Host string `toml:"host,omitempty"` // default localhost
}

// FormulasConfig holds formula directory settings.
type FormulasConfig struct {
	Dir string `toml:"dir,omitempty"`
}

// DaemonConfig holds controller daemon settings.
type DaemonConfig struct {
	PatrolInterval string `toml:"patrol_interval,omitempty"`
}

// PatrolIntervalDuration returns the patrol interval as a time.Duration.
// Defaults to 30s if empty or unparseable.
func (d *DaemonConfig) PatrolIntervalDuration() time.Duration {
	if d.PatrolInterval == "" {
		return 30 * time.Second
	}
	dur, err := time.ParseDuration(d.PatrolInterval)
	if err != nil {
		return 30 * time.Second
	}
	return dur
}

// FormulasDir returns the formulas directory, defaulting to ".gc/formulas".
func (c *City) FormulasDir() string {
	if c.Formulas.Dir != "" {
		return c.Formulas.Dir
	}
	return ".gc/formulas"
}

// PoolConfig defines elastic pool parameters for an agent. When present
// on an Agent, that agent becomes a pool with scaling behavior.
type PoolConfig struct {
	Min          int    `toml:"min,omitempty"`
	Max          int    `toml:"max,omitempty"`
	Check        string `toml:"check,omitempty"`
	DrainTimeout string `toml:"drain_timeout,omitempty"`
}

// DrainTimeoutDuration returns the drain timeout as a time.Duration.
// Defaults to 5m if empty or unparseable.
func (p *PoolConfig) DrainTimeoutDuration() time.Duration {
	if p.DrainTimeout == "" {
		return 5 * time.Minute
	}
	dur, err := time.ParseDuration(p.DrainTimeout)
	if err != nil {
		return 5 * time.Minute
	}
	return dur
}

// Agent defines a configured agent in the city.
type Agent struct {
	Name           string `toml:"name"`
	Dir            string `toml:"dir,omitempty"`
	PromptTemplate string `toml:"prompt_template,omitempty"`
	Provider       string `toml:"provider,omitempty"`
	StartCommand   string `toml:"start_command,omitempty"`
	// Provider field overrides (sparse — only set fields override the provider).
	Args                   []string          `toml:"args,omitempty"`
	PromptMode             string            `toml:"prompt_mode,omitempty"`
	PromptFlag             string            `toml:"prompt_flag,omitempty"`
	ReadyDelayMs           *int              `toml:"ready_delay_ms,omitempty"`
	ReadyPromptPrefix      string            `toml:"ready_prompt_prefix,omitempty"`
	ProcessNames           []string          `toml:"process_names,omitempty"`
	EmitsPermissionWarning *bool             `toml:"emits_permission_warning,omitempty"`
	Env                    map[string]string `toml:"env,omitempty"`
	Pool                   *PoolConfig       `toml:"pool,omitempty"`
}

// EffectivePool returns the pool configuration for this agent, applying
// defaults. If Pool is nil, returns an always-on singleton (min=1, max=1,
// check="echo 1"). If Pool is set, defaults Check to "echo 1" if empty.
func (a *Agent) EffectivePool() PoolConfig {
	if a.Pool == nil {
		return PoolConfig{Min: 1, Max: 1, Check: "echo 1"}
	}
	p := *a.Pool
	if p.Check == "" {
		p.Check = "echo 1"
	}
	return p
}

// IsPool reports whether this agent has explicit pool configuration.
func (a *Agent) IsPool() bool {
	return a.Pool != nil
}

// ValidateAgents checks agent configurations for errors. It returns an error
// if any agent is missing required fields, has duplicate names, or has invalid
// pool bounds.
func ValidateAgents(agents []Agent) error {
	seen := make(map[string]bool, len(agents))
	for i, a := range agents {
		if a.Name == "" {
			return fmt.Errorf("agent[%d]: name is required", i)
		}
		if seen[a.Name] {
			return fmt.Errorf("agent %q: duplicate name", a.Name)
		}
		seen[a.Name] = true
		if a.Pool != nil {
			if a.Pool.Min < 0 {
				return fmt.Errorf("agent %q: pool min must be >= 0", a.Name)
			}
			if a.Pool.Max < 0 {
				return fmt.Errorf("agent %q: pool max must be >= 0", a.Name)
			}
			if a.Pool.Min > a.Pool.Max {
				return fmt.Errorf("agent %q: pool min (%d) must be <= max (%d)", a.Name, a.Pool.Min, a.Pool.Max)
			}
		}
	}
	return nil
}

// ValidateRigs checks rig configurations for errors. It returns an error if
// any rig is missing required fields, has duplicate names, or has colliding
// prefixes. The cityName is used to derive the HQ prefix for collision checks.
func ValidateRigs(rigs []Rig, cityName string) error {
	seenNames := make(map[string]bool, len(rigs))
	seenPrefixes := make(map[string]string) // prefix → rig name (for error messages)

	// HQ prefix participates in collision detection.
	hqPrefix := DeriveBeadsPrefix(cityName)
	seenPrefixes[hqPrefix] = cityName + " (HQ)"

	for i, r := range rigs {
		if r.Name == "" {
			return fmt.Errorf("rig[%d]: name is required", i)
		}
		if r.Path == "" {
			return fmt.Errorf("rig %q: path is required", r.Name)
		}
		if seenNames[r.Name] {
			return fmt.Errorf("rig %q: duplicate name", r.Name)
		}
		seenNames[r.Name] = true

		prefix := r.EffectivePrefix()
		if other, ok := seenPrefixes[prefix]; ok {
			return fmt.Errorf("rig %q: prefix %q collides with %s", r.Name, prefix, other)
		}
		seenPrefixes[prefix] = r.Name
	}
	return nil
}

// DefaultCity returns a City with the given name and a single default
// agent named "mayor". This is the config written by "gc init".
func DefaultCity(name string) City {
	return City{
		Workspace: Workspace{Name: name},
		Agents:    []Agent{{Name: "mayor", PromptTemplate: "prompts/mayor.md"}},
	}
}

// WizardCity returns a City with the given name, a workspace-level provider
// or start command, and two agents (mayor + worker). This is the config
// written by "gc init" when the interactive wizard runs. If startCommand is
// set, it takes precedence over provider.
func WizardCity(name, provider, startCommand string) City {
	ws := Workspace{Name: name}
	if startCommand != "" {
		ws.StartCommand = startCommand
	} else {
		ws.Provider = provider
	}
	return City{
		Workspace: ws,
		Agents: []Agent{
			{Name: "mayor", PromptTemplate: "prompts/mayor.md"},
			{Name: "worker", PromptTemplate: "prompts/worker.md"},
		},
	}
}

// Marshal encodes a City to TOML bytes.
func (c *City) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	return buf.Bytes(), nil
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
