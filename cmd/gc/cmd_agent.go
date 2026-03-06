package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/spf13/cobra"
)

// AgentListEntry is the JSON output format for a single agent in "gc agent list --json".
type AgentListEntry struct {
	Name          string         `json:"name"`
	QualifiedName string         `json:"qualified_name"`
	Dir           string         `json:"dir"`
	Scope         string         `json:"scope"`
	Suspended     bool           `json:"suspended"`
	RigSuspended  bool           `json:"rig_suspended"`
	Pool          *PoolJSON      `json:"pool"`
	Multi         bool           `json:"multi,omitempty"`
	Instances     []InstanceJSON `json:"instances,omitempty"`
}

// InstanceJSON is the JSON output format for a multi-instance agent.
type InstanceJSON struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// loadCityConfig loads the city configuration with full pack expansion.
// Most CLI commands need this instead of config.Load so that agents defined
// via packs are visible. The only exceptions are quick pre-fetch checks
// in cmd_config.go and cmd_start.go that intentionally use config.Load to
// discover remote packs before fetching them.
func loadCityConfig(cityPath string) (*config.City, error) {
	cfg, _, err := config.LoadWithIncludes(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		return nil, err
	}
	injectBuiltinPacks(cfg, cityPath)
	return cfg, nil
}

// loadCityConfigFS is the testable variant of loadCityConfig that accepts a
// filesystem implementation. Used by functions that take an fsys.FS parameter
// for unit testing.
func loadCityConfigFS(fs fsys.FS, tomlPath string) (*config.City, error) {
	cfg, _, err := config.LoadWithIncludes(fs, tomlPath)
	return cfg, err
}

// loadCityConfigForEditFS loads the raw city config WITHOUT pack/include
// expansion. Use for commands that modify city.toml and write it back —
// preserves include directives, pack references, and patches.
func loadCityConfigForEditFS(fs fsys.FS, tomlPath string) (*config.City, error) {
	return config.Load(fs, tomlPath)
}

// resolveAgentIdentity resolves an agent input string to a config.Agent using
// 3-step resolution:
//  1. Literal: try the input as-is (e.g., "mayor" or "hello-world/polecat").
//  2. Contextual: if input has no "/" and currentRigDir is set, try
//     "{currentRigDir}/{input}" to resolve rig-scoped agents from context.
//  3. Unambiguous bare name: scan all agents by Name (ignoring Dir).
//     Succeeds only when exactly one agent matches.
func resolveAgentIdentity(cfg *config.City, input, currentRigDir string) (config.Agent, bool) {
	// Step 1: literal match.
	if a, ok := findAgentByQualified(cfg, input); ok {
		return a, true
	}
	// Step 2: contextual (bare name + rig context).
	if !strings.Contains(input, "/") && currentRigDir != "" {
		if a, ok := findAgentByQualified(cfg, currentRigDir+"/"+input); ok {
			return a, true
		}
	}
	// Step 3: unambiguous bare name — scan all agents by Name (ignoring Dir).
	// Succeeds only when exactly one agent matches. Handles pool instances too.
	if !strings.Contains(input, "/") {
		var matches []config.Agent
		for _, a := range cfg.Agents {
			if a.Name == input {
				matches = append(matches, a)
				continue
			}
			// Pool instance: "polecat-2" matches pool "polecat" with Max >= 2 (or unlimited).
			if a.Pool != nil && a.Pool.IsMultiInstance() {
				prefix := a.Name + "-"
				if strings.HasPrefix(input, prefix) {
					suffix := input[len(prefix):]
					if n, err := strconv.Atoi(suffix); err == nil && n >= 1 && (a.Pool.IsUnlimited() || n <= a.Pool.Max) {
						instance := a
						instance.Name = input
						instance.Pool = nil
						matches = append(matches, instance)
					}
				}
			}
		}
		if len(matches) == 1 {
			return matches[0], true
		}
	}
	return config.Agent{}, false
}

