package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/api"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/mail"
	"github.com/gastownhall/gascity/internal/session"
	"github.com/gastownhall/gascity/internal/supervisor"
	"github.com/spf13/cobra"
)

func newSupervisorCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supervisor",
		Short: "Manage the machine-wide supervisor",
		Long: `Manage the machine-wide supervisor daemon.

The supervisor manages all registered cities from a single process,
hosting a unified API server. Use "gc register" to add cities.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newSupervisorStartCmd(stdout, stderr),
		newSupervisorStopCmd(stdout, stderr),
		newSupervisorStatusCmd(stdout, stderr),
	)
	return cmd
}

func newSupervisorStartCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the machine-wide supervisor (foreground)",
		Long: `Start the machine-wide supervisor in the foreground.

The supervisor reads ~/.gc/cities.toml for registered cities and
~/.gc/supervisor.toml for configuration. It starts a CityRuntime
for each registered city and hosts a single API server.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if runSupervisor(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	return cmd
}

func newSupervisorStopCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the machine-wide supervisor",
		Long:  `Stop the running machine-wide supervisor and all its cities.`,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if stopSupervisor(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	return cmd
}

func newSupervisorStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check if the supervisor is running",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if supervisorStatus(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	return cmd
}

// acquireSupervisorLock takes an exclusive flock on the supervisor lock file.
func acquireSupervisorLock() (*os.File, error) {
	dir := supervisor.RuntimeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating runtime dir: %w", err)
	}
	path := filepath.Join(dir, "supervisor.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening supervisor lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close() //nolint:errcheck
		return nil, fmt.Errorf("supervisor already running")
	}
	return f, nil
}

// supervisorSocketPath returns the path to the supervisor control socket.
func supervisorSocketPath() string {
	return filepath.Join(supervisor.RuntimeDir(), "supervisor.sock")
}

// startSupervisorSocket creates a Unix domain socket at the given path
// and handles ping/stop commands. Unlike startControllerSocket (which
// constructs its own path), this binds to the exact path provided.
func startSupervisorSocket(sockPath string, cancelFn context.CancelFunc) (net.Listener, error) {
	os.Remove(sockPath) //nolint:errcheck // remove stale socket from previous crash
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on supervisor socket: %w", err)
	}
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return // listener closed
			}
			go handleSupervisorConn(conn, cancelFn)
		}
	}()
	return lis, nil
}

// handleSupervisorConn reads from a connection and dispatches commands.
// Supported: "stop" (shutdown), "ping" (liveness check, returns PID).
func handleSupervisorConn(conn net.Conn, cancelFn context.CancelFunc) {
	defer conn.Close()                                     //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		switch scanner.Text() {
		case "stop":
			cancelFn()
			conn.Write([]byte("ok\n")) //nolint:errcheck
		case "ping":
			fmt.Fprintf(conn, "%d\n", os.Getpid()) //nolint:errcheck
		}
	}
}

// supervisorAlive checks whether the supervisor is running by pinging
// the control socket. Returns the PID if alive, 0 otherwise.
func supervisorAlive() int {
	sockPath := supervisorSocketPath()
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return 0
	}
	defer conn.Close()                                    //nolint:errcheck
	conn.Write([]byte("ping\n"))                          //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}

// stopSupervisor sends a stop command to the running supervisor.
func stopSupervisor(stdout, stderr io.Writer) int {
	sockPath := supervisorSocketPath()
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		fmt.Fprintln(stderr, "gc supervisor stop: supervisor is not running") //nolint:errcheck
		return 1
	}
	defer conn.Close()                                     //nolint:errcheck
	conn.Write([]byte("stop\n"))                           //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	if n > 0 && string(buf[:n]) == "ok\n" {
		fmt.Fprintln(stdout, "Supervisor stopping...") //nolint:errcheck
		return 0
	}
	fmt.Fprintln(stderr, "gc supervisor stop: no acknowledgment from supervisor") //nolint:errcheck
	return 1
}

