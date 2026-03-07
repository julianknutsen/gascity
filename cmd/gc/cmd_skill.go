package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed skills/*.md
var skillContents embed.FS

// skillTopics defines the available skill topics with their descriptions.
// Used by both the gc skill command (content lookup) and stub materialization.
var skillTopics = []struct {
	Name string // skill name, e.g. "gc-work"
	Desc string // frontmatter description
	Arg  string // gc skill argument, e.g. "work"
}{
	{"gc-work", "Work items (beads) — create tasks, find available work, claim assignments, update progress, close completed beads, show dependency graphs. Use whenever you need to make a new task, see what needs doing, pick up an assignment, track progress on a bead, finish work, or understand what blocks what.", "work"},
	{"gc-dispatch", "Dispatching and routing work — assign beads to agents or pools with gc sling, instantiate formulas to create molecules, group work into convoys, land completed convoys, set up automations. Use whenever you need to send work to someone, start a workflow, track a batch of tasks, or automate recurring work.", "dispatch"},
	{"gc-agents", "Agent management and communication — list agents, check status, peek at output, view session logs, nudge agents, attach to sessions, hand off context between agents, suspend, resume, drain, kill, start or destroy multi-instances. Use whenever you need to see what agents are doing, talk to an agent, read their logs, transfer work to another agent, or control an agent's lifecycle.", "agents"},
	{"gc-rigs", "Rig (project directory) management — register directories, list rigs, check rig status and health, suspend or resume rigs, restart rig agents. Use whenever you need to add a project, see which projects are registered, check a rig's agents, or pause and unpause a rig.", "rigs"},
	{"gc-mail", "Inter-agent messaging — send mail, reply to messages, check inbox, read and peek at mail, view conversation threads, archive or delete messages. Use whenever you need to communicate with another agent, check for new messages, read your mail, follow a conversation, or clean up old messages.", "mail"},
	{"gc-city", "City lifecycle — initialize workspace, start and stop the city, restart, check status, suspend and resume, show and explain config, run diagnostics, tail events, manage packs. Use whenever you need to bring up or shut down the city, see what is running, troubleshoot issues, or inspect configuration.", "city"},
	{"gc-dashboard", "Web monitoring dashboard — start the browser UI, connect to the API server, view convoys, agents, mail, events, health, and work items in real time. Use whenever you want a visual overview of city activity or need to configure the dashboard.", "dashboard"},
	{"gc-sessions", "Chat session management — create persistent conversations from agent templates, list sessions, attach to or resume sessions, suspend and close sessions, rename sessions, prune old ones, peek at output. Use whenever you need to start a new conversation, switch between sessions, resume where you left off, or clean up stale sessions.", "sessions"},
}

func newSkillCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "skill [topic]",
		Short: "Show command reference for a topic",
		Long: `Show curated command reference for a Gas City topic.

Without arguments, lists available topics. With a topic name,
prints the full command reference for that topic.`,
		Example: `  gc skill work       # beads command reference
  gc skill dispatch   # sling and formula reference
  gc skill            # list all topics`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				listSkillTopics(stdout)
				return nil
			}
			topic := args[0]
			content, err := fs.ReadFile(skillContents, "skills/"+topic+".md")
			if err != nil {
				fmt.Fprintf(stderr, "gc skill: unknown topic %q\n", topic) //nolint:errcheck // best-effort stderr
				fmt.Fprintln(stderr, "Available topics:")                  //nolint:errcheck // best-effort stderr
				listSkillTopics(stderr)
				return errExit
			}
			fmt.Fprint(stdout, string(content)) //nolint:errcheck // best-effort stdout
			return nil
		},
	}
}

// listSkillTopics prints available skill topics with descriptions.
func listSkillTopics(w io.Writer) {
	// Sort by arg name for stable output.
	sorted := make([]struct{ Arg, Desc string }, len(skillTopics))
	for i, t := range skillTopics {
		sorted[i] = struct{ Arg, Desc string }{t.Arg, t.Desc}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Arg < sorted[j].Arg
	})
	// Find max arg length for alignment.
	maxLen := 0
	for _, t := range sorted {
		if len(t.Arg) > maxLen {
			maxLen = len(t.Arg)
		}
	}
	for _, t := range sorted {
		pad := strings.Repeat(" ", maxLen-len(t.Arg))
		fmt.Fprintf(w, "  %s%s  %s\n", t.Arg, pad, t.Desc) //nolint:errcheck // best-effort
	}
}