// findAgentByQualified looks up an agent by its qualified identity (dir+name).
// For pool agents with Max > 1, matches {name}-{N} patterns within the same dir.
// For multi agents, matches {template}/{instance} by checking the registry.
func findAgentByQualified(cfg *config.City, identity string) (config.Agent, bool) {
	dir, name := config.ParseQualifiedName(identity)
	for _, a := range cfg.Agents {
		if a.Dir == dir && a.Name == name {
			return a, true
		}
		// Pool: match {name}-{N} within same dir.
		if a.Dir == dir && a.Pool != nil && a.Pool.IsMultiInstance() {
			prefix := a.Name + "-"
			if strings.HasPrefix(name, prefix) {
				suffix := name[len(prefix):]
				if n, err := strconv.Atoi(suffix); err == nil && n >= 1 && (a.Pool.IsUnlimited() || n <= a.Pool.Max) {
					instance := a
					instance.Name = name
					instance.Pool = nil // instances are not pools
					return instance, true
				}
			}
		}
	}
	// Multi: try interpreting as {template}/{instance}.
	// For city-scoped multi agents: "researcher/spike-1" → template="researcher", instance="spike-1".
	// For rig-scoped: "rig/researcher/spike-1" → template="rig/researcher", instance="spike-1".
	if strings.Contains(identity, "/") {
		// Try all multi agents to see if the identity starts with their QN.
		for _, a := range cfg.Agents {
			if !a.IsMulti() {
				continue
			}
			templateQN := a.QualifiedName()
			prefix := templateQN + "/"
			if strings.HasPrefix(identity, prefix) {
				instanceName := identity[len(prefix):]
				if instanceName != "" {
					instance := a
					instance.Name = instanceName
					instance.Multi = false
					instance.PoolName = templateQN
					return instance, true
				}
			}
		}
	}
	return config.Agent{}, false
}