// supervisorStatus checks and reports whether the supervisor is running.
func supervisorStatus(stdout, _ io.Writer) int {
	pid := supervisorAlive()
	if pid > 0 {
		fmt.Fprintf(stdout, "Supervisor is running (PID %d)\n", pid) //nolint:errcheck
		return 0
	}
	fmt.Fprintln(stdout, "Supervisor is not running") //nolint:errcheck
	return 1
}

// managedCity tracks a running CityRuntime inside the supervisor.
type managedCity struct {
	cr     *CityRuntime
	cancel context.CancelFunc
	done   chan struct{} // closed when the city goroutine exits
	closer io.Closer     // FileRecorder (or nil); closed on city stop
}

// runSupervisor is the main supervisor loop. It acquires the lock,
// starts a control socket, reads the registry, starts CityRuntimes,
// and runs until canceled.
func runSupervisor(stdout, stderr io.Writer) int {
	lock, err := acquireSupervisorLock()
	if err != nil {
		fmt.Fprintf(stderr, "gc supervisor: %v\n", err) //nolint:errcheck
		return 1
	}
	defer lock.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Control socket — uses supervisor-specific path, not the per-city controller socket.
	sockPath := supervisorSocketPath()
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: creating socket dir: %v\n", err) //nolint:errcheck
		return 1
	}
	lis, err := startSupervisorSocket(sockPath, cancel)
	if err != nil {
		fmt.Fprintf(stderr, "gc supervisor: %v\n", err) //nolint:errcheck
		return 1
	}
	defer lis.Close()         //nolint:errcheck
	defer os.Remove(sockPath) //nolint:errcheck

	// Load supervisor config.
	supCfg, err := supervisor.LoadConfig(supervisor.ConfigPath())
	if err != nil {
		fmt.Fprintf(stderr, "gc supervisor: config: %v\n", err) //nolint:errcheck
		return 1
	}

	reg := supervisor.NewRegistry(supervisor.RegistryPath())

	// Track managed cities.
	var mu sync.Mutex
	cities := make(map[string]*managedCity)

	// Start API server with a multi-city state dispatcher.
	mcs := &multiCityState{cities: cities, mu: &mu, startedAt: time.Now()}
	bind := supCfg.Supervisor.BindOrDefault()
	port := supCfg.Supervisor.PortOrDefault()
	nonLocal := bind != "127.0.0.1" && bind != "localhost" && bind != "::1"
	var apiSrv *api.Server
	if nonLocal {
		apiSrv = api.NewReadOnly(mcs)
		fmt.Fprintf(stderr, "gc supervisor: binding to %s — mutation endpoints disabled (non-localhost)\n", bind) //nolint:errcheck
	} else {
		apiSrv = api.New(mcs)
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	apiLis, apiErr := net.Listen("tcp", addr)
	if apiErr != nil {
		fmt.Fprintf(stderr, "gc supervisor: api: listen %s failed: %v\n", addr, apiErr) //nolint:errcheck
		// Non-fatal — continue without API.
	} else {
		go func() {
			if err := apiSrv.Serve(apiLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(stderr, "gc supervisor: api: %v\n", err) //nolint:errcheck
			}
		}()
		defer func() {
			shutCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()
			apiSrv.Shutdown(shutCtx) //nolint:errcheck
		}()
		fmt.Fprintf(stdout, "Supervisor API listening on http://%s\n", addr) //nolint:errcheck
	}

	fmt.Fprintln(stdout, "Supervisor started.") //nolint:errcheck

	// Reconciliation loop.
	interval := supCfg.Supervisor.PatrolIntervalDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Panic backoff tracking for crash-loop prevention.
	panicHistory := make(map[string]*panicRecord)

	// Initial reconcile.
	reconcileCities(reg, cities, &mu, panicHistory, stdout, stderr)

	for {
		select {
		case <-ticker.C:
			reconcileCities(reg, cities, &mu, panicHistory, stdout, stderr)
		case <-ctx.Done():
			// Shutdown all cities. Collect under lock, then stop outside to
			// avoid blocking API requests during graceful shutdown.
			mu.Lock()
			toStop := make(map[string]*managedCity, len(cities))
			for k, v := range cities {
				toStop[k] = v
				delete(cities, k)
			}
			mu.Unlock()
			for name, mc := range toStop {
				fmt.Fprintf(stdout, "Stopping city '%s'...\n", name) //nolint:errcheck
				mc.cancel()
				<-mc.done
				mc.cr.shutdown()
				if mc.closer != nil {
					mc.closer.Close() //nolint:errcheck
				}
				fmt.Fprintf(stdout, "City '%s' stopped.\n", name) //nolint:errcheck
			}
			fmt.Fprintln(stdout, "Supervisor stopped.") //nolint:errcheck
			return 0
		}
	}
}

