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

// QualifiedName returns the agent's canonical identity.
// Rig-scoped: "hello-world/polecat". City-wide: "mayor".
func (a *Agent) QualifiedName() string {
	if a.Dir == "" {
		return a.Name
	}
	return a.Dir + "/" + a.Name
}

// ParseQualifiedName splits an agent identity into (dir, name).
// "hello-world/polecat" → ("hello-world", "polecat").
// "mayor" → ("", "mayor").
func ParseQualifiedName(identity string) (dir, name string) {
	if i := strings.LastIndex(identity, "/"); i >= 0 {
		return identity[:i], identity[i+1:]
	}
	return "", identity
}

// City is the top-level configuration for a Gas City instance.
// Parsed from city.toml at the root of a city directory.
type City struct {
	// Include lists config fragment files to merge into this config.
	// Processed by LoadWithIncludes; not recursive (fragments cannot include).
	Include []string `toml:"include,omitempty"`
	// Workspace holds city-level metadata (name, default provider).
	Workspace Workspace `toml:"workspace"`
	// Providers defines named provider presets for agent startup.
	Providers map[string]ProviderSpec `toml:"providers,omitempty"`
	// Topologies defines named remote topology sources fetched via git.
	Topologies map[string]TopologySource `toml:"topologies,omitempty"`
	// Agents lists all configured agents in this city.
	Agents []Agent `toml:"agents"`
	// Rigs lists external projects registered in the city.
	Rigs []Rig `toml:"rigs,omitempty"`
	// Patches holds targeted modifications applied after fragment merge.
	Patches Patches `toml:"patches,omitempty"`
	// Beads configures the bead store backend.
	Beads BeadsConfig `toml:"beads,omitempty"`
	// Dolt configures optional dolt server connection overrides.
	Dolt DoltConfig `toml:"dolt,omitempty"`
	// Formulas configures formula directory settings.
	Formulas FormulasConfig `toml:"formulas,omitempty"`
	// Daemon configures controller daemon settings.
	Daemon DaemonConfig `toml:"daemon,omitempty"`
}

// Rig defines an external project registered in the city.
type Rig struct {
	// Name is the unique identifier for this rig.
	Name string `toml:"name" jsonschema:"required"`
	// Path is the absolute filesystem path to the rig's repository.
	Path string `toml:"path" jsonschema:"required"`
	// Prefix overrides the auto-derived bead ID prefix for this rig.
	Prefix string `toml:"prefix,omitempty"`
	// Suspended prevents the reconciler from spawning agents in this rig. Toggle with gc rig suspend/resume.
	Suspended bool `toml:"suspended,omitempty"`
	// Topology is the path to a topology directory to stamp agents from.
	// Relative paths resolve against the declaring file's directory.
	Topology string `toml:"topology,omitempty"`
	// Overrides are per-agent patches applied after topology expansion.
	Overrides []AgentOverride `toml:"overrides,omitempty"`
}

// AgentOverride modifies a topology-stamped agent for a specific rig.
// Uses pointer fields to distinguish "not set" from "set to zero value."
type AgentOverride struct {
	// Agent is the name of the topology agent to override (required).
	Agent string `toml:"agent" jsonschema:"required"`
	// Dir overrides the stamped dir (default: rig name).
	Dir *string `toml:"dir,omitempty"`
	// Suspended sets the agent's suspended state.
	Suspended *bool `toml:"suspended,omitempty"`
	// Pool overrides pool configuration fields.
	Pool *PoolOverride `toml:"pool,omitempty"`
	// Env adds or overrides environment variables.
	Env map[string]string `toml:"env,omitempty"`
	// EnvRemove lists env var keys to remove.
	EnvRemove []string `toml:"env_remove,omitempty"`
	// Isolation overrides the isolation mode.
	Isolation *string `toml:"isolation,omitempty"`
	// PromptTemplate overrides the prompt template path.
	PromptTemplate *string `toml:"prompt_template,omitempty"`
	// Provider overrides the provider name.
	Provider *string `toml:"provider,omitempty"`
	// StartCommand overrides the start command.
	StartCommand *string `toml:"start_command,omitempty"`
	// Nudge overrides the nudge text.
	Nudge *string `toml:"nudge,omitempty"`
	// IdleTimeout overrides the idle timeout duration.
	IdleTimeout *string `toml:"idle_timeout,omitempty"`
	// InstallAgentHooks overrides the agent's install_agent_hooks list.
	InstallAgentHooks []string `toml:"install_agent_hooks,omitempty"`
	// HooksInstalled overrides automatic hook detection.
	HooksInstalled *bool `toml:"hooks_installed,omitempty"`
}

