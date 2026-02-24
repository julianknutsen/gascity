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

// evaluatePool runs scale_check, parses the output as an integer, and clamps
// the result to [min, max]. Returns min on error (honors configured minimum).
func evaluatePool(pool *config.Pool, runner ScaleCheckRunner) (int, error) {
	out, err := runner(pool.ScaleCheck)
	if err != nil {
		return pool.Min, fmt.Errorf("pool %q: %w", pool.Name, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return pool.Min, fmt.Errorf("pool %q: scale_check output %q is not an integer", pool.Name, strings.TrimSpace(out))
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
// Names follow the pattern {pool}-{n} (1-indexed).
// Sessions follow the pattern gc-{city}-{pool}-{n}.
func poolAgents(pool *config.Pool, desired int, cityName, cityPath string,
	ws *config.Workspace, providers map[string]config.ProviderSpec,
	lookPath config.LookPathFunc, fs fsys.FS, sp session.Provider,
) ([]agent.Agent, error) {
	if desired <= 0 {
		return nil, nil
	}

	var agents []agent.Agent
	for i := 1; i <= desired; i++ {
		name := fmt.Sprintf("%s-%d", pool.Name, i)
		cfgAgent := pool.ToAgent(name)

		resolved, err := config.ResolveProvider(&cfgAgent, ws, providers, lookPath)
		if err != nil {
			return nil, fmt.Errorf("pool %q agent %q: %w", pool.Name, name, err)
		}

		command := resolved.CommandString()
		sn := sessionName(cityName, name)
		prompt := readPromptFile(fs, cityPath, pool.PromptTemplate)
		env := mergeEnv(passthroughEnv(), resolved.Env, pool.Env, map[string]string{
			"GC_AGENT": name,
			"GC_CITY":  cityPath,
		})
		hints := agent.StartupHints{
			ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
			ReadyDelayMs:           resolved.ReadyDelayMs,
			ProcessNames:           resolved.ProcessNames,
			EmitsPermissionWarning: resolved.EmitsPermissionWarning,
		}
		agents = append(agents, agent.New(name, sn, command, prompt, env, hints, sp))
	}
	return agents, nil
}
