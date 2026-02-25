package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// defaultPrimePrompt is the run-once worker prompt output when no agent name
// matches a configured agent. This is for users who start Claude Code manually
// inside a rig without being a managed agent.
const defaultPrimePrompt = `# Gas City Agent

You are an agent in a Gas City workspace. Check for available work
and execute it.

## Your tools

- ` + "`bd ready`" + ` — see available work items
- ` + "`bd show <id>`" + ` — see details of a work item
- ` + "`bd close <id>`" + ` — mark work as done

## How to work

1. Check for available work: ` + "`bd ready`" + `
2. Pick a bead and execute the work described in its title
3. When done, close it: ` + "`bd close <id>`" + `
4. Check for more work. Repeat until the queue is empty.
`

// newPrimeCmd creates the "gc prime [agent-name]" command.
func newPrimeCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prime [agent-name]",
		Short: "Output the behavioral prompt for an agent",
		Long: `Outputs the behavioral prompt for an agent.

Use it to prime any CLI coding agent with city-aware instructions:
  claude "$(gc prime mayor)"
  codex --prompt "$(gc prime worker)"

If agent-name matches a configured agent with a prompt_template,
that template is output. Otherwise outputs a default worker prompt.`,
		Args: cobra.MaximumNArgs(1),
	}
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		if doPrime(args, stdout, stderr) != 0 {
			return errExit
		}
		return nil
	}
	return cmd
}

// doPrime is the pure logic for "gc prime". Looks up the agent name in
// city.toml and outputs the corresponding prompt template. Falls back to
// the default run-once prompt if no match is found or no city exists.
func doPrime(args []string, stdout, _ io.Writer) int { //nolint:unparam // always returns 0 by design (graceful fallback)
	agentName := ""
	if len(args) > 0 {
		agentName = args[0]
	}

	// Try to find city and load config.
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprint(stdout, defaultPrimePrompt) //nolint:errcheck // best-effort stdout
		return 0
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprint(stdout, defaultPrimePrompt) //nolint:errcheck // best-effort stdout
		return 0
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	// Look up agent in config. First try qualified identity resolution
	// (handles "rig/agent" and rig-context matching), then fall back to
	// bare template name lookup (handles "gc prime polecat" for pool agents
	// whose config name is "polecat" regardless of dir).
	if agentName != "" {
		a, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
		if !ok {
			a, ok = findAgentByName(cfg, agentName)
		}
		if ok && a.PromptTemplate != "" {
			ctx := buildPrimeContext(cityPath, &a, cfg.Rigs)
			prompt := renderPrompt(fsys.OSFS{}, cityPath, cityName, a.PromptTemplate, ctx, cfg.Workspace.SessionTemplate, io.Discard)
			if prompt != "" {
				fmt.Fprint(stdout, prompt) //nolint:errcheck // best-effort stdout
				return 0
			}
		}
	}

	// Fallback: default run-once prompt.
	fmt.Fprint(stdout, defaultPrimePrompt) //nolint:errcheck // best-effort stdout
	return 0
}

// findAgentByName looks up an agent by its bare config name, ignoring dir.
// This allows "gc prime polecat" to find an agent with name="polecat" even
// when it has dir="myrig". Returns the first match.
func findAgentByName(cfg *config.City, name string) (config.Agent, bool) {
	for _, a := range cfg.Agents {
		if a.Name == name {
			return a, true
		}
	}
	return config.Agent{}, false
}

// buildPrimeContext constructs a PromptContext for gc prime. Uses GC_*
// environment variables when running inside a managed session, falls back
// to currentRigContext when run manually.
func buildPrimeContext(cityPath string, a *config.Agent, rigs []config.Rig) PromptContext {
	ctx := PromptContext{
		CityRoot:     cityPath,
		TemplateName: a.Name,
		Env:          a.Env,
	}

	// Agent identity: prefer GC_AGENT env (managed session), else config.
	if gcAgent := os.Getenv("GC_AGENT"); gcAgent != "" {
		ctx.AgentName = gcAgent
	} else {
		ctx.AgentName = a.QualifiedName()
	}

	// Working directory.
	if gcDir := os.Getenv("GC_DIR"); gcDir != "" {
		ctx.WorkDir = gcDir
	}

	// Rig context.
	if gcRig := os.Getenv("GC_RIG"); gcRig != "" {
		ctx.RigName = gcRig
		ctx.IssuePrefix = findRigPrefix(gcRig, rigs)
	} else if a.Dir != "" {
		ctx.RigName = a.Dir
		ctx.IssuePrefix = findRigPrefix(a.Dir, rigs)
	}

	ctx.Branch = os.Getenv("GC_BRANCH")
	ctx.WorkQuery = a.EffectiveWorkQuery()
	return ctx
}
