package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

// acquireControllerLock takes an exclusive flock on .gc/controller.lock.
// Returns the locked file (caller must defer Close) or an error if another
// controller is already running.
func acquireControllerLock(cityPath string) (*os.File, error) {
	path := filepath.Join(cityPath, ".gc", "controller.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening controller lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close() //nolint:errcheck // closing after flock failure
		return nil, fmt.Errorf("controller already running")
	}
	return f, nil
}

// startControllerSocket listens on a Unix socket at .gc/controller.sock.
// When a client sends "stop\n", cancelFn is called to shut down the
// controller loop. Returns the listener for cleanup.
func startControllerSocket(cityPath string, cancelFn context.CancelFunc) (net.Listener, error) {
	sockPath := filepath.Join(cityPath, ".gc", "controller.sock")
	// Remove stale socket from a previous crash.
	os.Remove(sockPath) //nolint:errcheck // stale socket cleanup
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on controller socket: %w", err)
	}
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return // listener closed
			}
			go handleControllerConn(conn, cancelFn)
		}
	}()
	return lis, nil
}

// handleControllerConn reads from a connection and calls cancelFn if
// the client sends "stop".
func handleControllerConn(conn net.Conn, cancelFn context.CancelFunc) {
	defer conn.Close() //nolint:errcheck // best-effort cleanup
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		if scanner.Text() == "stop" {
			cancelFn()
			conn.Write([]byte("ok\n")) //nolint:errcheck // best-effort ack
		}
	}
}

// controllerLoop runs reconciliation periodically until ctx is canceled.
// buildFn is called on each tick to re-evaluate the desired agent set
// (pool check commands are re-run).
func controllerLoop(
	ctx context.Context,
	interval time.Duration,
	buildFn func() []agent.Agent,
	sp session.Provider,
	rops reconcileOps,
	rec events.Recorder,
	prefix string,
	stdout, stderr io.Writer,
) {
	// Initial reconciliation.
	agents := buildFn()
	doReconcileAgents(agents, sp, rops, rec, prefix, stdout, stderr)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			agents = buildFn()
			doReconcileAgents(agents, sp, rops, rec, prefix, stdout, stderr)
		case <-ctx.Done():
			return
		}
	}
}

// runController runs the persistent controller loop. It acquires a lock,
// opens a control socket, runs the reconciliation loop, and on shutdown
// stops all agents. Returns an exit code.
func runController(
	cityPath string,
	cfg *config.City,
	buildFn func() []agent.Agent,
	sp session.Provider,
	rec events.Recorder,
	stdout, stderr io.Writer,
) int {
	lock, err := acquireControllerLock(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	defer lock.Close() //nolint:errcheck // best-effort cleanup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler: SIGINT/SIGTERM → cancel.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	sockPath := filepath.Join(cityPath, ".gc", "controller.sock")
	lis, err := startControllerSocket(cityPath, cancel)
	if err != nil {
		fmt.Fprintf(stderr, "gc start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	defer lis.Close()         //nolint:errcheck // best-effort cleanup
	defer os.Remove(sockPath) //nolint:errcheck // best-effort cleanup

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}
	cityPrefix := "gc-" + cityName + "-"

	rec.Record(events.Event{Type: events.ControllerStarted, Actor: "gc"})
	fmt.Fprintln(stdout, "Controller started.") //nolint:errcheck // best-effort stdout

	controllerLoop(ctx, cfg.Daemon.PatrolIntervalDuration(),
		buildFn, sp, newReconcileOps(sp), rec, cityPrefix, stdout, stderr)

	// Shutdown: stop all agents.
	for _, a := range buildFn() {
		if a.IsRunning() {
			if err := a.Stop(); err != nil {
				fmt.Fprintf(stderr, "gc stop: stopping %s: %v\n", a.Name(), err) //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stdout, "Stopped agent '%s'\n", a.Name()) //nolint:errcheck // best-effort stdout
			}
		}
	}

	rec.Record(events.Event{Type: events.ControllerStopped, Actor: "gc"})
	fmt.Fprintln(stdout, "Controller stopped.") //nolint:errcheck // best-effort stdout
	return 0
}
