package main

import (
	"fmt"
	"io"
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

	// Look up agent in config.
	if agentName != "" {
		if a, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg)); ok && a.PromptTemplate != "" {
			prompt := readPromptFile(fsys.OSFS{}, cityPath, a.PromptTemplate)
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
