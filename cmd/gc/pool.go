package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

// ScaleCheckRunner runs a scale_check command and returns stdout.
type ScaleCheckRunner func(command string) (string, error)

// shellScaleCheck runs a scale_check command via sh -c and returns stdout.
func shellScaleCheck(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		return "", fmt.Errorf("running scale_check %q: %w", command, err)
	}
	return string(out), nil
}

// evaluatePool runs check, parses the output as an integer, and clamps
// the result to [min, max]. Returns min on error (honors configured minimum).
func evaluatePool(agentName string, pool config.PoolConfig, runner ScaleCheckRunner) (int, error) {
	out, err := runner(pool.Check)
	if err != nil {
		return pool.Min, fmt.Errorf("agent %q: %w", agentName, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return pool.Min, fmt.Errorf("agent %q: check %q produced empty output", agentName, pool.Check)
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return pool.Min, fmt.Errorf("agent %q: check output %q is not an integer", agentName, trimmed)
	}
	if n < pool.Min {
		return pool.Min, nil
	}
	if n > pool.Max {
		return pool.Max, nil
	}
	return n, nil
}

// poolAgents builds agent.Agent instances for a pool at the desired count.
// If pool.Max == 1, uses the bare agent name (no suffix).
// If pool.Max > 1, names follow the pattern {name}-{n} (1-indexed).
// Sessions follow the pattern gc-{city}-{name} or gc-{city}-{name}-{n}.
func poolAgents(cfgAgent *config.Agent, desired int, cityName, cityPath string,
	ws *config.Workspace, providers map[string]config.ProviderSpec,
	lookPath config.LookPathFunc, fs fsys.FS, sp session.Provider,
	rigs []config.Rig,
) ([]agent.Agent, error) {
	if desired <= 0 {
		return nil, nil
	}

	pool := cfgAgent.EffectivePool()

	workDir, err := resolveAgentDir(cityPath, cfgAgent.Dir)
	if err != nil {
		return nil, fmt.Errorf("agent %q: %w", cfgAgent.Name, err)
	}

	var agents []agent.Agent
	for i := 1; i <= desired; i++ {
		// If max == 1, use bare name (no suffix).
		// If max > 1, use {name}-{N} suffix.
		name := cfgAgent.Name
		if pool.Max > 1 {
			name = fmt.Sprintf("%s-%d", cfgAgent.Name, i)
		}

		// Deep-copy the agent config for instance resolution.
		instanceAgent := config.Agent{
			Name:                   name,
			Dir:                    cfgAgent.Dir,
			Isolation:              cfgAgent.Isolation,
			Provider:               cfgAgent.Provider,
			PromptTemplate:         cfgAgent.PromptTemplate,
			StartCommand:           cfgAgent.StartCommand,
			PromptMode:             cfgAgent.PromptMode,
			PromptFlag:             cfgAgent.PromptFlag,
			ReadyDelayMs:           cfgAgent.ReadyDelayMs,
			ReadyPromptPrefix:      cfgAgent.ReadyPromptPrefix,
			EmitsPermissionWarning: cfgAgent.EmitsPermissionWarning,
		}
		if len(cfgAgent.Args) > 0 {
			instanceAgent.Args = make([]string, len(cfgAgent.Args))
			copy(instanceAgent.Args, cfgAgent.Args)
		}
		if len(cfgAgent.ProcessNames) > 0 {
			instanceAgent.ProcessNames = make([]string, len(cfgAgent.ProcessNames))
			copy(instanceAgent.ProcessNames, cfgAgent.ProcessNames)
		}
		if len(cfgAgent.Env) > 0 {
			instanceAgent.Env = make(map[string]string, len(cfgAgent.Env))
			for k, v := range cfgAgent.Env {
				instanceAgent.Env[k] = v
			}
		}

		resolved, err := config.ResolveProvider(&instanceAgent, ws, providers, lookPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q instance %q: %w", cfgAgent.Name, name, err)
		}

		// Worktree isolation: create per-instance worktree from rig repo.
		instanceWorkDir := workDir
		agentEnv := map[string]string{
			"GC_AGENT": name,
			"GC_CITY":  cityPath,
			"GC_DIR":   workDir,
		}
		if cfgAgent.Isolation == "worktree" {
			rn, rp, found := findRigByDir(workDir, rigs)
			if found {
				wt, br, wtErr := createAgentWorktree(rp, cityPath, rn, name)
				if wtErr != nil {
					return nil, fmt.Errorf("agent %q instance %q: %w", cfgAgent.Name, name, wtErr)
				}
				if rdErr := setupBeadsRedirect(wt, rp); rdErr != nil {
					return nil, fmt.Errorf("agent %q instance %q: %w", cfgAgent.Name, name, rdErr)
				}
				instanceWorkDir = wt
				agentEnv["GC_DIR"] = wt
				agentEnv["GC_RIG"] = rn
				agentEnv["GC_BRANCH"] = br
			}
		}

		appendClaudeSettings(resolved, cityPath)
		command := resolved.CommandString()
		prompt := readPromptFile(fs, cityPath, cfgAgent.PromptTemplate)
		env := mergeEnv(passthroughEnv(), resolved.Env, cfgAgent.Env, agentEnv)
		hints := agent.StartupHints{
			ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
			ReadyDelayMs:           resolved.ReadyDelayMs,
			ProcessNames:           resolved.ProcessNames,
			EmitsPermissionWarning: resolved.EmitsPermissionWarning,
		}
		agents = append(agents, agent.New(name, cityName, command, prompt, env, hints, instanceWorkDir, sp))
	}
	return agents, nil
}