// panicRecord tracks consecutive panic count and next-eligible-restart time
// for crash-loop backoff on consistently-failing cities.
type panicRecord struct {
	count   int
	backoff time.Time // don't restart until after this time
}

// reconcileCities compares the registry against running cities and
// starts/stops as needed. panicHistory tracks cities that have panicked
// to implement crash-loop backoff.
func reconcileCities(
	reg *supervisor.Registry,
	cities map[string]*managedCity,
	mu *sync.Mutex,
	panicHistory map[string]*panicRecord,
	stdout, stderr io.Writer,
) {
	entries, err := reg.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc supervisor: registry: %v\n", err) //nolint:errcheck
		return
	}

	// Build desired set.
	desired := make(map[string]supervisor.CityEntry, len(entries))
	for _, e := range entries {
		desired[e.Path] = e
	}

	// Stop cities no longer in registry. Collect under lock, stop outside
	// to avoid blocking API requests during graceful shutdown.
	mu.Lock()
	var toStop []*managedCity
	var toStopPaths []string
	for path, mc := range cities {
		if _, ok := desired[path]; !ok {
			toStop = append(toStop, mc)
			toStopPaths = append(toStopPaths, path)
			delete(cities, path)
		}
	}
	mu.Unlock()

	for i, mc := range toStop {
		name := filepath.Base(toStopPaths[i])
		fmt.Fprintf(stdout, "Unregistered city '%s', stopping...\n", name) //nolint:errcheck
		mc.cancel()
		<-mc.done
		mc.cr.shutdown()
		if mc.closer != nil {
			mc.closer.Close() //nolint:errcheck
		}
		fmt.Fprintf(stdout, "City '%s' stopped.\n", name) //nolint:errcheck
	}

	// Start new cities. Build list of cities to start under lock, then
	// release lock for I/O-heavy initialization (config loading, bead
	// lifecycle, formula materialization, etc.).
	mu.Lock()
	var toStart []supervisor.CityEntry
	for path, entry := range desired {
		if _, running := cities[path]; !running {
			toStart = append(toStart, entry)
		}
	}
	mu.Unlock()

	for _, entry := range toStart {
		path := entry.Path
		name := entry.Name()

		// Crash-loop backoff: skip cities that panicked recently.
		// Must read panicHistory under lock since city goroutines write it.
		mu.Lock()
		pr := panicHistory[path]
		skipBackoff := pr != nil && time.Now().Before(pr.backoff)
		mu.Unlock()
		if skipBackoff {
			continue
		}

		// Load city config.
		tomlPath := filepath.Join(path, "city.toml")
		cfg, loadErr := config.Load(fsys.OSFS{}, tomlPath)
		if loadErr != nil {
			fmt.Fprintf(stderr, "gc supervisor: city '%s': %v (skipping)\n", name, loadErr) //nolint:errcheck
			continue
		}

		cityName := cfg.Workspace.Name
		if cityName == "" {
			cityName = filepath.Base(path)
		}

		// Run critical city initialization (same steps as cmd_start.go).
		if err := prepareCityForSupervisor(path, cityName, cfg, stderr); err != nil {
			fmt.Fprintf(stderr, "gc supervisor: city '%s': init: %v (skipping)\n", cityName, err) //nolint:errcheck
			continue
		}

		// Warn if city has its own API port.
		if cfg.API.Port > 0 {
			fmt.Fprintf(stderr, "gc supervisor: city '%s' has [api] port=%d which is ignored under supervisor mode\n", //nolint:errcheck
				cityName, cfg.API.Port)
		}

		sp, spErr := newSessionProviderByName(
			effectiveProviderName(cfg.Session.Provider), cfg.Session, cityName)
		if spErr != nil {
			fmt.Fprintf(stderr, "gc supervisor: city '%s': session provider: %v (skipping)\n", cityName, spErr) //nolint:errcheck
			continue
		}

		rec := events.Discard
		var eventProv events.Provider
		evPath := filepath.Join(path, ".gc", "events.jsonl")
		fr, frErr := events.NewFileRecorder(evPath, stderr)
		if frErr == nil {
			rec = fr
			eventProv = fr
		}

		dops := newDrainOps(sp)
		poolSessions := computePoolSessions(cfg, cityName, sp)
		poolDeathHandlers := computePoolDeathHandlers(cfg, cityName, path, sp)
		watchDirs := config.WatchDirs(nil, cfg, path)

		cr := newCityRuntime(CityRuntimeParams{
			CityPath:          path,
			CityName:          cityName,
			TomlPath:          tomlPath,
			WatchDirs:         watchDirs,
			Cfg:               cfg,
			SP:                sp,
			BuildFn:           supervisorBuildAgentsFn(path, cityName, stderr),
			Dops:              dops,
			Rec:               rec,
			PoolSessions:      poolSessions,
			PoolDeathHandlers: poolDeathHandlers,
			Stdout:            stdout,
			Stderr:            stderr,
		})

		// Wire API state.
		cs := newControllerState(cfg, sp, eventProv, cityName, path)
		cs.ct = cr.crashTrack()
		cr.setControllerState(cs)

		// Run pool on_boot hooks (same as runController does).
		runPoolOnBoot(cfg, path, shellScaleCheck, stderr)

		cityCtx, cityCancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		// Insert into map BEFORE launching goroutine to prevent races
		// where an early panic deletes a non-existent entry, leaving a
		// zombie after the post-launch insertion.
		mu.Lock()
		// Re-check: another goroutine might have added this city while we
		// were initializing outside the lock.
		if _, running := cities[path]; running {
			mu.Unlock()
			cityCancel()
			cr.shutdown()
			if fr != nil {
				fr.Close() //nolint:errcheck
			}
			continue
		}
		cities[path] = &managedCity{cr: cr, cancel: cityCancel, done: done, closer: fr}
		mu.Unlock()

		go func(n, p string, cityFr *events.FileRecorder) {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(stderr, "gc supervisor: city '%s' panicked: %v\n", n, r) //nolint:errcheck
					// Gracefully stop agents so they aren't orphaned.
					// Wrap in recovery to prevent nested panic from crashing
					// the entire supervisor.
					func() {
						defer func() { recover() }() //nolint:errcheck
						cr.shutdown()
					}()
					// Close the file recorder (only on panic — normal exit
					// leaves it for the external caller via mc.closer).
					if cityFr != nil {
						cityFr.Close() //nolint:errcheck
					}
					// Record panic for crash-loop backoff.
					mu.Lock()
					pr := panicHistory[p]
					if pr == nil {
						pr = &panicRecord{}
						panicHistory[p] = pr
					}
					pr.count++
					// Exponential backoff: 10s, 20s, 40s, ... capped at 5 min.
					exp := pr.count - 1
					if exp > 5 {
						exp = 5 // prevent int overflow at high panic counts
					}
					delay := time.Duration(10<<exp) * time.Second
					if delay > 5*time.Minute {
						delay = 5 * time.Minute
					}
					pr.backoff = time.Now().Add(delay)
					fmt.Fprintf(stderr, "gc supervisor: city '%s' panic #%d, next retry in %s\n", n, pr.count, delay) //nolint:errcheck
					mu.Unlock()
				} else {
					// Normal exit (context canceled) — reset panic counter.
					mu.Lock()
					delete(panicHistory, p)
					mu.Unlock()
				}
				// Remove from map so reconcile can restart this city.
				mu.Lock()
				delete(cities, p)
				mu.Unlock()
			}()
			cr.run(cityCtx)
		}(cityName, path, fr)

		rec.Record(events.Event{Type: events.ControllerStarted, Actor: "gc"})
		fmt.Fprintf(stdout, "Started city '%s' (%s)\n", cityName, path) //nolint:errcheck
	}
}

