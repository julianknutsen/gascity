package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/hooks"
	"github.com/steveyegge/gascity/internal/overlay"
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

// SessionSetupContext holds template variables for session_setup command expansion.
type SessionSetupContext struct {
	Session   string // tmux session name
	Agent     string // qualified agent name
	Rig       string // rig name (empty for city-scoped)
	CityRoot  string // city directory path
	CityName  string // workspace name
	WorkDir   string // agent working directory
	ConfigDir string // source directory where agent config was defined
}

// expandSessionSetup expands Go text/template strings in session_setup commands.
// On parse or execute error, the raw command is kept (graceful fallback).
func expandSessionSetup(cmds []string, ctx SessionSetupContext) []string {
	if len(cmds) == 0 {
		return nil
	}
	result := make([]string, len(cmds))
	for i, raw := range cmds {
		tmpl, err := template.New("setup").Parse(raw)
		if err != nil {
			result[i] = raw
			continue
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			result[i] = raw
			continue
		}
		result[i] = buf.String()
	}
	return result
}

// expandDirTemplate expands Go text/template strings in dir fields.
// On parse or execute error, the raw dir is returned (graceful fallback).
func expandDirTemplate(dir string, ctx SessionSetupContext) string {
	if dir == "" || !strings.Contains(dir, "{{") {
		return dir
	}
	tmpl, err := template.New("dir").Parse(dir)
	if err != nil {
		return dir
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return dir
	}
	return buf.String()
}

// resolveSetupScript resolves a session_setup_script path relative to cityPath.
// Returns the path unchanged if already absolute.
func resolveSetupScript(script, cityPath string) string {
	if script == "" || filepath.IsAbs(script) {
		return script
	}
	return filepath.Join(cityPath, script)
}

