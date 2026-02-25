package main

import (
	"fmt"
	"io"
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
				fmt.Fprintln(stderr, "gc agent: missing subcommand (add, attach, claim, claimed, drain, drain-ack, drain-check, list, nudge, unclaim, undrain)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc agent: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newAgentAddCmd(stdout, stderr),
		newAgentAttachCmd(stdout, stderr),
		newAgentClaimCmd(stdout, stderr),
		newAgentClaimedCmd(stdout, stderr),
		newAgentDrainCmd(stdout, stderr),
		newAgentDrainAckCmd(stdout, stderr),
		newAgentDrainCheckCmd(stdout, stderr),
		newAgentListCmd(stdout, stderr),
		newAgentNudgeCmd(stdout, stderr),
		newAgentUnclaimCmd(stdout, stderr),
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

func newAgentClaimCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "claim <agent-name> <bead-id>",
		Short: "Claim a bead for an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentClaim(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentClaim is the CLI entry point for claiming a bead for an agent. It
// validates the agent exists in city.toml, opens the bead store, and
// delegates to doAgentClaim.
func cmdAgentClaim(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc agent claim: usage: gc agent claim <agent-name> <bead-id>") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]
	beadID := args[1]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent claim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent claim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent claim: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	store, code := openCityStore(stderr, "gc agent claim")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doAgentClaim(store, rec, agentName, beadID, stdout, stderr)
}

// doAgentClaim claims a bead for an agent. Accepts an injected store and
// recorder for testability.
func doAgentClaim(store beads.Store, rec events.Recorder, agentName, beadID string, stdout, stderr io.Writer) int {
	err := store.Claim(beadID, agentName)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent claim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.BeadClaimed,
		Actor:   eventActor(),
		Subject: beadID,
		Message: agentName,
	})
	fmt.Fprintf(stdout, "Claimed bead '%s' for agent '%s'\n", beadID, agentName) //nolint:errcheck // best-effort stdout
	return 0
}

func newAgentClaimedCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "claimed <agent-name>",
		Short: "Show the bead claimed by an agent",
		Long: `Show the bead currently claimed by the given agent.

Supported flags:
  --format text|json|toon   Output format (default: text)
  --json                    Shorthand for --format json`,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentClaimed(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentClaimed is the CLI entry point for showing the bead claimed by an
// agent. It opens the bead store in the current city and delegates to
// doAgentClaimed.
func cmdAgentClaimed(args []string, stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc agent claimed")
	if store == nil {
		return code
	}
	return doAgentClaimed(store, args, stdout, stderr)
}

// doAgentClaimed shows the bead currently claimed by the given agent. Output
// format matches bd show. Accepts an injected store for testability.
func doAgentClaimed(store beads.Store, args []string, stdout, stderr io.Writer) int {
	format, args := parseBeadFormat(args)
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent claimed: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	b, err := store.Claimed(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gc agent claimed: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	switch format {
	case "json":
		writeBeadJSON(b, stdout)
	case "toon":
		writeBeadTOON(b, stdout)
	default:
		w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
		w(fmt.Sprintf("ID:       %s", b.ID))
		w(fmt.Sprintf("Status:   %s", b.Status))
		w(fmt.Sprintf("Type:     %s", b.Type))
		w(fmt.Sprintf("Title:    %s", b.Title))
		w(fmt.Sprintf("Created:  %s", b.CreatedAt.Format("2006-01-02 15:04:05")))
		w(fmt.Sprintf("Assignee: %s", b.Assignee))
	}
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
	sp := newSessionProvider()
	hints := agent.StartupHints{
		ReadyPromptPrefix:      resolved.ReadyPromptPrefix,
		ReadyDelayMs:           resolved.ReadyDelayMs,
		ProcessNames:           resolved.ProcessNames,
		EmitsPermissionWarning: resolved.EmitsPermissionWarning,
	}
	a := agent.New(cfgAgent.Name, cityName, resolved.CommandString(), "", resolved.Env, hints, "", sp)
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
	cityPath, err := resolveCity()
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

func newAgentUnclaimCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "unclaim <agent-name> <bead-id>",
		Short: "Release a bead from an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentUnclaim(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// cmdAgentUnclaim is the CLI entry point for releasing a bead from an agent.
// It validates the agent exists in city.toml, opens the bead store, and
// delegates to doAgentUnclaim.
func cmdAgentUnclaim(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc agent unclaim: usage: gc agent unclaim <agent-name> <bead-id>") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]
	beadID := args[1]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent unclaim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc agent unclaim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Validate agent exists in config.
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent unclaim: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	store, code := openCityStore(stderr, "gc agent unclaim")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doAgentUnclaim(store, rec, agentName, beadID, stdout, stderr)
}

// doAgentUnclaim releases a bead from an agent. Accepts an injected store
// and recorder for testability.
func doAgentUnclaim(store beads.Store, rec events.Recorder, agentName, beadID string, stdout, stderr io.Writer) int {
	err := store.Unclaim(beadID, agentName)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent unclaim: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	rec.Record(events.Event{
		Type:    events.BeadUnclaimed,
		Actor:   eventActor(),
		Subject: beadID,
		Message: agentName,
	})
	fmt.Fprintf(stdout, "Unclaimed bead '%s' from agent '%s'\n", beadID, agentName) //nolint:errcheck // best-effort stdout
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
	if _, found := findAgentInConfig(cfg, agentName); !found {
		fmt.Fprintf(stderr, "gc agent nudge: agent %q not found in city.toml\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Resolve session name and construct a minimal Agent.
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	sp := newSessionProvider()
	a := agent.New(agentName, cityName, "", "", nil, agent.StartupHints{}, "", sp)
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

// cmdAgentList is the CLI entry point for listing agents. It locates
// the city root and delegates to doAgentList.
func cmdAgentList(stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
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