// prepareCityForSupervisor runs the critical city initialization steps
// that cmd_start.go performs before runController. Without these, cities
// would have no formulas, no bead stores, and no resolved rig paths.
func prepareCityForSupervisor(cityPath, cityName string, cfg *config.City, stderr io.Writer) error {
	// Validate rigs.
	if err := config.ValidateRigs(cfg.Rigs, cityName); err != nil {
		return fmt.Errorf("validate rigs: %w", err)
	}

	// Materialize the gc-beads-bd script.
	if _, err := MaterializeBeadsBdScript(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': materializing gc-beads-bd: %v\n", cityName, err) //nolint:errcheck
		// Non-fatal.
	}

	// Materialize builtin packs and inject them.
	if err := MaterializeBuiltinPacks(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': builtin packs: %v\n", cityName, err) //nolint:errcheck
		// Non-fatal.
	}
	injectBuiltinPacks(cfg, cityPath)

	// Materialize builtin prompts and formulas.
	if err := materializeBuiltinPrompts(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': builtin prompts: %v\n", cityName, err) //nolint:errcheck
	}
	if err := materializeBuiltinFormulas(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': builtin formulas: %v\n", cityName, err) //nolint:errcheck
	}

	// Resolve rig paths and start bead store lifecycle.
	resolveRigPaths(cityPath, cfg.Rigs)
	if err := startBeadsLifecycle(cityPath, cityName, cfg, stderr); err != nil {
		return fmt.Errorf("beads lifecycle: %w", err)
	}

	// Post-startup bead provider health check.
	if err := healthBeadsProvider(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': beads health: %v\n", cityName, err) //nolint:errcheck
		// Non-fatal.
	}

	// Materialize system formulas and prepend as Layer 0.
	sysDir, sysErr := MaterializeSystemFormulas(systemFormulasFS, "system_formulas", cityPath)
	if sysErr != nil {
		fmt.Fprintf(stderr, "gc supervisor: city '%s': system formulas: %v\n", cityName, sysErr) //nolint:errcheck
	}
	if sysDir != "" {
		cfg.FormulaLayers.City = append([]string{sysDir}, cfg.FormulaLayers.City...)
		for rigName, layers := range cfg.FormulaLayers.Rigs {
			cfg.FormulaLayers.Rigs[rigName] = append([]string{sysDir}, layers...)
		}
	}

	// Resolve formula symlinks.
	if len(cfg.FormulaLayers.City) > 0 {
		if err := ResolveFormulas(cityPath, cfg.FormulaLayers.City); err != nil {
			fmt.Fprintf(stderr, "gc supervisor: city '%s': city formulas: %v\n", cityName, err) //nolint:errcheck
		}
	}
	for _, r := range cfg.Rigs {
		if layers, ok := cfg.FormulaLayers.Rigs[r.Name]; ok && len(layers) > 0 {
			if err := ResolveFormulas(r.Path, layers); err != nil {
				fmt.Fprintf(stderr, "gc supervisor: city '%s': rig %q formulas: %v\n", cityName, r.Name, err) //nolint:errcheck
			}
		}
	}

	// Materialize Claude skill stubs.
	if cfg.Workspace.Provider == "claude" {
		dirs := []string{cityPath}
		for _, r := range cfg.Rigs {
			if r.Path != "" {
				dirs = append(dirs, r.Path)
			}
		}
		if err := materializeSkillStubs(dirs...); err != nil {
			fmt.Fprintf(stderr, "gc supervisor: city '%s': skill stubs: %v\n", cityName, err) //nolint:errcheck
		}
	}

	// Validate agents.
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		return fmt.Errorf("validate agents: %w", err)
	}

	return nil
}

