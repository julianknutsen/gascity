package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/sessionlog"
	"github.com/spf13/cobra"
)

func newAgentLogsCmd(stdout, stderr io.Writer) *cobra.Command {
	var follow bool
	var tail int
	cmd := &cobra.Command{
		Use:   "logs <agent-name>",
		Short: "Show session logs for an agent",
		Long: `Show structured session log messages from an agent's Claude JSONL file.

Reads the agent's session log, resolves the conversation DAG, and prints
messages in chronological order. Use --tail to control how many compaction
segments to show (0 = all). Use -f to follow new messages as they arrive.`,
		Example: `  gc agent logs mayor
  gc agent logs mayor --tail 0
  gc agent logs myrig/polecat-1 -f`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdAgentLogs(args, follow, tail, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow new messages as they arrive")
	cmd.Flags().IntVar(&tail, "tail", 1, "Number of compaction segments to show (0 = all)")
	return cmd
}

// cmdAgentLogs is the CLI entry point for viewing agent session logs.
func cmdAgentLogs(args []string, follow bool, tail int, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc agent logs: missing agent name") //nolint:errcheck // best-effort stderr
		return 1
	}
	agentName := args[0]

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc agent logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	found, ok := resolveAgentIdentity(cfg, agentName, currentRigContext(cfg))
	if !ok {
		fmt.Fprintln(stderr, agentNotFoundMsg("gc agent logs", agentName, cfg)) //nolint:errcheck // best-effort stderr
		return 1
	}

	workDir := resolveAgentWorkDir(found, cfg, cityPath)
	if workDir == "" {
		fmt.Fprintf(stderr, "gc agent logs: cannot resolve working directory for %q\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	path := sessionlog.FindSessionFile(sessionlog.DefaultSearchPaths(), workDir)
	if path == "" {
		fmt.Fprintf(stderr, "gc agent logs: no session file found for %q\n", agentName) //nolint:errcheck // best-effort stderr
		return 1
	}

	return doAgentLogs(path, follow, tail, stdout, stderr)
}

// resolveAgentWorkDir returns the absolute working directory for an agent.
// For rig-scoped agents, this is the rig's Path. For city-scoped agents,
// this is the city root.
func resolveAgentWorkDir(a config.Agent, cfg *config.City, cityPath string) string {
	if a.Dir == "" {
		return cityPath
	}
	for _, rig := range cfg.Rigs {
		if rig.Name == a.Dir {
			return rig.Path
		}
	}
	return ""
}

// doAgentLogs reads the session file and prints messages. If follow is true,
// it polls for new messages every 2 seconds.
func doAgentLogs(path string, follow bool, tail int, stdout, stderr io.Writer) int {
	sess, err := sessionlog.ReadFile(path, tail)
	if err != nil {
		fmt.Fprintf(stderr, "gc agent logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	seen := make(map[string]bool)
	for _, msg := range sess.Messages {
		printLogEntry(stdout, msg)
		seen[msg.UUID] = true
	}

	if !follow {
		return 0
	}

	// Follow mode: poll every 2 seconds for new messages.
	for {
		time.Sleep(2 * time.Second)

		sess, err = sessionlog.ReadFile(path, tail)
		if err != nil {
			// File might be temporarily unavailable during writes; skip.
			continue
		}

		for _, msg := range sess.Messages {
			if seen[msg.UUID] {
				continue
			}
			printLogEntry(stdout, msg)
			seen[msg.UUID] = true
		}
	}
}

// resolveMessage handles both message formats found in Claude JSONL files:
// object format: {"role":"user","content":"hello"}
// string format: "{\"role\":\"user\",\"content\":\"hello\"}" (escaped JSON string)
// Returns the message content struct if parseable.
func resolveMessage(raw json.RawMessage) *sessionlog.MessageContent {
	if len(raw) == 0 {
		return nil
	}
	// Try object format first.
	var mc sessionlog.MessageContent
	if err := json.Unmarshal(raw, &mc); err == nil && mc.Role != "" {
		return &mc
	}
	// Try string format (JSON-encoded string containing the object).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if err := json.Unmarshal([]byte(s), &mc); err == nil && mc.Role != "" {
			return &mc
		}
	}
	return nil
}

// printLogEntry prints a single session log entry to stdout.
func printLogEntry(w io.Writer, e *sessionlog.Entry) {
	if e.IsCompactBoundary() {
		fmt.Fprintln(w, "── context compacted ──") //nolint:errcheck
		return
	}

	// Timestamp prefix.
	ts := ""
	if !e.Timestamp.IsZero() {
		ts = e.Timestamp.Format("15:04:05") + " "
	}

	// Type badge.
	typeStr := strings.ToUpper(e.Type)

	mc := resolveMessage(e.Message)
	if mc == nil {
		// Unparseable message — print raw truncated.
		if len(e.Message) > 0 {
			raw := string(e.Message)
			if len(raw) > 200 {
				raw = raw[:200] + "..."
			}
			fmt.Fprintf(w, "%s[%s] %s\n", ts, typeStr, raw) //nolint:errcheck
		}
		return
	}

	// Try content as plain string.
	var text string
	if json.Unmarshal(mc.Content, &text) == nil && text != "" {
		fmt.Fprintf(w, "%s[%s] %s\n", ts, typeStr, text) //nolint:errcheck
		return
	}

	// Try content as array of blocks.
	var blocks []sessionlog.ContentBlock
	if json.Unmarshal(mc.Content, &blocks) == nil && len(blocks) > 0 {
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					fmt.Fprintf(w, "%s[%s] %s\n", ts, typeStr, b.Text) //nolint:errcheck
				}
			case "tool_use":
				fmt.Fprintf(w, "%s[%s] tool_use: %s\n", ts, typeStr, b.Name) //nolint:errcheck
			case "tool_result":
				if b.IsError {
					fmt.Fprintf(w, "%s[%s] tool_result: error\n", ts, typeStr) //nolint:errcheck
				}
			}
		}
		return
	}

	// Fallback: print raw content truncated.
	raw := string(mc.Content)
	if len(raw) > 200 {
		raw = raw[:200] + "..."
	}
	fmt.Fprintf(w, "%s[%s] %s\n", ts, typeStr, raw) //nolint:errcheck
}
