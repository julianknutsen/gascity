package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// resolveAgentIdentity resolves an agent input string to a config.Agent using
// 2-step resolution:
//  1. Literal: try the input as-is (e.g., "mayor" or "hello-world/polecat").
//  2. Contextual: if input has no "/" and currentRigDir is set, try
//     "{currentRigDir}/{input}" to resolve rig-scoped agents from context.
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
	return config.Agent{}, false
}

// findAgentByQualified looks up an agent by its qualified identity (dir+name).
// For pool agents with Max > 1, matches {name}-{N} patterns within the same dir.
func findAgentByQualified(cfg *config.City, identity string) (config.Agent, bool) {
	dir, name := config.ParseQualifiedName(identity)
	for _, a := range cfg.Agents {
		if a.Dir == dir && a.Name == name {
			return a, true
		}
		// Pool: match {name}-{N} within same dir.
		if a.Dir == dir && a.Pool != nil && a.Pool.Max > 1 {
			prefix := a.Name + "-"
			if strings.HasPrefix(name, prefix) {
				suffix := name[len(prefix):]
				if n, err := strconv.Atoi(suffix); err == nil && n >= 1 && n <= a.Pool.Max {
					instance := a
					instance.Name = name
					instance.Pool = nil // instances are not pools
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
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc agent: missing subcommand (add, attach, drain, drain-ack, drain-check, list, nudge, peek, resume, suspend, undrain)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc agent: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newAgentAddCmd(stdout, stderr),
		newAgentAttachCmd(stdout, stderr),
		newAgentDrainCmd(stdout, stderr),
		newAgentDrainAckCmd(stdout, stderr),
		newAgentDrainCheckCmd(stdout, stderr),
		newAgentListCmd(stdout, stderr),
		newAgentNudgeCmd(stdout, stderr),
		newAgentPeekCmd(stdout, stderr),
		newAgentResumeCmd(stdout, stderr),
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
		Args:  cobra.ArbitraryArgs,
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspace agents",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentList(dir, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Filter agents by working directory")
	return cmd
}

func newAgentAttachCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach to an agent session",
		Args:  cobra.ArbitraryArgs,
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
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
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
			fmt.Fprintf(stderr, "gc agent attach: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
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
	a := agent.New(cfgAgent.QualifiedName(), cityName, resolved.CommandString(), "", resolved.Env, hints, "", cfg.Workspace.SessionTemplate, sp)
	return doAgentAttach(a, stdout, stderr)
}

// doAgentAttach is the pure logic for "gc agent attach <name>".
// It is idempotent: starts the session if not already running, then attaches.
func doAgentAttach(a agent.Agent, stdout, stderr io.Writer) int {
	if !a.IsRunning() {
		if err := a.Start(); err != nil {
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
	cfg, err := config.Load(fs, tomlPath)
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
		Args:  cobra.ArbitraryArgs,
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
// Accepts an injected FS for testability.
func doAgentSuspend(fs fsys.FS, cityPath, name string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent suspend: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve the input to find the target agent (supports bare names and qualified names).
	resolved, ok := resolveAgentIdentity(cfg, name, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent suspend: agent %q not found in city.toml\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}
	found := false
	for i := range cfg.Agents {
		if cfg.Agents[i].Dir == resolved.Dir && cfg.Agents[i].Name == resolved.Name {
			cfg.Agents[i].Suspended = true
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(stderr, "gc agent suspend: agent %q not found in city.toml\n", name) //nolint:errcheck // best-effort stderr
		return 1
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

func newAgentResumeCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "resume <name>",
		Short: "Resume a suspended agent",
		Args:  cobra.ArbitraryArgs,
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
// Accepts an injected FS for testability.
func doAgentResume(fs fsys.FS, cityPath, name string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent resume: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve the input to find the target agent (supports bare names and qualified names).
	resolved, ok := resolveAgentIdentity(cfg, name, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent resume: agent %q not found in city.toml\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}
	found := false
	for i := range cfg.Agents {
		if cfg.Agents[i].Dir == resolved.Dir && cfg.Agents[i].Name == resolved.Name {
			cfg.Agents[i].Suspended = false
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(stderr, "gc agent resume: agent %q not found in city.toml\n", name) //nolint:errcheck // best-effort stderr
		return 1
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

func newAgentNudgeCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "nudge <agent-name> <message>",
		Short: "Send a message to wake or redirect an agent",
		Args:  cobra.ArbitraryArgs,
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
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent nudge: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent nudge: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve session name and construct a minimal Agent.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	a := agent.New(found.QualifiedName(), cityName, "", "", nil, agent.StartupHints{}, "", cfg.Workspace.SessionTemplate, sp)
	return doAgentNudge(a, message, stdout, stderr)
}

// doAgentNudge is the pure logic for "gc agent nudge". Accepts an injected
// Agent for testability.
func doAgentNudge(a agent.Agent, message string, stdout, stderr io.Writer) int {
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
		Args:  cobra.ArbitraryArgs,
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
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent peek: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc agent peek: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve session name and construct a minimal Agent.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	a := agent.New(found.QualifiedName(), cityName, "", "", nil, agent.StartupHints{}, "", cfg.Workspace.SessionTemplate, sp)
	return doAgentPeek(a, lines, stdout, stderr)
}

// doAgentPeek is the pure logic for "gc agent peek". Accepts an injected
// Agent for testability.
func doAgentPeek(a agent.Agent, lines int, stdout, stderr io.Writer) int {
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
func cmdAgentList(dirFilter string, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentList(fsys.OSFS{}, cityPath, dirFilter, stdout, stderr)
}

// doAgentList is the pure logic for "gc agent list". It loads city.toml
// and prints the city name header followed by agent names. When dirFilter
// is non-empty, only agents whose Dir matches are shown.
// Accepts an injected FS for testability.
func doAgentList(fs fsys.FS, cityPath, dirFilter string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
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
		}
		if a.Pool != nil {
			annotations = append(annotations, fmt.Sprintf("pool: min=%d, max=%d", a.Pool.Min, a.Pool.Max))
		}
		if len(annotations) > 0 {
			fmt.Fprintf(stdout, "  %s  (%s)\n", displayName, strings.Join(annotations, ", ")) //nolint:errcheck // best-effort stdout
		} else {
			fmt.Fprintf(stdout, "  %s\n", displayName) //nolint:errcheck // best-effort stdout
		}
	}
	return 0
}
