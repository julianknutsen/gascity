package config

import "fmt"

// Patches holds all patch blocks from composition. Patches target existing
// resources by identity key and modify specific fields. They are applied
// after fragment merge, before validation.
type Patches struct {
	// Agents targets agents by (dir, name).
	Agents []AgentPatch `toml:"agents,omitempty"`
	// Rigs targets rigs by name.
	Rigs []RigPatch `toml:"rigs,omitempty"`
	// Providers targets providers by name.
	Providers []ProviderPatch `toml:"providers,omitempty"`
}

// AgentPatch modifies an existing agent identified by (Dir, Name).
// Pointer fields distinguish "not set" from "set to zero value."
type AgentPatch struct {
	// Dir is the targeting key (required with Name).
	Dir string `toml:"dir"`
	// Name is the targeting key (required).
	Name string `toml:"name"`
	// Suspended overrides the agent's suspended state.
	Suspended *bool `toml:"suspended,omitempty"`
	// Pool overrides pool configuration fields.
	Pool *PoolOverride `toml:"pool,omitempty"`
	// Env adds or overrides environment variables.
	Env map[string]string `toml:"env,omitempty"`
	// EnvRemove lists env var keys to remove after merging.
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

// PoolOverride modifies pool configuration fields. Nil fields are not changed.
type PoolOverride struct {
	// Min overrides pool minimum instances.
	Min *int `toml:"min,omitempty"`
	// Max overrides pool maximum instances.
	Max *int `toml:"max,omitempty"`
	// Check overrides the pool check command.
	Check *string `toml:"check,omitempty"`
	// DrainTimeout overrides the drain timeout.
	DrainTimeout *string `toml:"drain_timeout,omitempty"`
}

// RigPatch modifies an existing rig identified by Name.
type RigPatch struct {
	// Name is the targeting key (required).
	Name string `toml:"name"`
	// Path overrides the rig's filesystem path.
	Path *string `toml:"path,omitempty"`
	// Prefix overrides the bead ID prefix.
	Prefix *string `toml:"prefix,omitempty"`
	// Suspended overrides the rig's suspended state.
	Suspended *bool `toml:"suspended,omitempty"`
}

// ProviderPatch modifies an existing provider identified by Name.
type ProviderPatch struct {
	// Name is the targeting key (required).
	Name string `toml:"name"`
	// Command overrides the provider command.
	Command *string `toml:"command,omitempty"`
	// Args overrides the provider args.
	Args []string `toml:"args,omitempty"`
	// PromptMode overrides prompt delivery mode.
	PromptMode *string `toml:"prompt_mode,omitempty"`
	// PromptFlag overrides the prompt flag.
	PromptFlag *string `toml:"prompt_flag,omitempty"`
	// ReadyDelayMs overrides the ready delay.
	ReadyDelayMs *int `toml:"ready_delay_ms,omitempty"`
	// Env adds or overrides environment variables.
	Env map[string]string `toml:"env,omitempty"`
	// EnvRemove lists env var keys to remove.
	EnvRemove []string `toml:"env_remove,omitempty"`
	// Replace replaces the entire provider block instead of deep-merging.
	Replace bool `toml:"_replace,omitempty"`
}

// IsEmpty reports whether p has no patch operations.
func (p *Patches) IsEmpty() bool {
	return len(p.Agents) == 0 && len(p.Rigs) == 0 && len(p.Providers) == 0
}

// ApplyPatches applies all patches to the config. Patches target existing
// resources by identity key. If a patch targets a nonexistent resource,
// an error is returned. Patches are intentional — they never generate
// collision warnings.
func ApplyPatches(cfg *City, patches Patches) error {
	for i, p := range patches.Agents {
		if err := applyAgentPatch(cfg, &p); err != nil {
			return fmt.Errorf("patches.agents[%d]: %w", i, err)
		}
	}
	for i, p := range patches.Rigs {
		if err := applyRigPatch(cfg, &p); err != nil {
			return fmt.Errorf("patches.rigs[%d]: %w", i, err)
		}
	}
	for i, p := range patches.Providers {
		if err := applyProviderPatch(cfg, &p); err != nil {
			return fmt.Errorf("patches.providers[%d]: %w", i, err)
		}
	}
	return nil
}

// applyAgentPatch finds an agent by (dir, name) and applies the patch.
func applyAgentPatch(cfg *City, patch *AgentPatch) error {
	if patch.Name == "" {
		return fmt.Errorf("agent patch: name is required")
	}
	target := qualifiedNameFromPatch(patch.Dir, patch.Name)
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		if a.Dir == patch.Dir && a.Name == patch.Name {
			applyAgentPatchFields(a, patch)
			return nil
		}
	}
	return fmt.Errorf("agent %q not found in merged config", target)
}

