package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

func newStopCmd(stdout, stderr io.Writer) *cobra.Command {
	var clean bool
	cmd := &cobra.Command{
		Use:   "stop [path]",
		Short: "Stop all agent sessions in the city",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdStop(args, clean, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clean, "clean", false,
		"remove agent worktrees (default: preserve for restart)")
	return cmd
}

// cmdStop stops the city by terminating all configured agent sessions.
// If a path is given, operates there; otherwise uses cwd.
func cmdStop(args []string, clean bool, stdout, stderr io.Writer) int {
	var dir string
	var err error
	switch {
	case len(args) > 0:
		dir, err = filepath.Abs(args[0])
	case cityFlag != "":
		dir, err = filepath.Abs(cityFlag)
	default:
		dir, err = os.Getwd()
	}
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	// If a controller is running, ask it to shut down (it stops agents).
	if tryStopController(cityPath, stdout) {
		// Controller handled the shutdown — still stop dolt below.
		if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
			if err := dolt.StopCity(cityPath); err != nil {
				fmt.Fprintf(stderr, "gc stop: dolt: %v\n", err) //nolint:errcheck // best-effort stderr
			}
		}
		return 0
	}

	sp := newSessionProvider()
	st := cfg.Workspace.SessionTemplate
	var agents []agent.Agent
	desired := make(map[string]bool, len(cfg.Agents))
	for _, a := range cfg.Agents {
		pool := a.EffectivePool()
		qn := a.QualifiedName()
		if pool.Max <= 1 {
			// Single agent.
			agents = append(agents, agent.New(qn, cityName, "", "", nil, agent.StartupHints{}, "", st, sp))
			desired[agent.SessionNameFor(cityName, qn, st)] = true
		} else {
			// Pool agent: generate {name}-1 through {name}-{max}.
			for i := 1; i <= pool.Max; i++ {
				instanceName := fmt.Sprintf("%s-%d", a.Name, i)
				qualifiedInstance := instanceName
				if a.Dir != "" {
					qualifiedInstance = a.Dir + "/" + instanceName
				}
				agents = append(agents, agent.New(qualifiedInstance, cityName, "", "", nil, agent.StartupHints{}, "", st, sp))
				desired[agent.SessionNameFor(cityName, qualifiedInstance, st)] = true
			}
		}
	}
	recorder := events.Discard
	if fr, err := events.NewFileRecorder(
		filepath.Join(cityPath, ".gc", "events.jsonl"), stderr); err == nil {
		recorder = fr
	}

	code := doStop(agents, sp, cfg.Daemon.ShutdownTimeoutDuration(), recorder, stdout, stderr)

	// Clean up orphan sessions (sessions with the city prefix that are
	// not in the current config).
	cityPrefix := "gc-" + cityName + "-"
	rops := newReconcileOps(sp)
	doStopOrphans(sp, rops, desired, cityPrefix, cfg.Daemon.ShutdownTimeoutDuration(), recorder, stdout, stderr)

	// Clean up worktrees only when --clean is set. By default worktrees
	// persist so agents can resume work after gc start (like gt down).
	if clean {
		cleanupWorktrees(cityPath, cfg.Rigs, stderr)
	}

	// Stop dolt server after agents.
	if beadsProvider(cityPath) == "bd" && os.Getenv("GC_DOLT") != "skip" {
		if err := dolt.StopCity(cityPath); err != nil {
			fmt.Fprintf(stderr, "gc stop: dolt: %v\n", err) //nolint:errcheck // best-effort stderr
			// Non-fatal warning.
		}
	}

	return code
}

// tryStopController connects to .gc/controller.sock and sends "stop".
// Returns true if a controller acknowledged the shutdown. If no controller
// is running (socket doesn't exist or connection refused), returns false.
func tryStopController(cityPath string, stdout io.Writer) bool {
	sockPath := filepath.Join(cityPath, ".gc", "controller.sock")
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()                                     //nolint:errcheck // best-effort cleanup
	conn.Write([]byte("stop\n"))                           //nolint:errcheck // best-effort
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck // best-effort
	buf := make([]byte, 64)
	conn.Read(buf)                                 //nolint:errcheck // best-effort
	fmt.Fprintln(stdout, "Controller stopping...") //nolint:errcheck // best-effort stdout
	return true
}

// doStop is the pure logic for "gc stop". It collects running agents and
// performs graceful shutdown (interrupt → wait → kill). Accepts pre-built
// agents, provider, timeout, and recorder for testability.
func doStop(agents []agent.Agent, sp session.Provider, timeout time.Duration,
	rec events.Recorder, stdout, stderr io.Writer,
) int {
	var names []string
	for _, a := range agents {
		if a.IsRunning() {
			names = append(names, a.SessionName())
		}
	}
	gracefulStopAll(names, sp, timeout, rec, stdout, stderr)
	fmt.Fprintln(stdout, "City stopped.") //nolint:errcheck // best-effort stdout
	return 0
}
