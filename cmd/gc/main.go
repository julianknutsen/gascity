// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/beads"
	beadsexec "github.com/gastownhall/gascity/internal/beads/exec"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/telemetry"
	"github.com/spf13/cobra"
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
	// Initialize OTel telemetry (opt-in via GC_OTEL_METRICS_URL / GC_OTEL_LOGS_URL).
	provider, err := telemetry.Init(context.Background(), "gascity", version)
	if err != nil {
		fmt.Fprintf(stderr, "gc: telemetry init: %v\n", err) //nolint:errcheck // best-effort stderr
	}
	if provider != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = provider.Shutdown(ctx)
		}()
		telemetry.SetProcessOTELAttrs()
	}

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
			// Lazy fallback: if eager discovery missed a pack command
			// (e.g. config changed after binary started), try one more time.
			if tryPackCommandFallback(args, stdout, stderr) {
				return nil
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
		newRestartCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newSuspendCmd(stdout, stderr),
		newResumeCmd(stdout, stderr),
		newRigCmd(stdout, stderr),
		newMailCmd(stdout, stderr),
		newAgentCmd(stdout, stderr),
		newEventCmd(stdout, stderr),
		newEventsCmd(stdout, stderr),
		newAutomationCmd(stdout, stderr),
		newConfigCmd(stdout, stderr),
		newPackCmd(stdout, stderr),
		newDoctorCmd(stdout, stderr),
		newHookCmd(stdout, stderr),
		newSlingCmd(stdout, stderr),
		newConvoyCmd(stdout, stderr),
		newPrimeCmd(stdout, stderr),
		newHandoffCmd(stdout, stderr),
		newDaemonCmd(stdout, stderr),
		newBeadsCmd(stdout, stderr),
		newBuildImageCmd(stdout, stderr),
		newSkillCmd(stdout, stderr),
		newVersionCmd(stdout),
		newDashboardCmd(stdout, stderr),
		newGraphCmd(stdout, stderr),
		newRegisterCmd(stdout, stderr),
		newUnregisterCmd(stdout, stderr),
		newCitiesCmd(stdout, stderr),
		newSupervisorCmd(stdout, stderr),
		newSessionCmd(stdout, stderr),
		newMigrationCmd(stdout, stderr),
	)
	// gen-doc needs the root command to walk the tree; add after construction.
	root.AddCommand(newGenDocCmd(stdout, stderr, root))

	// Best-effort: discover pack CLI commands if we're inside a city.
	registerPackCommands(root, stdout, stderr)

	return root
}

// sessionName returns the session name for a city agent.
// Delegates to agent.SessionNameFor — the single source of truth.
// sessionTemplate is a Go text/template string (empty = default pattern).
//
// When running inside a container (Docker/K8s), the tmux session has a
// fixed name ("agent" or "main") that differs from the controller's
// session name. GC_TMUX_SESSION overrides the resolved name so agent-side
// commands (drain-check, drain-ack, request-restart) target the correct
// tmux session for metadata reads/writes.
func sessionName(cityName, agentName, sessionTemplate string) string {
	if override := os.Getenv("GC_TMUX_SESSION"); override != "" {
		return override
	}
	return agent.SessionNameFor(cityName, agentName, sessionTemplate)
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
	store, err := openCityStoreAt(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	return store, 0
}

// openCityStoreAt opens a bead store at the given city path.
// Used by the controller (which already knows the city path) and by
// openCityStore (which resolves the path first).
func openCityStoreAt(cityPath string) (beads.Store, error) {
	provider := rawBeadsProvider(cityPath)
	if strings.HasPrefix(provider, "exec:") {
		return beadsexec.NewStore(strings.TrimPrefix(provider, "exec:")), nil
	}
	switch provider {
	case "file":
		store, err := beads.OpenFileStore(fsys.OSFS{}, filepath.Join(cityPath, ".gc", "beads.json"))
		if err != nil {
			return nil, err
		}
		return store, nil
	default: // "bd" or unrecognized → use bd
		if _, err := exec.LookPath("bd"); err != nil {
			return nil, fmt.Errorf("bd not found in PATH (install beads or set GC_BEADS=file)")
		}
		return beads.NewBdStore(cityPath, beads.ExecCommandRunner()), nil
	}
}
