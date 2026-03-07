package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/hooks"
	"github.com/gastownhall/gascity/internal/runtime"
	sessionauto "github.com/gastownhall/gascity/internal/runtime/auto"
)

// agentBuildParams holds shared, per-city parameters for building agents.
// These are constant across all agents in a single buildAgents call.
type agentBuildParams struct {
	cityName        string
	cityPath        string
	workspace       *config.Workspace
	providers       map[string]config.ProviderSpec
	lookPath        config.LookPathFunc
	fs              fsys.FS
	sp              runtime.Provider
	rigs            []config.Rig
	sessionTemplate string
	beaconTime      time.Time
	packDirs        []string
	packOverlayDirs []string
	rigOverlayDirs  map[string][]string
	globalFragments []string
	stderr          io.Writer
}

// buildOneAgent resolves a config.Agent into an agent.Agent. This is the
// single canonical path for building agents — both fixed agents and pool
// instances flow through here. The caller is responsible for setting the
// correct Name and Dir on cfgAgent (pool callers modify these for each
// instance before calling).
//
// qualifiedName is the agent's canonical identity (e.g., "mayor" or
// "hello-world/polecat-2"). fpExtra carries additional data for config
// fingerprinting (e.g., pool bounds); pass nil for pool instances.
func buildOneAgent(p *agentBuildParams, cfgAgent *config.Agent, qualifiedName string, fpExtra map[string]string, onStop ...func() error) (agent.Agent, error) {
	// Resolve provider preset.
	resolved, err := config.ResolveProvider(cfgAgent, p.workspace, p.providers, p.lookPath)
	if err != nil {
		return nil, fmt.Errorf("agent %q: %w", qualifiedName, err)
	}

	// Validate session vs provider compatibility.
	if cfgAgent.Session == "acp" && !resolved.SupportsACP {
		return nil, fmt.Errorf("agent %q: session = \"acp\" but provider %q does not support ACP (set supports_acp = true on the provider)", qualifiedName, resolved.Name)
	}

	// Expand dir template (e.g. ".gc/worktrees/{{.Rig}}/{{.Agent}}").
	expandedDir := expandDirTemplate(cfgAgent.Dir, SessionSetupContext{
		Agent:    qualifiedName,
		Rig:      cfgAgent.Dir,
		CityRoot: p.cityPath,
		CityName: p.cityName,
	})
	workDir, err := resolveAgentDir(p.cityPath, expandedDir)
	if err != nil {
		return nil, fmt.Errorf("agent %q: %w", qualifiedName, err)
	}

	// Install provider hooks if configured.
	if ih := config.ResolveInstallHooks(cfgAgent, p.workspace); len(ih) > 0 {
		if hErr := hooks.Install(p.fs, p.cityPath, workDir, ih); hErr != nil {
			fmt.Fprintf(p.stderr, "agent %q: hooks: %v\n", qualifiedName, hErr) //nolint:errcheck // best-effort stderr
		}
	}

	// Resolve overlay directory path (provider handles the copy).
	overlayDir := resolveOverlayDir(cfgAgent.OverlayDir, p.cityPath)

	// Build copy_files for container providers.
	var copyFiles []runtime.CopyEntry
	command := resolved.CommandString()
	if sa := settingsArgs(p.cityPath, resolved.Name); sa != "" {
		command = command + " " + sa
		settingsFile := filepath.Join(p.cityPath, ".gc", "settings.json")
		copyFiles = append(copyFiles, runtime.CopyEntry{Src: settingsFile, RelDst: filepath.Join(".gc", "settings.json")})
	}
	scriptsDir := filepath.Join(p.cityPath, ".gc", "scripts")
	if info, sErr := os.Stat(scriptsDir); sErr == nil && info.IsDir() {
		copyFiles = append(copyFiles, runtime.CopyEntry{Src: scriptsDir, RelDst: filepath.Join(".gc", "scripts")})
	}
	copyFiles = stageHookFiles(copyFiles, p.cityPath, workDir)

	// Resolve rig association for prompt context.
	rigName := resolveRigForAgent(workDir, p.rigs)

	// Compute session name early — needed for env vars and later template expansion.
	sessName := agent.SessionNameFor(p.cityName, qualifiedName, p.sessionTemplate)

	// Build agent environment.
	// Emit both new GC_SESSION_* vars and legacy GC_AGENT/GC_CITY/GC_DIR
	// vars during the migration transition period (removed in Phase 5).
	agentEnv := map[string]string{
		// New canonical vars
		"GC_SESSION_NAME": sessName,
		"GC_TEMPLATE":     qualifiedName,
		// Legacy compat vars
		"GC_AGENT": qualifiedName,
		"GC_CITY":  p.cityPath,
		"GC_DIR":   workDir,
	}
	if rigName != "" {
		agentEnv["GC_RIG"] = rigName
	}

	// Render prompt with beacon.
	var prompt string
	if resolved.PromptMode != "none" {
		fragments := mergeFragmentLists(p.globalFragments, cfgAgent.InjectFragments)
		prompt = renderPrompt(p.fs, p.cityPath, p.cityName, cfgAgent.PromptTemplate, PromptContext{
			CityRoot:      p.cityPath,
			AgentName:     qualifiedName,
			TemplateName:  cfgAgent.Name,
			RigName:       rigName,
			WorkDir:       workDir,
			IssuePrefix:   findRigPrefix(rigName, p.rigs),
			DefaultBranch: defaultBranchFor(workDir),
			WorkQuery:     cfgAgent.EffectiveWorkQuery(),
			SlingQuery:    cfgAgent.EffectiveSlingQuery(),
			Env:           cfgAgent.Env,
		}, p.sessionTemplate, p.stderr, p.packDirs, fragments)
		hasHooks := config.AgentHasHooks(cfgAgent, p.workspace, resolved.Name)
		beacon := runtime.FormatBeaconAt(p.cityName, qualifiedName, !hasHooks, p.beaconTime)
		if prompt != "" {
			prompt = beacon + "\n\n" + prompt
		} else {
			prompt = beacon
		}
	}

	// Merge environment layers.
	env := mergeEnv(passthroughEnv(), expandEnvMap(resolved.Env), expandEnvMap(cfgAgent.Env), agentEnv)

	// Register ACP route on the auto provider for dynamic sessions
	// (e.g., pool instances) not known at newSessionProvider() time.
	if cfgAgent.Session == "acp" {
		if autoSP, ok := p.sp.(*sessionauto.Provider); ok {
			autoSP.RouteACP(sessName)
		}
	}

	configDir := p.cityPath
	if cfgAgent.SourceDir != "" {
		configDir = cfgAgent.SourceDir
	}
	setupCtx := SessionSetupContext{
		Session:   sessName,
		Agent:     qualifiedName,
		Rig:       rigName,
		CityRoot:  p.cityPath,
		CityName:  p.cityName,
		WorkDir:   workDir,
		ConfigDir: configDir,
	}
	if strings.Contains(command, "{{") {
		expanded := expandSessionSetup([]string{command}, setupCtx)
		command = expanded[0]
	}
	expandedSetup := expandSessionSetup(cfgAgent.SessionSetup, setupCtx)
	resolvedScript := resolveSetupScript(cfgAgent.SessionSetupScript, p.cityPath)
	expandedPreStart := expandSessionSetup(cfgAgent.PreStart, setupCtx)
	expandedLive := expandSessionSetup(cfgAgent.SessionLive, setupCtx)

	hints := agent.StartupHints{
		ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
		ReadyDelayMs:           resolved.ReadyDelayMs,
		ProcessNames:           resolved.ProcessNames,
		EmitsPermissionWarning: resolved.EmitsPermissionWarning,
		Nudge:                  cfgAgent.Nudge,
		PreStart:               expandedPreStart,
		SessionSetup:           expandedSetup,
		SessionSetupScript:     resolvedScript,
		SessionLive:            expandedLive,
		PackOverlayDirs:        effectiveOverlayDirs(p.packOverlayDirs, p.rigOverlayDirs, rigName),
		OverlayDir:             overlayDir,
		CopyFiles:              copyFiles,
	}
	return agent.New(qualifiedName, p.cityName, command, prompt, env, hints, workDir, p.sessionTemplate, fpExtra, p.sp, onStop...), nil
}

