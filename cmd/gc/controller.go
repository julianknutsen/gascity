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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/fsys"
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

// watchConfig starts an fsnotify watcher on city.toml and sets dirty to true
// on any filesystem event. Returns a cleanup function. If the watcher cannot
// be created, returns a no-op cleanup (degraded to tick-only, no file watching).
func watchConfig(tomlPath string, dirty *atomic.Bool, stderr io.Writer) func() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(stderr, "gc start: config watcher: %v (reload on tick only)\n", err) //nolint:errcheck // best-effort stderr
		return func() {}
	}
	if err := watcher.Add(tomlPath); err != nil {
		fmt.Fprintf(stderr, "gc start: config watcher: %v (reload on tick only)\n", err) //nolint:errcheck // best-effort stderr
		watcher.Close()                                                                  //nolint:errcheck // closing after setup failure
		return func() {}
	}
	go func() {
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				dirty.Store(true)
				// Re-watch after Rename (vim atomic save).
				if ev.Has(fsnotify.Rename) || ev.Has(fsnotify.Remove) {
					_ = watcher.Add(tomlPath)
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return func() { watcher.Close() } //nolint:errcheck // best-effort cleanup
}

// tryReloadConfig attempts to reload city.toml. Returns the new config on
// success, or nil with an error on failure (parse error, validation error,
// cityName changed). Callers should keep the old config on nil.
func tryReloadConfig(tomlPath, lockedCityName string) (*config.City, error) {
	newCfg, err := config.Load(fsys.OSFS{}, tomlPath)
	if err != nil {
		return nil, fmt.Errorf("parsing city.toml: %w", err)
	}
	if err := config.ValidateAgents(newCfg.Agents); err != nil {
		return nil, fmt.Errorf("validating agents: %w", err)
	}
	newName := newCfg.Workspace.Name
	if newName == "" {
		newName = filepath.Base(filepath.Dir(tomlPath))
	}
	if newName != lockedCityName {
		return nil, fmt.Errorf("workspace.name changed from %q to %q (restart controller to apply)", lockedCityName, newName)
	}
	return newCfg, nil
}

// gracefulStopAll performs two-pass graceful shutdown:
//  1. Send Interrupt (Ctrl-C) to all sessions
//  2. Wait shutdown_timeout
//  3. Stop (force-kill) any survivors
func gracefulStopAll(
	names []string,
	sp session.Provider,
	timeout time.Duration,
	rec events.Recorder,
	stdout, stderr io.Writer,
) {
	if timeout <= 0 || len(names) == 0 {
		// Immediate kill (no grace period).
		for _, name := range names {
			if err := sp.Stop(name); err != nil {
				fmt.Fprintf(stderr, "gc stop: stopping %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stdout, "Stopped agent '%s'\n", name) //nolint:errcheck // best-effort stdout
				rec.Record(events.Event{
					Type: events.AgentStopped, Actor: "gc", Subject: name,
				})
			}
		}
		return
	}

	// Pass 1: interrupt all.
	for _, name := range names {
		_ = sp.Interrupt(name) // best-effort
	}
	fmt.Fprintf(stdout, "Sent interrupt to %d agent(s), waiting %s...\n", //nolint:errcheck // best-effort stdout
		len(names), timeout)

	time.Sleep(timeout)

	// Pass 2: kill survivors.
	for _, name := range names {
		if !sp.IsRunning(name) {
			fmt.Fprintf(stdout, "Agent '%s' exited gracefully\n", name) //nolint:errcheck // best-effort stdout
			rec.Record(events.Event{
				Type: events.AgentStopped, Actor: "gc", Subject: name,
			})
			continue
		}
		if err := sp.Stop(name); err != nil {
			fmt.Fprintf(stderr, "gc stop: stopping %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
		} else {
			fmt.Fprintf(stdout, "Stopped agent '%s'\n", name) //nolint:errcheck // best-effort stdout
			rec.Record(events.Event{
				Type: events.AgentStopped, Actor: "gc", Subject: name,
			})
		}
	}
}

// controllerLoop runs reconciliation periodically until ctx is canceled.
// buildFn is called on each tick to re-evaluate the desired agent set
// (pool check commands are re-run). If tomlPath is non-empty, the loop
// watches city.toml for changes and reloads config on the next tick.
func controllerLoop(
	ctx context.Context,
	interval time.Duration,
	cfg *config.City,
	cityName string,
	tomlPath string,
	buildFn func(*config.City) []agent.Agent,
	sp session.Provider,
	rops reconcileOps,
	dops drainOps,
	ct crashTracker,
	rec events.Recorder,
	prefix string,
	poolSessions map[string]time.Duration,
	suspendedNames map[string]bool,
	stdout, stderr io.Writer,
) {
	dirty := &atomic.Bool{}
	if tomlPath != "" {
		cleanup := watchConfig(tomlPath, dirty, stderr)
		defer cleanup()
	}

	// Initial reconciliation.
	agents := buildFn(cfg)
	doReconcileAgents(agents, sp, rops, dops, ct, rec, prefix, poolSessions, suspendedNames, stdout, stderr)
	fmt.Fprintln(stdout, "City started.") //nolint:errcheck // best-effort stdout

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if dirty.Swap(false) {
				newCfg, err := tryReloadConfig(tomlPath, cityName)
				if err != nil {
					fmt.Fprintf(stderr, "gc start: config reload: %v (keeping old config)\n", err) //nolint:errcheck // best-effort stderr
				} else {
					cfg = newCfg
					poolSessions = computePoolSessions(cfg, cityName)
					suspendedNames = computeSuspendedNames(cfg, cityName)
					// Rebuild crash tracker if config changed.
					maxR := cfg.Daemon.MaxRestartsOrDefault()
					if maxR > 0 {
						ct = newCrashTracker(maxR, cfg.Daemon.RestartWindowDuration())
					} else {
						ct = nil
					}
					fmt.Fprintln(stdout, "Config reloaded.") //nolint:errcheck // best-effort stdout
				}
			}
			agents = buildFn(cfg)
			doReconcileAgents(agents, sp, rops, dops, ct, rec, prefix, poolSessions, suspendedNames, stdout, stderr)
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
	tomlPath string,
	cfg *config.City,
	buildFn func(*config.City) []agent.Agent,
	sp session.Provider,
	dops drainOps,
	poolSessions map[string]time.Duration,
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

	rops := newReconcileOps(sp)

	// Build crash tracker from config.
	var ct crashTracker
	maxR := cfg.Daemon.MaxRestartsOrDefault()
	if maxR > 0 {
		ct = newCrashTracker(maxR, cfg.Daemon.RestartWindowDuration())
	}

	suspendedNames := computeSuspendedNames(cfg, cityName)
	controllerLoop(ctx, cfg.Daemon.PatrolIntervalDuration(),
		cfg, cityName, tomlPath,
		buildFn, sp, rops, dops, ct, rec, cityPrefix, poolSessions, suspendedNames, stdout, stderr)

	// Shutdown: graceful stop all sessions with the city prefix.
	timeout := cfg.Daemon.ShutdownTimeoutDuration()
	if rops != nil {
		running, _ := rops.listRunning(cityPrefix)
		gracefulStopAll(running, sp, timeout, rec, stdout, stderr)
	} else {
		var names []string
		for _, a := range buildFn(cfg) {
			if a.IsRunning() {
				names = append(names, a.SessionName())
			}
		}
		gracefulStopAll(names, sp, timeout, rec, stdout, stderr)
	}

	rec.Record(events.Event{Type: events.ControllerStopped, Actor: "gc"})
	fmt.Fprintln(stdout, "Controller stopped.") //nolint:errcheck // best-effort stdout
	return 0
}