// currentRigContext returns the rig name that provides context for bare agent
// name resolution. Checks GC_DIR env var first, then cwd.
func currentRigContext(cfg *config.City) string {
	if gcDir := os.Getenv("GC_DIR"); gcDir != "" {
		for _, r := range cfg.Rigs {
			if filepath.Clean(gcDir) == filepath.Clean(r.Path) {
				return r.Name
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if name, _, found := findEnclosingRig(cwd, cfg.Rigs); found {
			return name
		}
	}
	return ""
}

func newAgentCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Long: `Manage agents in the city workspace.

Agents are the autonomous workers that execute tasks. Each agent runs
in its own tmux session with a configured provider (Claude, Codex, etc).
Agents can be fixed (single instance) or pooled (multiple instances
scaled by demand).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc agent: missing subcommand (add, attach, destroy, drain, drain-ack, drain-check, kill, list, logs, nudge, peek, request-restart, resume, start, status, stop, suspend, undrain)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc agent: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newAgentAddCmd(stdout, stderr),
		newAgentAttachCmd(stdout, stderr),
		newAgentDestroyCmd(stdout, stderr),
		newAgentDrainCmd(stdout, stderr),
		newAgentDrainAckCmd(stdout, stderr),
		newAgentDrainCheckCmd(stdout, stderr),
		newAgentKillCmd(stdout, stderr),
		newAgentListCmd(stdout, stderr),
		newAgentLogsCmd(stdout, stderr),
		newAgentNudgeCmd(stdout, stderr),
		newAgentPeekCmd(stdout, stderr),
		newAgentRequestRestartCmd(stdout, stderr),
		newAgentResumeCmd(stdout, stderr),
		newAgentStartCmd(stdout, stderr),
		newAgentStatusCmd(stdout, stderr),
		newAgentStopCmd(stdout, stderr),
		newAgentSuspendCmd(stdout, stderr),
		newAgentUndrainCmd(stdout, stderr),
	)
	return cmd
}

func newAgentAddCmd(stdout, stderr io.Writer) *cobra.Command {
	var name, promptTemplate, dir string
	var suspended bool
	cmd := &cobra.Command{
		Use:   "add --name <name>",
		Short: "Add an agent to the workspace",
		Long: `Add a new agent to the workspace configuration.

Appends an [[agents]] block to city.toml. The agent will be started
on the next "gc start" or controller reconcile tick. Use --dir to
scope the agent to a rig's working directory.`,
		Example: `  gc agent add --name mayor
  gc agent add --name polecat --dir my-project
  gc agent add --name worker --prompt-template prompts/worker.md --suspended`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentAdd(name, promptTemplate, dir, suspended, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name of the agent")
	cmd.Flags().StringVar(&promptTemplate, "prompt-template", "", "Path to prompt template file (relative to city root)")
	cmd.Flags().StringVar(&dir, "dir", "", "Working directory for the agent (relative to city root)")
	cmd.Flags().BoolVar(&suspended, "suspended", false, "Register the agent in suspended state")
	return cmd
}

func newAgentListCmd(stdout, stderr io.Writer) *cobra.Command {
	var dir string
	var jsonFlag bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace agents",
		Long: `List all agents configured in city.toml with annotations.

Shows each agent's qualified name, suspension status, rig suspension
inheritance, and pool configuration. Use --dir to filter by working
directory.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentList(dir, jsonFlag, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Filter agents by working directory")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Output in JSON format")
	return cmd
}

func newAgentAttachCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach to an agent session",
		Long: `Attach to an agent's tmux session for interactive debugging.

Starts the session if not already running, then attaches your terminal.
Detach with the standard tmux detach key (Ctrl-B D by default). Supports
both fixed agents and pool instances (e.g. "polecat-2").`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentAttach(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentAttach is the CLI entry point for attaching to an agent session.
// It loads city config, finds the agent, determines the command, constructs
// the session name, and delegates to doAgentAttach.
func cmdAgentAttach(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent attach: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent attach: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent attach: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Find agent in config.
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		if len(cfg.Agents) == 0 {
			fmt.Fprintln(stderr, "gc agent attach: no agents configured; run 'gc init' to set up your city") //nolint:errcheck // best-effort stderr
		} else {
			fmt.Fprintln(stderr, agentNotFoundMsg("gc agent attach", agentName, cfg)) //nolint:errcheck // best-effort stderr
		}
		return 1
	}
	cfgAgent := &found

	// Determine command: agent > workspace > auto-detect.
	resolved, err := config.ResolveProvider(cfgAgent, &cfg.Workspace, cfg.Providers, exec.LookPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent attach: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Construct session name and attach.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	hints := agent.StartupHints{
		ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
		ReadyDelayMs:           resolved.ReadyDelayMs,
		ProcessNames:           resolved.ProcessNames,
		EmitsPermissionWarning: resolved.EmitsPermissionWarning,
	}
	a := agent.New(cfgAgent.QualifiedName(), cityName, resolved.CommandString(), "", resolved.Env, hints, "", cfg.Workspace.SessionTemplate, nil, sp)
	return doAgentAttach(a, stdout, stderr)
}

// doAgentAttach is the pure logic for "gc agent attach <name>".
// It is idempotent: starts the session if not already running, then attaches.
func doAgentAttach(a agent.Agent, stdout, stderr io.Writer) int {
	if !a.IsRunning() {
		if err := a.Start(context.Background()); err != nil {
			fmt.Fprintf(stderr, "gc agent attach: starting session: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	fmt.Fprintf(stdout, "Attaching to agent '%s'...\n", a.Name()) //nolint:errcheck // best-effort stdout

	if err := a.Attach(); err != nil {
		fmt.Fprintf(stderr, "gc agent attach: attaching to session: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// cmdAgentAdd is the CLI entry point for adding an agent. It locates
// the city root and delegates to doAgentAdd.
func cmdAgentAdd(name, promptTemplate, dir string, suspended bool, stdout, stderr io.Writer) int {
	if name == "" {
		fmt.Fprintln(stderr, "gc agent add: missing --name flag") //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentAdd(fsys.OSFS{}, cityPath, name, promptTemplate, dir, suspended, stdout, stderr)
}

// doAgentAdd is the pure logic for "gc agent add". It loads city.toml,
// checks for duplicates, appends the new agent, and writes back.
// Accepts an injected FS for testability.
func doAgentAdd(fs fsys.FS, cityPath, name, promptTemplate, dir string, suspended bool, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := loadCityConfigForEditFS(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	inputDir, inputName := config.ParseQualifiedName(name)
	for _, a := range cfg.Agents {
		if a.Dir == inputDir && a.Name == inputName {
			fmt.Fprintf(stderr, "gc agent add: agent %q already exists\n", name) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	// If input contained a dir component, use it (overrides --dir flag).
	if inputDir != "" {
		dir = inputDir
		name = inputName
	}

	newAgent := config.Agent{
		Name:           name,
		Dir:            dir,
		PromptTemplate: promptTemplate,
		Suspended:      suspended,
	}
	cfg.Agents = append(cfg.Agents, newAgent)
	content, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.WriteFile(tomlPath, content, 0o644); err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Added agent '%s'\n", name) //nolint:errcheck // best-effort stdout
	return 0
}

func newAgentSuspendCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "suspend <name>",
		Short: "Suspend an agent (reconciler will skip it)",
		Long: `Suspend an agent by setting suspended=true in city.toml.

Suspended agents are skipped by the reconciler — their sessions are not
started or restarted. Existing sessions continue running but won't be
replaced if they exit. Use "gc agent resume" to restore.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentSuspend(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentSuspend is the CLI entry point for suspending an agent.
func cmdAgentSuspend(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent suspend: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent suspend: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentSuspend(fsys.OSFS{}, cityPath, args[0], stdout, stderr)
}

// doAgentSuspend sets suspended=true on the named agent in city.toml.
// Uses raw config (no pack expansion) to preserve includes/patches on write-back.
// If the agent isn't found in raw config but exists in expanded config, it's
// pack-derived and the user gets a helpful error directing them to [[patches]].
// Accepts an injected FS for testability.
func doAgentSuspend(fs fsys.FS, cityPath, name string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")

	// Phase 1: load raw config (no expansion) for safe write-back.
	cfg, err := loadCityConfigForEditFS(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent suspend: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Try to find agent in raw config.
	resolved, ok := resolveAgentIdentity(cfg, name, currentRigContext(cfg))
	if ok {
		// Found in raw config — toggle and write back.
		for i := range cfg.Agents {
			if cfg.Agents[i].Dir == resolved.Dir && cfg.Agents[i].Name == resolved.Name {
				cfg.Agents[i].Suspended = true
				break
			}
		}
		content, err := cfg.Marshal()
		if err != nil {
			fmt.Fprintf(stderr, "gc agent suspend: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if err := fs.WriteFile(tomlPath, content, 0o644); err != nil {
			fmt.Fprintf(stderr, "gc agent suspend: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		fmt.Fprintf(stdout, "Suspended agent '%s'\n", name) //nolint:errcheck // best-effort stdout
		return 0
	}

	// Phase 2: not in raw config — check expanded config for pack-derived agents.
	expanded, err := loadCityConfigFS(fs, tomlPath)
	if err != nil {
		// Fall through to generic not-found using raw cfg.
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent suspend", name, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}
	if _, ok := resolveAgentIdentity(expanded, name, currentRigContext(expanded)); ok {
		fmt.Fprintf(stderr, "gc agent suspend: agent %q is defined by a pack — use [[patches]] to override\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Not found anywhere.
	fmt.Fprintln(stderr, agentNotFoundMsg("gc agent suspend", name, expanded)) //nolint:errcheck // best-effort stderr
	return 1
}

func newAgentResumeCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <name>",
		Short: "Resume a suspended agent",
		Long: `Resume a suspended agent by clearing suspended in city.toml.

The reconciler will start the agent on its next tick. Supports bare
names (resolved via rig context) and qualified names (e.g. "myrig/worker").`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentResume(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentResume is the CLI entry point for resuming a suspended agent.
func cmdAgentResume(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent resume: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent resume: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentResume(fsys.OSFS{}, cityPath, args[0], stdout, stderr)
}

// doAgentResume clears suspended on the named agent in city.toml.
// Uses raw config (no pack expansion) to preserve includes/patches on write-back.
// If the agent isn't found in raw config but exists in expanded config, it's
// pack-derived and the user gets a helpful error directing them to [[patches]].
// Accepts an injected FS for testability.
func doAgentResume(fs fsys.FS, cityPath, name string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")

	// Phase 1: load raw config (no expansion) for safe write-back.
	cfg, err := loadCityConfigForEditFS(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent resume: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Try to find agent in raw config.
	resolved, ok := resolveAgentIdentity(cfg, name, currentRigContext(cfg))
	if ok {
		// Found in raw config — toggle and write back.
		for i := range cfg.Agents {
			if cfg.Agents[i].Dir == resolved.Dir && cfg.Agents[i].Name == resolved.Name {
				cfg.Agents[i].Suspended = false
				break
			}
		}
		content, err := cfg.Marshal()
		if err != nil {
			fmt.Fprintf(stderr, "gc agent resume: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		if err := fs.WriteFile(tomlPath, content, 0o644); err != nil {
			fmt.Fprintf(stderr, "gc agent resume: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		fmt.Fprintf(stdout, "Resumed agent '%s'\n", name) //nolint:errcheck // best-effort stdout
		return 0
	}

	// Phase 2: not in raw config — check expanded config for pack-derived agents.
	expanded, err := loadCityConfigFS(fs, tomlPath)
	if err != nil {
		// Fall through to generic not-found using raw cfg.
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent resume", name, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}
	if _, ok := resolveAgentIdentity(expanded, name, currentRigContext(expanded)); ok {
		fmt.Fprintf(stderr, "gc agent resume: agent %q is defined by a pack — use [[patches]] to override\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Not found anywhere.
	fmt.Fprintln(stderr, agentNotFoundMsg("gc agent resume", name, expanded)) //nolint:errcheck // best-effort stderr
	return 1
}

func newAgentNudgeCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "nudge <agent-name> <message>",
		Short: "Send a message to wake or redirect an agent",
		Long: `Send a text message to an agent's running session.

The message is typed into the agent's tmux session as if a human typed
it. Use this to redirect an agent's attention, provide new instructions,
or wake it from an idle state.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentNudge(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentNudge is the CLI entry point for nudging an agent. It validates the
// agent exists in city.toml, constructs a minimal Agent, and delegates to
// doAgentNudge.
func cmdAgentNudge(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc agent nudge: usage: gc agent nudge <agent-name> <message>") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]
	message := strings.Join(args[1:], " ")

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent nudge: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent nudge: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent nudge", agentName, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve session name and construct a lightweight Handle.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	h := agent.HandleFor(found.QualifiedName(), cityName, cfg.Workspace.SessionTemplate, sp)
	return doAgentNudge(h, message, stdout, stderr)
}

// doAgentNudge is the pure logic for "gc agent nudge". Accepts an injected
// Handle for testability.
func doAgentNudge(a agent.Handle, message string, stdout, stderr io.Writer) int {
	if err := a.Nudge(message); err != nil {
		fmt.Fprintf(stderr, "gc agent nudge: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Nudged agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
	return 0
}

func newAgentPeekCmd(stdout, stderr io.Writer) *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "peek <agent-name>",
		Short: "Capture recent output from an agent session",
		Long: `Capture recent terminal output from an agent's tmux session.

Reads the session's scrollback buffer without attaching. Use --lines
to control how much output to capture (0 = all available scrollback).
Useful for monitoring agent progress without interrupting it.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentPeek(args, lines, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 50, "Number of lines to capture (0 = all scrollback)")
	return cmd
}

// cmdAgentPeek is the CLI entry point for peeking at agent output. It
// validates the agent exists in city.toml, constructs a minimal Agent,
// and delegates to doAgentPeek.
func cmdAgentPeek(args []string, lines int, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent peek: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent peek: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent peek: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent peek", agentName, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve session name and construct a lightweight Handle.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	h := agent.HandleFor(found.QualifiedName(), cityName, cfg.Workspace.SessionTemplate, sp)
	return doAgentPeek(h, lines, stdout, stderr)
}

// doAgentPeek is the pure logic for "gc agent peek". Accepts an injected
// Handle for testability.
func doAgentPeek(a agent.Handle, lines int, stdout, stderr io.Writer) int {
	output, err := a.Peek(lines)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent peek: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprint(stdout, output) //nolint:errcheck // best-effort stdout
	return 0
}

// cmdAgentList is the CLI entry point for listing agents. It locates
// the city root and delegates to doAgentList.
func cmdAgentList(dirFilter string, jsonOutput bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if jsonOutput {
		return doAgentListJSON(fsys.OSFS{}, cityPath, dirFilter, stdout, stderr)
	}
	return doAgentList(fsys.OSFS{}, cityPath, dirFilter, stdout, stderr)
}

// doAgentListJSON outputs agent list as a JSON array. Accepts an injected FS
// for testability.
func doAgentListJSON(fs fsys.FS, cityPath, dirFilter string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := loadCityConfigFS(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Pre-compute suspended rig names.
	suspendedRigs := make(map[string]bool)
	for _, r := range cfg.Rigs {
		if r.Suspended {
			suspendedRigs[r.Name] = true
		}
	}

	var entries []AgentListEntry
	for _, a := range cfg.Agents {
		if dirFilter != "" && a.Dir != dirFilter {
			continue
		}
		scope := "city"
		if a.Dir != "" {
			scope = "rig"
		}
		rigSuspended := a.Dir != "" && suspendedRigs[a.Dir]
		var pool *PoolJSON
		if a.Pool != nil {
			pool = &PoolJSON{Min: a.Pool.Min, Max: a.Pool.Max}
		}
		entry := AgentListEntry{
			Name:          a.Name,
			QualifiedName: a.QualifiedName(),
			Dir:           a.Dir,
			Scope:         scope,
			Suspended:     a.Suspended,
			RigSuspended:  rigSuspended,
			Pool:          pool,
			Multi:         a.IsMulti(),
		}
		if a.IsMulti() {
			store, code := openCityStore(stderr, "gc agent list")
			if code == 0 {
				reg := newMultiRegistry(store)
				instances, iErr := reg.instancesForTemplate(a.QualifiedName())
				if iErr == nil {
					for _, mi := range instances {
						entry.Instances = append(entry.Instances, InstanceJSON{
							Name:  mi.Name,
							State: mi.State,
						})
					}
				}
			}
		}
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
	return 0
}

// ---------------------------------------------------------------------------
// gc agent kill <name>
// ---------------------------------------------------------------------------

func newAgentKillCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "kill <name>",
		Short: "Force-kill an agent session (reconciler will restart it)",
		Long: `Force-kill an agent's tmux session immediately.

The session is destroyed without graceful shutdown. If a controller is
running, it will restart the agent on its next reconcile tick. Use
"gc agent drain" for graceful wind-down instead.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentKill(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func cmdAgentKill(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent kill: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent kill: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent kill: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent kill", agentName, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName = found.QualifiedName()

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sn := sessionName(cityName, agentName, cfg.Workspace.SessionTemplate)
	sp := newSessionProvider()
	rec := openCityRecorder(stderr)
	return doAgentKill(sp, rec, agentName, sn, stdout, stderr)
}

// doAgentKill force-kills an agent's session. The reconciler will restart it
// on its next tick.
func doAgentKill(sp session.Provider, rec events.Recorder,
	agentName, sn string, stdout, stderr io.Writer,
) int {
	if !sp.IsRunning(sn) {
		fmt.Fprintf(stderr, "gc agent kill: agent %q is not running\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := sp.Stop(sn); err != nil {
		fmt.Fprintf(stderr, "gc agent kill: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.AgentStopped,
		Actor:   eventActor(),
		Subject: agentName,
	})
	fmt.Fprintf(stdout, "Killed agent '%s'\n", agentName) //nolint:errcheck // best-effort stdout
	return 0
}

// doAgentList is the pure logic for "gc agent list". It loads city.toml
// and prints the city name header followed by agent names. When dirFilter
// is non-empty, only agents whose Dir matches are shown.
// Accepts an injected FS for testability.
func doAgentList(fs fsys.FS, cityPath, dirFilter string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := loadCityConfigFS(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Pre-compute suspended rig paths for annotation.
	suspendedRigPaths := make(map[string]bool)
	for _, r := range cfg.Rigs {
		if r.Suspended {
			suspendedRigPaths[filepath.Clean(r.Path)] = true
		}
	}

	fmt.Fprintf(stdout, "%s:\n", cfg.Workspace.Name) //nolint:errcheck // best-effort stdout
	for _, a := range cfg.Agents {
		if dirFilter != "" && a.Dir != dirFilter {
			continue
		}
		displayName := a.QualifiedName()
		var annotations []string
		if a.Suspended {
			annotations = append(annotations, "suspended")
		} else if a.Dir != "" && len(suspendedRigPaths) > 0 {
			workDir := a.Dir
			if !filepath.IsAbs(workDir) {
				workDir = filepath.Join(cityPath, workDir)
			}
			if suspendedRigPaths[filepath.Clean(workDir)] {
				annotations = append(annotations, "rig suspended")
			}
		}
		if a.Pool != nil {
			annotations = append(annotations, fmt.Sprintf("pool: min=%d, max=%d", a.Pool.Min, a.Pool.Max))
		}
		if a.IsMulti() {
			annotations = append(annotations, "multi")
		}
		if len(annotations) > 0 {
			fmt.Fprintf(stdout, "  %s  (%s)\n", displayName, strings.Join(annotations, ", ")) //nolint:errcheck // best-effort stdout
		} else {
			fmt.Fprintf(stdout, "  %s\n", displayName) //nolint:errcheck // best-effort stdout
		}
		// Print multi instances if available.
		if a.IsMulti() {
			store, code := openCityStore(stderr, "gc agent list")
			if code == 0 {
				reg := newMultiRegistry(store)
				instances, iErr := reg.instancesForTemplate(a.QualifiedName())
				if iErr == nil {
					for _, mi := range instances {
						fmt.Fprintf(stdout, "    %s/%s  %s\n", a.QualifiedName(), mi.Name, mi.State) //nolint:errcheck // best-effort stdout
					}
				}
			}
		}
	}
	return 0
}