func applyAgentPatchFields(a *Agent, p *AgentPatch) {
	if p.Suspended != nil {
		a.Suspended = *p.Suspended
	}
	if p.Isolation != nil {
		a.Isolation = *p.Isolation
	}
	if p.PromptTemplate != nil {
		a.PromptTemplate = *p.PromptTemplate
	}
	if p.Provider != nil {
		a.Provider = *p.Provider
	}
	if p.StartCommand != nil {
		a.StartCommand = *p.StartCommand
	}
	if p.Nudge != nil {
		a.Nudge = *p.Nudge
	}
	if p.IdleTimeout != nil {
		a.IdleTimeout = *p.IdleTimeout
	}
	if len(p.InstallAgentHooks) > 0 {
		a.InstallAgentHooks = append([]string(nil), p.InstallAgentHooks...)
	}
	if p.HooksInstalled != nil {
		a.HooksInstalled = p.HooksInstalled
	}
	// Env: additive merge.
	if len(p.Env) > 0 {
		if a.Env == nil {
			a.Env = make(map[string]string, len(p.Env))
		}
		for k, v := range p.Env {
			a.Env[k] = v
		}
	}
	// EnvRemove: remove keys after merge.
	for _, k := range p.EnvRemove {
		delete(a.Env, k)
	}
	// Pool: sub-field patching.
	if p.Pool != nil {
		applyPoolOverride(a, p.Pool)
	}
}

// applyPoolOverride applies pool sub-field patches. Creates the PoolConfig
// if the agent doesn't have one yet.
func applyPoolOverride(a *Agent, po *PoolOverride) {
	if a.Pool == nil {
		a.Pool = &PoolConfig{}
	}
	if po.Min != nil {
		a.Pool.Min = *po.Min
	}
	if po.Max != nil {
		a.Pool.Max = *po.Max
	}
	if po.Check != nil {
		a.Pool.Check = *po.Check
	}
	if po.DrainTimeout != nil {
		a.Pool.DrainTimeout = *po.DrainTimeout
	}
}

// applyRigPatch finds a rig by name and applies the patch.
func applyRigPatch(cfg *City, patch *RigPatch) error {
	if patch.Name == "" {
		return fmt.Errorf("rig patch: name is required")
	}
	for i := range cfg.Rigs {
		r := &cfg.Rigs[i]
		if r.Name == patch.Name {
			if patch.Path != nil {
				r.Path = *patch.Path
			}
			if patch.Prefix != nil {
				r.Prefix = *patch.Prefix
			}
			if patch.Suspended != nil {
				r.Suspended = *patch.Suspended
			}
			return nil
		}
	}
	return fmt.Errorf("rig %q not found in merged config", patch.Name)
}

// applyProviderPatch modifies a provider. If Replace is true, replaces the
// entire block. Otherwise deep-merges per-field.
func applyProviderPatch(cfg *City, patch *ProviderPatch) error {
	if patch.Name == "" {
		return fmt.Errorf("provider patch: name is required")
	}
	if cfg.Providers == nil {
		return fmt.Errorf("provider %q not found in merged config", patch.Name)
	}
	spec, ok := cfg.Providers[patch.Name]
	if !ok {
		return fmt.Errorf("provider %q not found in merged config", patch.Name)
	}
	if patch.Replace {
		// Full replacement — build a new spec from patch fields only.
		var newSpec ProviderSpec
		if patch.Command != nil {
			newSpec.Command = *patch.Command
		}
		if len(patch.Args) > 0 {
			newSpec.Args = make([]string, len(patch.Args))
			copy(newSpec.Args, patch.Args)
		}
		if patch.PromptMode != nil {
			newSpec.PromptMode = *patch.PromptMode
		}
		if patch.PromptFlag != nil {
			newSpec.PromptFlag = *patch.PromptFlag
		}
		if patch.ReadyDelayMs != nil {
			newSpec.ReadyDelayMs = *patch.ReadyDelayMs
		}
		if len(patch.Env) > 0 {
			newSpec.Env = make(map[string]string, len(patch.Env))
			for k, v := range patch.Env {
				newSpec.Env[k] = v
			}
		}
		cfg.Providers[patch.Name] = newSpec
		return nil
	}
	// Deep merge: only set fields override.
	if patch.Command != nil {
		spec.Command = *patch.Command
	}
	if len(patch.Args) > 0 {
		spec.Args = make([]string, len(patch.Args))
		copy(spec.Args, patch.Args)
	}
	if patch.PromptMode != nil {
		spec.PromptMode = *patch.PromptMode
	}
	if patch.PromptFlag != nil {
		spec.PromptFlag = *patch.PromptFlag
	}
	if patch.ReadyDelayMs != nil {
		spec.ReadyDelayMs = *patch.ReadyDelayMs
	}
	// Env: additive merge.
	if len(patch.Env) > 0 {
		if spec.Env == nil {
			spec.Env = make(map[string]string, len(patch.Env))
		}
		for k, v := range patch.Env {
			spec.Env[k] = v
		}
	}
	for _, k := range patch.EnvRemove {
		delete(spec.Env, k)
	}
	cfg.Providers[patch.Name] = spec
	return nil
}

func qualifiedNameFromPatch(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