// effectiveProviderName returns the provider name respecting GC_SESSION env override.
func effectiveProviderName(configured string) string {
	if v := os.Getenv("GC_SESSION"); v != "" {
		return v
	}
	return configured
}

// supervisorBuildAgentsFn returns a buildFn suitable for CityRuntimeParams.
// This mirrors the buildAgents closure in cmd_start.go, including dynamic
// pool evaluation via scale_check commands.
func supervisorBuildAgentsFn(cityPath, cityName string, stderr io.Writer) func(*config.City, session.Provider) []agent.Agent {
	return func(c *config.City, sp session.Provider) []agent.Agent {
		if c.Workspace.Suspended {
			return nil
		}
		bp := newAgentBuildParams(cityName, cityPath, c, sp, time.Now(), stderr)

		// Pre-compute suspended rig paths.
		suspendedRigPaths := make(map[string]bool)
		for _, r := range c.Rigs {
			if r.Suspended {
				suspendedRigPaths[filepath.Clean(r.Path)] = true
			}
		}

		type poolEvalWork struct {
			agentIdx int
			pool     config.PoolConfig
			poolDir  string
		}

		var agents []agent.Agent
		var pendingPools []poolEvalWork
		for i := range c.Agents {
			a := &c.Agents[i]
			if a.Suspended {
				continue
			}
			pool := a.EffectivePool()
			if pool.Max == 0 {
				continue
			}

			// Check rig suspension.
			if a.Dir != "" {
				if wd, wdErr := resolveAgentDir(cityPath, a.Dir); wdErr == nil {
					if suspendedRigPaths[filepath.Clean(wd)] {
						continue
					}
				}
			}

			if pool.IsMultiInstance() {
				// Pool agent — collect for parallel scale_check evaluation.
				poolDir := cityPath
				if a.Dir != "" {
					if pd, pdErr := resolveAgentDir(cityPath, a.Dir); pdErr == nil {
						poolDir = pd
					}
				}
				pendingPools = append(pendingPools, poolEvalWork{agentIdx: i, pool: pool, poolDir: poolDir})
				continue
			}

			qualifiedName := a.QualifiedName()
			fpExtra := buildFingerprintExtra(a)
			built, err := buildOneAgent(bp, a, qualifiedName, fpExtra)
			if err != nil {
				fmt.Fprintf(stderr, "gc supervisor: agent %q: %v (skipping)\n", qualifiedName, err) //nolint:errcheck
				continue
			}
			agents = append(agents, built)
		}

		// Evaluate pool scale_check commands in parallel.
		type poolEvalResult struct {
			desired int
			err     error
		}
		evalResults := make([]poolEvalResult, len(pendingPools))
		var wg sync.WaitGroup
		for j, pw := range pendingPools {
			wg.Add(1)
			go func(idx int, name string, pool config.PoolConfig, dir string) {
				defer wg.Done()
				desired, err := evaluatePool(name, pool, dir, shellScaleCheck)
				evalResults[idx] = poolEvalResult{desired: desired, err: err}
			}(j, c.Agents[pw.agentIdx].Name, pw.pool, pw.poolDir)
		}
		wg.Wait()

		for j, pw := range pendingPools {
			pr := evalResults[j]
			if pr.err != nil {
				fmt.Fprintf(stderr, "gc supervisor: %v (using min=%d)\n", pr.err, pw.pool.Min) //nolint:errcheck
			}
			running := countRunningPoolInstances(c.Agents[pw.agentIdx].Name, c.Agents[pw.agentIdx].Dir, pw.pool, cityName, c.Workspace.SessionTemplate, sp)
			if pr.desired != running {
				fmt.Fprintf(stderr, "gc supervisor: pool '%s': check returned %d, %d running → scaling %s\n", //nolint:errcheck
					c.Agents[pw.agentIdx].Name, pr.desired, running, scaleDirection(running, pr.desired))
			}
			pa, err := poolAgents(bp, &c.Agents[pw.agentIdx], pr.desired)
			if err != nil {
				fmt.Fprintf(stderr, "gc supervisor: pool %q: %v (skipping)\n", c.Agents[pw.agentIdx].QualifiedName(), err) //nolint:errcheck
				continue
			}
			agents = append(agents, pa...)
		}
		return agents
	}
}