// TopologySource defines a remote topology repository.
// Referenced by name in rig topology fields and fetched into the cache.
type TopologySource struct {
	// Source is the git repository URL.
	Source string `toml:"source" jsonschema:"required"`
	// Ref is the git ref to checkout (branch, tag, or commit). Defaults to HEAD.
	Ref string `toml:"ref,omitempty"`
	// Path is a subdirectory within the repo containing the topology files.
	Path string `toml:"path,omitempty"`
}

// TopologyMeta holds metadata from a topology's [topology] header.
type TopologyMeta struct {
	// Name is the topology's identifier.
	Name string `toml:"name"`
	// Version is a semver-style version string.
	Version string `toml:"version"`
	// Schema is the topology format version (currently 1).
	Schema int `toml:"schema"`
	// RequiresGC is an optional minimum gc version requirement.
	RequiresGC string `toml:"requires_gc,omitempty"`
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
	// Name is the human-readable name for this city.
	Name string `toml:"name" jsonschema:"required"`
	// Provider is the default provider name used by agents that don't specify one.
	Provider string `toml:"provider,omitempty"`
	// StartCommand overrides the provider's command for all agents.
	StartCommand string `toml:"start_command,omitempty"`
	// SessionTemplate is a Go text/template string for session naming.
	// Available variables: .City, .Agent (sanitized), .Dir, .Name.
	// Default (empty): "gc-{{.City}}-{{.Agent}}".
	SessionTemplate string `toml:"session_template,omitempty"`
	// InstallAgentHooks lists provider names whose hooks should be installed
	// into agent working directories. Agent-level overrides workspace-level
	// (replace, not additive). Supported: "claude", "gemini", "opencode", "copilot".
	InstallAgentHooks []string `toml:"install_agent_hooks,omitempty"`
}

// BeadsConfig holds bead store settings.
type BeadsConfig struct {
	// Provider selects the bead store backend: "bd" (default) or "file".
	Provider string `toml:"provider,omitempty" jsonschema:"enum=bd,enum=file,default=bd"`
}

// DoltConfig holds optional dolt server overrides.
// When present in city.toml, these override the defaults.
type DoltConfig struct {
	// Port is the dolt server port. Defaults to 3307.
	Port int `toml:"port,omitempty" jsonschema:"default=3307"`
	// Host is the dolt server hostname. Defaults to localhost.
	Host string `toml:"host,omitempty" jsonschema:"default=localhost"`
}

// FormulasConfig holds formula directory settings.
type FormulasConfig struct {
	// Dir is the path to the formulas directory. Defaults to ".gc/formulas".
	Dir string `toml:"dir,omitempty" jsonschema:"default=.gc/formulas"`
}