// newAgentBuildParams constructs agentBuildParams from the common startup values.
func newAgentBuildParams(cityName, cityPath string, cfg *config.City, sp runtime.Provider, beaconTime time.Time, stderr io.Writer) *agentBuildParams {
	return &agentBuildParams{
		cityName:        cityName,
		cityPath:        cityPath,
		workspace:       &cfg.Workspace,
		providers:       cfg.Providers,
		lookPath:        exec.LookPath,
		fs:              fsys.OSFS{},
		sp:              sp,
		rigs:            cfg.Rigs,
		sessionTemplate: cfg.Workspace.SessionTemplate,
		beaconTime:      beaconTime,
		packDirs:        cfg.PackDirs,
		packOverlayDirs: cfg.PackOverlayDirs,
		rigOverlayDirs:  cfg.RigOverlayDirs,
		globalFragments: cfg.Workspace.GlobalFragments,
		stderr:          stderr,
	}
}

// effectiveOverlayDirs merges city-level and rig-level pack overlay dirs.
// City dirs come first (lower priority), then rig-specific dirs.
func effectiveOverlayDirs(cityDirs []string, rigDirs map[string][]string, rigName string) []string {
	rigSpecific := rigDirs[rigName]
	if len(rigSpecific) == 0 {
		return cityDirs
	}
	if len(cityDirs) == 0 {
		return rigSpecific
	}
	merged := make([]string, 0, len(cityDirs)+len(rigSpecific))
	merged = append(merged, cityDirs...)
	merged = append(merged, rigSpecific...)
	return merged
}