// multiCityState implements api.State by delegating to the first registered
// city's controllerState. This is the v0 compatibility behavior described
// in the design doc — Phase 2 adds proper city-namespaced routing.
type multiCityState struct {
	cities    map[string]*managedCity
	mu        *sync.Mutex
	startedAt time.Time
}

// firstCity returns the controllerState of the first city (by sorted path),
// or nil if no cities are running. Deterministic ordering ensures API
// requests always route to the same city.
func (m *multiCityState) firstCity() *controllerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.cities) == 0 {
		return nil
	}
	paths := make([]string, 0, len(m.cities))
	for p := range m.cities {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if mc := m.cities[p]; mc.cr.cs != nil {
			return mc.cr.cs
		}
	}
	return nil
}

// --- api.State implementation (delegates to first city) ---

func (m *multiCityState) Config() *config.City {
	if cs := m.firstCity(); cs != nil {
		return cs.Config()
	}
	return &config.City{}
}

func (m *multiCityState) SessionProvider() session.Provider {
	if cs := m.firstCity(); cs != nil {
		return cs.SessionProvider()
	}
	// Return a no-op fake so API handlers don't nil-panic when zero cities are running.
	return &session.Fake{}
}

func (m *multiCityState) BeadStore(rig string) beads.Store {
	if cs := m.firstCity(); cs != nil {
		return cs.BeadStore(rig)
	}
	return nil // API handlers check for nil bead stores.
}

