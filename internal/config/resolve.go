package config

import (
	"fmt"
	"strings"
)

// LookPathFunc is the signature for exec.LookPath (or a test fake).
type LookPathFunc func(string) (string, error)

// ResolveProvider determines the fully-resolved provider for an agent.
//
// Resolution chain:
//  1. agent.StartCommand set? Escape hatch → ResolvedProvider{Command: startCommand}
//  2. Determine provider name: agent.Provider > workspace.Provider > auto-detect
//     (workspace.StartCommand is escape hatch if no provider name found)
//  3. Look up ProviderSpec: cityProviders[name] > BuiltinProviders()[name]
//     (verify binary exists in PATH via lookPath)
//  4. Merge agent-level overrides: non-zero agent fields replace base spec fields
//     (env merges additively — agent env adds to/overrides base env)
//  5. Default prompt_mode to "arg" if still empty
func ResolveProvider(agent *Agent, ws *Workspace, cityProviders map[string]ProviderSpec, lookPath LookPathFunc) (*ResolvedProvider, error) {
	// Step 1: agent.StartCommand is the escape hatch.
	if agent.StartCommand != "" {
		return &ResolvedProvider{Command: agent.StartCommand, PromptMode: "arg"}, nil
	}

	// Step 2: determine provider name.
	name := agent.Provider
	if name == "" && ws != nil {
		name = ws.Provider
	}
	if name == "" {
		// No provider name — check workspace start_command escape hatch.
		if ws != nil && ws.StartCommand != "" {
			return &ResolvedProvider{Command: ws.StartCommand, PromptMode: "arg"}, nil
		}
		// Auto-detect: scan PATH for known binaries.
		detected, err := detectProviderName(lookPath)
		if err != nil {
			return nil, err
		}
		name = detected
	}

	// Step 3: look up the ProviderSpec.
	spec, err := lookupProvider(name, cityProviders, lookPath)
	if err != nil {
		return nil, err
	}

	// Step 4: merge agent-level overrides.
	resolved := specToResolved(name, spec)
	mergeAgentOverrides(resolved, agent)

	// Step 5: default prompt_mode.
	if resolved.PromptMode == "" {
		resolved.PromptMode = "arg"
	}

	return resolved, nil
}

// lookupProvider finds a ProviderSpec by name, checking city-level providers
// first, then built-in presets. Verifies the binary exists in PATH.
func lookupProvider(name string, cityProviders map[string]ProviderSpec, lookPath LookPathFunc) (*ProviderSpec, error) {
	// City-level providers take precedence.
	if cityProviders != nil {
		if spec, ok := cityProviders[name]; ok {
			if spec.Command != "" {
				if _, err := lookPath(spec.Command); err != nil {
					return nil, fmt.Errorf("provider %q: command %q not found in PATH", name, spec.Command)
				}
			}
			return &spec, nil
		}
	}

	// Fall back to built-in presets.
	builtins := BuiltinProviders()
	if spec, ok := builtins[name]; ok {
		if _, err := lookPath(spec.Command); err != nil {
			return nil, fmt.Errorf("provider %q not found in PATH", name)
		}
		return &spec, nil
	}

	return nil, fmt.Errorf("unknown provider %q", name)
}

// detectProviderName scans PATH for known built-in provider binaries.
// Returns the first found in priority order (see BuiltinProviderOrder).
func detectProviderName(lookPath LookPathFunc) (string, error) {
	builtins := BuiltinProviders()
	order := BuiltinProviderOrder()
	for _, name := range order {
		if _, err := lookPath(builtins[name].Command); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no supported agent CLI found in PATH (looked for: %s)", strings.Join(order, ", "))
}

// specToResolved converts a ProviderSpec to a ResolvedProvider.
func specToResolved(name string, spec *ProviderSpec) *ResolvedProvider {
	rp := &ResolvedProvider{
		Name:                   name,
		Command:                spec.Command,
		PromptMode:             spec.PromptMode,
		PromptFlag:             spec.PromptFlag,
		ReadyDelayMs:           spec.ReadyDelayMs,
		ReadyPromptPrefix:      spec.ReadyPromptPrefix,
		EmitsPermissionWarning: spec.EmitsPermissionWarning,
	}
	// Copy slices to avoid aliasing.
	if len(spec.Args) > 0 {
		rp.Args = make([]string, len(spec.Args))
		copy(rp.Args, spec.Args)
	}
	if len(spec.ProcessNames) > 0 {
		rp.ProcessNames = make([]string, len(spec.ProcessNames))
		copy(rp.ProcessNames, spec.ProcessNames)
	}
	if len(spec.Env) > 0 {
		rp.Env = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			rp.Env[k] = v
		}
	}
	return rp
}

// mergeAgentOverrides applies non-zero agent-level fields on top of the
// resolved provider. Env merges additively (agent keys add to / override
// base keys). All other fields replace when set.
func mergeAgentOverrides(rp *ResolvedProvider, agent *Agent) {
	if len(agent.Args) > 0 {
		rp.Args = make([]string, len(agent.Args))
		copy(rp.Args, agent.Args)
	}
	if agent.PromptMode != "" {
		rp.PromptMode = agent.PromptMode
	}
	if agent.PromptFlag != "" {
		rp.PromptFlag = agent.PromptFlag
	}
	if agent.ReadyDelayMs != nil {
		rp.ReadyDelayMs = *agent.ReadyDelayMs
	}
	if agent.ReadyPromptPrefix != "" {
		rp.ReadyPromptPrefix = agent.ReadyPromptPrefix
	}
	if len(agent.ProcessNames) > 0 {
		rp.ProcessNames = make([]string, len(agent.ProcessNames))
		copy(rp.ProcessNames, agent.ProcessNames)
	}
	if agent.EmitsPermissionWarning != nil {
		rp.EmitsPermissionWarning = *agent.EmitsPermissionWarning
	}
	// Env merges additively.
	if len(agent.Env) > 0 {
		if rp.Env == nil {
			rp.Env = make(map[string]string, len(agent.Env))
		}
		for k, v := range agent.Env {
			rp.Env[k] = v
		}
	}
}