// poolAgents builds agent.Agent instances for a pool at the desired count.
// If pool.Max == 1, uses the bare agent name (no suffix).
// If pool.Max > 1, names follow the pattern {name}-{n} (1-indexed).
// Sessions follow the session naming template (default: gc-{city}-{name}).
func poolAgents(cfgAgent *config.Agent, desired int, cityName, cityPath string,
	ws *config.Workspace, providers map[string]config.ProviderSpec,
	lookPath config.LookPathFunc, fs fsys.FS, sp session.Provider,
	rigs []config.Rig, sessionTemplate string, _ config.FormulaLayers,
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
		// Build the qualified instance name for rig-scoped pools.
		qualifiedInstance := name
		if cfgAgent.Dir != "" {
			qualifiedInstance = cfgAgent.Dir + "/" + name
		}

		// Expand dir template for pool instance (e.g. ".gc/worktrees/{{.Rig}}/{{.Agent}}").
		expandedDir := expandDirTemplate(cfgAgent.Dir, SessionSetupContext{
			Agent:    qualifiedInstance,
			Rig:      cfgAgent.Dir,
			CityRoot: cityPath,
			CityName: cityName,
		})

		// Deep-copy the agent config for instance resolution.
		instanceAgent := config.Agent{
			Name:                   name,
			Dir:                    expandedDir,
			Provider:               cfgAgent.Provider,
			PromptTemplate:         cfgAgent.PromptTemplate,
			Nudge:                  cfgAgent.Nudge,
			StartCommand:           cfgAgent.StartCommand,
			PromptMode:             cfgAgent.PromptMode,
			PromptFlag:             cfgAgent.PromptFlag,
			ReadyDelayMs:           cfgAgent.ReadyDelayMs,
			ReadyPromptPrefix:      cfgAgent.ReadyPromptPrefix,
			EmitsPermissionWarning: cfgAgent.EmitsPermissionWarning,
			HooksInstalled:         cfgAgent.HooksInstalled,
			WorkQuery:              cfgAgent.WorkQuery,
			SlingQuery:             cfgAgent.SlingQuery,
			SessionSetupScript:     cfgAgent.SessionSetupScript,
			OverlayDir:             cfgAgent.OverlayDir,
			SourceDir:              cfgAgent.SourceDir,
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
		if len(cfgAgent.PreStart) > 0 {
			instanceAgent.PreStart = make([]string, len(cfgAgent.PreStart))
			copy(instanceAgent.PreStart, cfgAgent.PreStart)
		}
		if len(cfgAgent.SessionSetup) > 0 {
			instanceAgent.SessionSetup = make([]string, len(cfgAgent.SessionSetup))
			copy(instanceAgent.SessionSetup, cfgAgent.SessionSetup)
		}

		resolved, err := config.ResolveProvider(&instanceAgent, ws, providers, lookPath)
		if err != nil {
			return nil, fmt.Errorf("agent %q instance %q: %w", cfgAgent.Name, name, err)
		}

		// Resolve per-instance working directory (may differ from base if dir has templates).
		instanceWorkDir := workDir
		if expandedDir != cfgAgent.Dir {
			iwd, iwdErr := resolveAgentDir(cityPath, expandedDir)
			if iwdErr != nil {
				return nil, fmt.Errorf("agent %q instance %q: %w", cfgAgent.Name, name, iwdErr)
			}
			instanceWorkDir = iwd
		}
		agentEnv := map[string]string{
			"GC_AGENT": qualifiedInstance,
			"GC_CITY":  cityPath,
			"GC_DIR":   instanceWorkDir,
		}

		// Install provider hooks if configured.
		if ih := config.ResolveInstallHooks(cfgAgent, ws); len(ih) > 0 {
			if hErr := hooks.Install(fs, cityPath, instanceWorkDir, ih); hErr != nil {
				// Non-fatal for pool instances.
				_ = hErr
			}
		}

		// Copy overlay directory into agent working directory.
		// For exec session providers (e.g., K8s), skip host-side copy and
		// pass the overlay path through the wire format instead.
		overlayDir := resolveOverlayDir(cfgAgent.OverlayDir, cityPath)
		if overlayDir != "" && !isExecSessionProvider() {
			_ = overlay.CopyDir(overlayDir, instanceWorkDir, io.Discard) // Non-fatal for pool instances.
		}

		command := resolved.CommandString()
		if sa := settingsArgs(cityPath, resolved.Name); sa != "" {
			command = command + " " + sa
		}
		rigName := resolveRigForAgent(instanceWorkDir, rigs)
		if rigName != "" {
			agentEnv["GC_RIG"] = rigName
		}
		prompt := renderPrompt(fs, cityPath, cityName, cfgAgent.PromptTemplate, PromptContext{
			CityRoot:      cityPath,
			AgentName:     qualifiedInstance,
			TemplateName:  cfgAgent.Name,
			RigName:       rigName,
			WorkDir:       instanceWorkDir,
			IssuePrefix:   findRigPrefix(rigName, rigs),
			DefaultBranch: defaultBranchFor(instanceWorkDir),
			WorkQuery:     cfgAgent.EffectiveWorkQuery(),
			SlingQuery:    cfgAgent.EffectiveSlingQuery(),
			Env:           cfgAgent.Env,
		}, sessionTemplate, io.Discard)
		env := mergeEnv(passthroughEnv(), resolved.Env, cfgAgent.Env, agentEnv)
		hasHooks := config.AgentHasHooks(cfgAgent, ws, resolved.Name)
		beacon := session.FormatBeacon(cityName, qualifiedInstance, !hasHooks)
		if prompt != "" {
			prompt = beacon + "\n\n" + prompt
		} else {
			prompt = beacon
		}
		// Expand session_setup templates with session context.
		sessName := agent.SessionNameFor(cityName, qualifiedInstance, sessionTemplate)
		configDir := cityPath
		if cfgAgent.SourceDir != "" {
			configDir = cfgAgent.SourceDir
		}
		expandedSetup := expandSessionSetup(instanceAgent.SessionSetup, SessionSetupContext{
			Session:   sessName,
			Agent:     qualifiedInstance,
			Rig:       rigName,
			CityRoot:  cityPath,
			CityName:  cityName,
			WorkDir:   instanceWorkDir,
			ConfigDir: configDir,
		})
		resolvedScript := resolveSetupScript(instanceAgent.SessionSetupScript, cityPath)
		expandedPreStart := expandSessionSetup(instanceAgent.PreStart, SessionSetupContext{
			Session:   sessName,
			Agent:     qualifiedInstance,
			Rig:       rigName,
			CityRoot:  cityPath,
			CityName:  cityName,
			WorkDir:   instanceWorkDir,
			ConfigDir: configDir,
		})
		hints := agent.StartupHints{
			ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
			ReadyDelayMs:           resolved.ReadyDelayMs,
			ProcessNames:           resolved.ProcessNames,
			EmitsPermissionWarning: resolved.EmitsPermissionWarning,
			Nudge:                  cfgAgent.Nudge,
			PreStart:               expandedPreStart,
			SessionSetup:           expandedSetup,
			SessionSetupScript:     resolvedScript,
			OverlayDir:             overlayDir,
		}
		agents = append(agents, agent.New(qualifiedInstance, cityName, command, prompt, env, hints, instanceWorkDir, sessionTemplate, nil, sp))
	}
	return agents, nil
}
