// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// errExit is a sentinel error returned by cobra RunE functions to signal
// non-zero exit. The command has already written its own error to stderr.
var errExit = errors.New("exit")

// cityFlag holds the value of the --city persistent flag.
// Empty means "discover from cwd."
var cityFlag string

// run executes the gc CLI with the given args, writing output to stdout and
// errors to stderr. Returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

// newRootCmd creates the root cobra command with all subcommands.
func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "gc",
		Short:         "Gas City CLI — orchestration-builder for multi-agent workflows",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			fmt.Fprintf(stderr, "gc: unknown command %q\n", args[0]) //nolint:errcheck // best-effort stderr
			return errExit
		},
	}
	root.PersistentFlags().StringVar(&cityFlag, "city", "",
		"path to the city directory (default: walk up from cwd)")
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		newStartCmd(stdout, stderr),
		newInitCmd(stdout, stderr),
		newStopCmd(stdout, stderr),
		newRigCmd(stdout, stderr),
		newMailCmd(stdout, stderr),
		newAgentCmd(stdout, stderr),
		newEventsCmd(stdout, stderr),
		newFormulaCmd(stdout, stderr),
		newMolCmd(stdout, stderr),
		newPrimeCmd(stdout, stderr),
		newVersionCmd(stdout),
	)
	return root
}

// sessionName returns the session name for a city agent.
// Delegates to agent.SessionNameFor — the single source of truth.
func sessionName(cityName, agentName string) string {
	return agent.SessionNameFor(cityName, agentName)
}

// findCity walks dir upward looking for a directory containing .gc/.
// Returns the city root path or an error.
func findCity(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".gc")); err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a city directory (no .gc/ found)")
		}
		dir = parent
	}
}

// resolveCity returns the city root path. If --city was provided, it
// verifies .gc/ exists there. Otherwise falls back to os.Getwd() →
// findCity().
func resolveCity() (string, error) {
	if cityFlag != "" {
		p, err := filepath.Abs(cityFlag)
		if err != nil {
			return "", err
		}
		if fi, err := os.Stat(filepath.Join(p, ".gc")); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("not a city directory: %s (no .gc/ found)", p)
		}
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findCity(cwd)
}

// openCityRecorder returns a Recorder that appends to .gc/events.jsonl in the
// current city. Returns events.Discard on any error — commands always get a
// valid recorder.
func openCityRecorder(stderr io.Writer) events.Recorder {
	cityPath, err := resolveCity()
	if err != nil {
		return events.Discard
	}
	rec, err := events.NewFileRecorder(
		filepath.Join(cityPath, ".gc", "events.jsonl"), stderr)
	if err != nil {
		return events.Discard
	}
	return rec
}

// eventActor returns the actor identity for events. If the GC_AGENT env var
// is set (agent session), it returns the agent name; otherwise "human".
func eventActor() string {
	if a := os.Getenv("GC_AGENT"); a != "" {
		return a
	}
	return "human"
}

// openCityStore locates the city root from the current directory and opens a
// Store using the configured provider. On error it writes to stderr and returns
// nil plus an exit code.
func openCityStore(stderr io.Writer, cmdName string) (beads.Store, int) {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}

	provider := beadsProvider(cityPath)
	switch provider {
	case "file":
		store, err := beads.OpenFileStore(fsys.OSFS{}, filepath.Join(cityPath, ".gc", "beads.json"))
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
			return nil, 1
		}
		return store, 0
	default: // "bd" or unrecognized → use bd
		if _, err := exec.LookPath("bd"); err != nil {
			fmt.Fprintf(stderr, "%s: bd not found in PATH (install beads or set GC_BEADS=file)\n", cmdName) //nolint:errcheck // best-effort stderr
			return nil, 1
		}
		return beads.NewBdStore(cityPath, beads.ExecCommandRunner()), 0
	}
}
