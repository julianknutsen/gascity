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
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/fsys"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// errExit is a sentinel error returned by cobra RunE functions to signal
// non-zero exit. The command has already written its own error to stderr.
var errExit = errors.New("exit")

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
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		newStartCmd(stdout, stderr),
		newInitCmd(stdout, stderr),
		newStopCmd(stdout, stderr),
		newRigCmd(stdout, stderr),
		newBeadCmd(stdout, stderr),
		newMailCmd(stdout, stderr),
		newAgentCmd(stdout, stderr),
		newVersionCmd(stdout),
	)
	return root
}

// sessionName returns the tmux session name for a city agent.
func sessionName(cityName, agentName string) string {
	return "gc-" + cityName + "-" + agentName
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

// openCityStore locates the city root from the current directory and opens a
// Store using the configured provider. On error it writes to stderr and returns
// nil plus an exit code.
func openCityStore(stderr io.Writer, cmdName string) (beads.Store, int) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	cityPath, err := findCity(cwd)
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