// DaemonConfig holds controller daemon settings.
type DaemonConfig struct {
	// PatrolInterval is the health patrol interval as a Go duration string. Defaults to "30s".
	PatrolInterval string `toml:"patrol_interval,omitempty" jsonschema:"default=30s"`
	// MaxRestarts is the maximum number of agent restarts within RestartWindow before
	// the agent is quarantined. 0 means unlimited (no crash loop detection). Defaults to 5.
	MaxRestarts *int `toml:"max_restarts,omitempty" jsonschema:"default=5"`
	// RestartWindow is the sliding time window for counting restarts, as a Go duration
	// string. Defaults to "1h".
	RestartWindow string `toml:"restart_window,omitempty" jsonschema:"default=1h"`
	// ShutdownTimeout is the time to wait after sending Ctrl-C before force-killing
	// agents during shutdown. Set to "0s" for immediate kill. Defaults to "5s".
	ShutdownTimeout string `toml:"shutdown_timeout,omitempty" jsonschema:"default=5s"`
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

// MaxRestartsOrDefault returns the max restarts threshold. Nil (unset) defaults
// to 5. Zero means unlimited (no crash loop detection).
func (d *DaemonConfig) MaxRestartsOrDefault() int {
	if d.MaxRestarts == nil {
		return 5
	}
	return *d.MaxRestarts
}

// RestartWindowDuration returns the restart window as a time.Duration.
// Defaults to 1h if empty or unparseable.
func (d *DaemonConfig) RestartWindowDuration() time.Duration {
	if d.RestartWindow == "" {
		return time.Hour
	}
	dur, err := time.ParseDuration(d.RestartWindow)
	if err != nil {
		return time.Hour
	}
	return dur
}

// ShutdownTimeoutDuration returns the shutdown timeout as a time.Duration.
// Defaults to 5s if empty or unparseable. Zero means immediate kill.
func (d *DaemonConfig) ShutdownTimeoutDuration() time.Duration {
	if d.ShutdownTimeout == "" {
		return 5 * time.Second
	}
	dur, err := time.ParseDuration(d.ShutdownTimeout)
	if err != nil {
		return 5 * time.Second
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
	// Min is the minimum number of pool instances. Defaults to 0.
	Min int `toml:"min,omitempty" jsonschema:"minimum=0"`
	// Max is the maximum number of pool instances. Defaults to 0.
	Max int `toml:"max,omitempty" jsonschema:"minimum=0"`
	// Check is a shell command whose output determines desired pool size. Defaults to "echo 1".
	Check string `toml:"check,omitempty" jsonschema:"default=echo 1"`
	// DrainTimeout is the maximum time to wait for a pool instance to drain. Defaults to "5m".
	DrainTimeout string `toml:"drain_timeout,omitempty" jsonschema:"default=5m"`
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
	// Name is the unique identifier for this agent.
	Name string `toml:"name" jsonschema:"required"`
	// Dir is the working directory for the agent session.
	Dir string `toml:"dir,omitempty"`
	// Suspended prevents the reconciler from spawning this agent. Toggle with gc agent suspend/resume.
	Suspended bool `toml:"suspended,omitempty"`
	// Isolation controls filesystem isolation: "none" (default) or "worktree".
	Isolation string `toml:"isolation,omitempty" jsonschema:"enum=none,enum=worktree,default=none"`
	// PromptTemplate is the path to this agent's prompt template file.
	PromptTemplate string `toml:"prompt_template,omitempty"`
	// Nudge is text typed into the agent's tmux session after startup.
	// Used for CLI agents that don't accept command-line prompts.
	Nudge string `toml:"nudge,omitempty"`
	// Provider names the provider preset to use for this agent.
	Provider string `toml:"provider,omitempty"`
	// StartCommand overrides the provider's command for this agent.
	StartCommand string `toml:"start_command,omitempty"`
	// Args overrides the provider's default arguments.
	Args []string `toml:"args,omitempty"`
	// PromptMode controls how prompts are delivered: "arg", "flag", or "none".
	PromptMode string `toml:"prompt_mode,omitempty" jsonschema:"enum=arg,enum=flag,enum=none,default=arg"`
	// PromptFlag is the CLI flag used to pass prompts when prompt_mode is "flag".
	PromptFlag string `toml:"prompt_flag,omitempty"`
	// ReadyDelayMs is milliseconds to wait after launch before considering the agent ready.
	ReadyDelayMs *int `toml:"ready_delay_ms,omitempty" jsonschema:"minimum=0"`
	// ReadyPromptPrefix is the string prefix that indicates the agent is ready for input.
	ReadyPromptPrefix string `toml:"ready_prompt_prefix,omitempty"`
	// ProcessNames lists process names to look for when checking if the agent is running.
	ProcessNames []string `toml:"process_names,omitempty"`
	// EmitsPermissionWarning indicates whether the agent emits permission prompts that should be suppressed.
	EmitsPermissionWarning *bool `toml:"emits_permission_warning,omitempty"`
	// Env sets additional environment variables for the agent process.
	Env map[string]string `toml:"env,omitempty"`
	// Pool configures elastic pool behavior. When set, the agent becomes a pool.
	Pool *PoolConfig `toml:"pool,omitempty"`
	// WorkQuery is the command to find available work for this agent.
	// Used by gc hook and available in prompt templates as {{ .WorkQuery }}.
	// Default: "bd ready --assignee=<agent-qualified-name>"
	WorkQuery string `toml:"work_query,omitempty"`
	// IdleTimeout is the maximum time an agent session can be inactive before
	// the controller kills and restarts it. Empty (default) disables idle
	// checking. Example: "15m", "1h".
	IdleTimeout string `toml:"idle_timeout,omitempty"`
	// InstallAgentHooks overrides workspace-level install_agent_hooks for this agent.
	// When set, replaces (not adds to) the workspace default.
	InstallAgentHooks []string `toml:"install_agent_hooks,omitempty"`
	// HooksInstalled overrides automatic hook detection. Set to true when hooks
	// are manually installed (e.g., merged into the project's own hook config)
	// and auto-installation via install_agent_hooks is not desired, but the agent
	// should still be treated as hook-enabled for startup behavior (no prime
	// instruction in beacon, no delayed nudge).
	HooksInstalled *bool `toml:"hooks_installed,omitempty"`
}

// IdleTimeoutDuration returns the idle timeout as a time.Duration.
// Returns 0 if empty or unparseable (disabled).
func (a *Agent) IdleTimeoutDuration() time.Duration {
	if a.IdleTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(a.IdleTimeout)
	if err != nil {
		return 0
	}
	return d
}

// EffectiveWorkQuery returns the work query command for this agent.
// If WorkQuery is set, returns it as-is. Otherwise returns the default
// "bd ready --assignee=<qualified-name>".
func (a *Agent) EffectiveWorkQuery() string {
	if a.WorkQuery != "" {
		return a.WorkQuery
	}
	return "bd ready --assignee=" + a.QualifiedName()
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
// if any agent is missing required fields, has duplicate identities, or has
// invalid pool bounds. Uniqueness is keyed on (dir, name) — the same name
// in different dirs is allowed.
func ValidateAgents(agents []Agent) error {
	type agentKey struct{ dir, name string }
	seen := make(map[agentKey]bool, len(agents))
	for i, a := range agents {
		if a.Name == "" {
			return fmt.Errorf("agent[%d]: name is required", i)
		}
		key := agentKey{dir: a.Dir, name: a.Name}
		if seen[key] {
			return fmt.Errorf("agent %q: duplicate name", a.QualifiedName())
		}
		seen[key] = true
		if a.Isolation != "" && a.Isolation != "none" && a.Isolation != "worktree" {
			return fmt.Errorf("agent %q: unknown isolation %q (must be \"none\" or \"worktree\")", a.Name, a.Isolation)
		}
		if a.Isolation == "worktree" && a.Dir == "" {
			return fmt.Errorf("agent %q: isolation \"worktree\" requires dir (target repo)", a.Name)
		}
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
// or start command, and one agent (mayor). This is the config written by
// "gc init" when the interactive wizard runs. If startCommand is set, it
// takes precedence over provider.
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
