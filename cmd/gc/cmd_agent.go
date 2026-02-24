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
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

// findAgentInConfig looks up an agent name in [[agents]].
// For pool agents with Max > 1, matches {name}-{N} patterns.
// Returns the matching Agent config and true if found, or zero Agent and false.
func findAgentInConfig(cfg *config.City, name string) (config.Agent, bool) {
	for _, a := range cfg.Agents {
		if a.Name == name {
			return a, true
		}
		// Pool agent with Max > 1: match {name}-{N} pattern.
		if a.Pool != nil && a.Pool.Max > 1 {
			prefix := a.Name + "-"
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			suffix := name[len(prefix):]
			n, err := strconv.Atoi(suffix)
			if err != nil || n < 1 || n > a.Pool.Max {
				continue
			}
			// Return a copy with the instance name.
			instance := a
			instance.Name = name
			instance.Pool = nil // instances are not pools
			return instance, true
		}
	}
	return config.Agent{}, false
}

func newAgentCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc agent: missing subcommand (add, attach, drain, drain-check, hook, list, undrain)") //nolint:errcheck // best-effort stderr
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
		newAgentDrainCheckCmd(stdout, stderr),
		newAgentHookCmd(stdout, stderr),
		newAgentListCmd(stdout, stderr),
		newAgentUndrainCmd(stdout, stderr),
	)
	return cmd
}

func newAgentAddCmd(stdout, stderr io.Writer) *cobra.Command {
	var name, promptTemplate string
	cmd := &cobra.Command{
		Use:   "add --name <name>",
		Short: "Add an agent to the workspace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentAdd(name, promptTemplate, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name of the agent")
	cmd.Flags().StringVar(&promptTemplate, "prompt-template", "", "Path to prompt template file (relative to city root)")
	return cmd
}

func newAgentListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workspace agents",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdAgentList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
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

func newAgentHookCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "hook <agent-name> <bead-id>",
		Short: "Hook a bead to an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentHook(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentHook is the CLI entry point for hooking a bead to an agent. It
// validates the agent exists in city.toml, opens the bead store, and
// delegates to doAgentHook.
func cmdAgentHook(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc agent hook: usage: gc agent hook <agent-name> <bead-id>") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]
	beadID := args[1]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent hook: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent hook: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent hook: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent hook: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	store, code := openCityStore(stderr, "gc agent hook")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doAgentHook(store, rec, agentName, beadID, stdout, stderr)
}

// doAgentHook hooks a bead to an agent. Accepts an injected store and
// recorder for testability.
func doAgentHook(store beads.Store, rec events.Recorder, agentName, beadID string, stdout, stderr io.Writer) int {
	err := store.Hook(beadID, agentName)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent hook: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.BeadHooked,
		Actor:   eventActor(),
		Subject: beadID,
		Message: agentName,
	})
	fmt.Fprintf(stdout, "Hooked bead '%s' to agent '%s'\n", beadID, agentName) //nolint:errcheck // best-effort stdout
	return 0
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

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent attach: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
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
	found, ok := findAgentInConfig(cfg, agentName)
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
	sn := sessionName(cityName, agentName)
	sp := newSessionProvider()
	hints := agent.StartupHints{
		ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
		ReadyDelayMs:           resolved.ReadyDelayMs,
		ProcessNames:           resolved.ProcessNames,
		EmitsPermissionWarning: resolved.EmitsPermissionWarning,
	}
	a := agent.New(cfgAgent.Name, sn, resolved.CommandString(), "", resolved.Env, hints, sp)
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
func cmdAgentAdd(name, promptTemplate string, stdout, stderr io.Writer) int {
	if name == "" {
		fmt.Fprintln(stderr, "gc agent add: missing --name flag") //nolint:errcheck // best-effort stderr
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentAdd(fsys.OSFS{}, cityPath, name, promptTemplate, stdout, stderr)
}

// doAgentAdd is the pure logic for "gc agent add". It loads city.toml,
// checks for duplicates, appends the new agent, and writes back.
// Accepts an injected FS for testability.
func doAgentAdd(fs fsys.FS, cityPath, name, promptTemplate string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent add: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	for _, a := range cfg.Agents {
		if a.Name == name {
			fmt.Fprintf(stderr, "gc agent add: agent %q already exists\n", name) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	cfg.Agents = append(cfg.Agents, config.Agent{Name: name, PromptTemplate: promptTemplate})
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

// cmdAgentList is the CLI entry point for listing agents. It locates
// the city root and delegates to doAgentList.
func cmdAgentList(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doAgentList(fsys.OSFS{}, cityPath, stdout, stderr)
}

// doAgentList is the pure logic for "gc agent list". It loads city.toml
// and prints the city name header followed by agent names.
// Accepts an injected FS for testability.
func doAgentList(fs fsys.FS, cityPath string, stdout, stderr io.Writer) int {
	tomlPath := filepath.Join(cityPath, "city.toml")
	cfg, err := config.Load(fs, tomlPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "%s:\n", cfg.Workspace.Name) //nolint:errcheck // best-effort stdout
	for _, a := range cfg.Agents {
		if a.Pool != nil {
			fmt.Fprintf(stdout, "  %s (pool: min=%d, max=%d)\n", a.Name, a.Pool.Min, a.Pool.Max) //nolint:errcheck // best-effort stdout
		} else {
			fmt.Fprintf(stdout, "  %s\n", a.Name) //nolint:errcheck // best-effort stdout
		}
	}
	return 0
}