func (m *multiCityState) BeadStores() map[string]beads.Store {
	if cs := m.firstCity(); cs != nil {
		return cs.BeadStores()
	}
	return map[string]beads.Store{}
}

func (m *multiCityState) MailProvider(rig string) mail.Provider {
	if cs := m.firstCity(); cs != nil {
		return cs.MailProvider(rig)
	}
	return nil // API handlers check for nil mail providers.
}

func (m *multiCityState) MailProviders() map[string]mail.Provider {
	if cs := m.firstCity(); cs != nil {
		return cs.MailProviders()
	}
	return map[string]mail.Provider{}
}

func (m *multiCityState) EventProvider() events.Provider {
	if cs := m.firstCity(); cs != nil {
		return cs.EventProvider()
	}
	return nil // API handlers check for nil event providers.
}

func (m *multiCityState) CityName() string {
	if cs := m.firstCity(); cs != nil {
		return cs.CityName()
	}
	return ""
}

func (m *multiCityState) CityPath() string {
	if cs := m.firstCity(); cs != nil {
		return cs.CityPath()
	}
	return ""
}

func (m *multiCityState) Version() string {
	return version
}

func (m *multiCityState) StartedAt() time.Time {
	return m.startedAt
}

func (m *multiCityState) IsQuarantined(sessionName string) bool {
	if cs := m.firstCity(); cs != nil {
		return cs.IsQuarantined(sessionName)
	}
	return false
}
