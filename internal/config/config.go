// Package config handles loading and parsing city.toml configuration files.
package config

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/fsys"
)

// City is the top-level configuration for a Gas City instance.
type City struct {
	Workspace Workspace               `toml:"workspace"`
	Providers map[string]ProviderSpec `toml:"providers,omitempty"`
	Agents    []Agent                 `toml:"agents"`
	Pools     []Pool                  `toml:"pools,omitempty"`
	Beads     BeadsConfig             `toml:"beads,omitempty"`
	Dolt      DoltConfig              `toml:"dolt,omitempty"`
	Formulas  FormulasConfig          `toml:"formulas,omitempty"`
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

// FormulasDir returns the formulas directory, defaulting to ".gc/formulas".
func (c *City) FormulasDir() string {
	if c.Formulas.Dir != "" {
		return c.Formulas.Dir
	}
	return ".gc/formulas"
}

// Agent defines a configured agent in the city.
type Agent struct {
	Name           string `toml:"name"`
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
	Suspended              *bool             `toml:"suspended,omitempty"`
}

// IsSuspended reports whether this agent is suspended.
func (a *Agent) IsSuspended() bool {
	return a.Suspended != nil && *a.Suspended
}

// Pool defines an elastic agent pool. Agents are started on demand based
// on the scale_check command's output, clamped to [Min, Max].
type Pool struct {
	Name                   string            `toml:"name"`
	Provider               string            `toml:"provider,omitempty"`
	PromptTemplate         string            `toml:"prompt_template,omitempty"`
	StartCommand           string            `toml:"start_command,omitempty"`
	Args                   []string          `toml:"args,omitempty"`
	PromptMode             string            `toml:"prompt_mode,omitempty"`
	PromptFlag             string            `toml:"prompt_flag,omitempty"`
	ReadyDelayMs           *int              `toml:"ready_delay_ms,omitempty"`
	ReadyPromptPrefix      string            `toml:"ready_prompt_prefix,omitempty"`
	ProcessNames           []string          `toml:"process_names,omitempty"`
	EmitsPermissionWarning *bool             `toml:"emits_permission_warning,omitempty"`
	Env                    map[string]string `toml:"env,omitempty"`
	Hook                   string            `toml:"hook,omitempty"`
	Hints                  string            `toml:"hints,omitempty"`
	Min                    int               `toml:"min"`
	Max                    int               `toml:"max"`
	ScaleCheck             string            `toml:"scale_check"`
}

// ToAgent converts a Pool template into an Agent with the given name.
// Copies all shared provider/startup fields from the pool.
func (p *Pool) ToAgent(name string) Agent {
	a := Agent{
		Name:                   name,
		Provider:               p.Provider,
		PromptTemplate:         p.PromptTemplate,
		StartCommand:           p.StartCommand,
		PromptMode:             p.PromptMode,
		PromptFlag:             p.PromptFlag,
		ReadyDelayMs:           p.ReadyDelayMs,
		ReadyPromptPrefix:      p.ReadyPromptPrefix,
		EmitsPermissionWarning: p.EmitsPermissionWarning,
	}
	if len(p.Args) > 0 {
		a.Args = make([]string, len(p.Args))
		copy(a.Args, p.Args)
	}
	if len(p.ProcessNames) > 0 {
		a.ProcessNames = make([]string, len(p.ProcessNames))
		copy(a.ProcessNames, p.ProcessNames)
	}
	if len(p.Env) > 0 {
		a.Env = make(map[string]string, len(p.Env))
		for k, v := range p.Env {
			a.Env[k] = v
		}
	}
	return a
}

// ValidatePools checks pool configurations for errors. It returns an error
// if any pool is missing required fields or has invalid bounds.
func ValidatePools(pools []Pool) error {
	seen := make(map[string]bool, len(pools))
	for i, p := range pools {
		if p.Name == "" {
			return fmt.Errorf("pool[%d]: name is required", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("pool %q: duplicate name", p.Name)
		}
		seen[p.Name] = true
		if p.Min < 0 {
			return fmt.Errorf("pool %q: min must be >= 0", p.Name)
		}
		if p.Max < 0 {
			return fmt.Errorf("pool %q: max must be >= 0", p.Name)
		}
		if p.Min > p.Max {
			return fmt.Errorf("pool %q: min (%d) must be <= max (%d)", p.Name, p.Min, p.Max)
		}
		if p.ScaleCheck == "" {
			return fmt.Errorf("pool %q: scale_check is required", p.Name)
		}
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
