package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
)

// --- Core checks ---

// CityStructureCheck verifies .gc/ dir and city.toml exist.
type CityStructureCheck struct{}

// Name returns the check identifier.
func (c *CityStructureCheck) Name() string { return "city-structure" }

// Run checks that the city directory has the expected structure.
func (c *CityStructureCheck) Run(ctx *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	gcDir := filepath.Join(ctx.CityPath, ".gc")
	toml := filepath.Join(ctx.CityPath, "city.toml")

	if fi, err := os.Stat(gcDir); err != nil || !fi.IsDir() {
		r.Status = StatusError
		r.Message = ".gc/ directory missing"
		return r
	}
	if _, err := os.Stat(toml); err != nil {
		r.Status = StatusError
		r.Message = "city.toml missing"
		return r
	}
	r.Status = StatusOK
	r.Message = ".gc/ and city.toml present"
	return r
}

// CanFix returns false — structure must be created by gc init.
func (c *CityStructureCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *CityStructureCheck) Fix(_ *CheckContext) error { return nil }

// CityConfigCheck verifies city.toml parses and workspace.name is set.
type CityConfigCheck struct{}

// Name returns the check identifier.
func (c *CityConfigCheck) Name() string { return "city-config" }

// Run parses city.toml and checks workspace.name.
func (c *CityConfigCheck) Run(ctx *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(ctx.CityPath, "city.toml"))
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("city.toml parse error: %v", err)
		return r
	}
	if cfg.Workspace.Name == "" {
		r.Status = StatusError
		r.Message = "workspace.name not set"
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("city.toml loaded (%d agents, %d rigs)", len(cfg.Agents), len(cfg.Rigs))
	return r
}

// CanFix returns false.
func (c *CityConfigCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *CityConfigCheck) Fix(_ *CheckContext) error { return nil }

// ConfigValidCheck runs ValidateAgents and ValidateRigs.
type ConfigValidCheck struct {
	cfg *config.City
}

// NewConfigValidCheck creates a check that validates the parsed config.
func NewConfigValidCheck(cfg *config.City) *ConfigValidCheck {
	return &ConfigValidCheck{cfg: cfg}
}

// Name returns the check identifier.
func (c *ConfigValidCheck) Name() string { return "config-valid" }

// Run validates agents and rigs in the config.
func (c *ConfigValidCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	if err := config.ValidateAgents(c.cfg.Agents); err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("agent validation: %v", err)
		return r
	}
	cityName := c.cfg.Workspace.Name
	if cityName == "" {
		cityName = "unknown"
	}
	if err := config.ValidateRigs(c.cfg.Rigs, cityName); err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("rig validation: %v", err)
		return r
	}
	r.Status = StatusOK
	r.Message = "agents and rigs valid"
	return r
}

// CanFix returns false.
func (c *ConfigValidCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *ConfigValidCheck) Fix(_ *CheckContext) error { return nil }

// --- Infrastructure checks ---

// LookPathFunc is the function used to find binaries. Defaults to exec.LookPath.
// Tests can override this.
type LookPathFunc func(file string) (string, error)

// BinaryCheck verifies a binary is on PATH.
type BinaryCheck struct {
	binary   string
	skipMsg  string // non-empty means skip with OK + this message
	lookPath LookPathFunc
}

// NewBinaryCheck creates a check for the given binary.
// If skipMsg is non-empty, the check returns OK with that message (used when
// the binary is not needed due to env config like GC_BEADS=file).
func NewBinaryCheck(binary string, skipMsg string, lp LookPathFunc) *BinaryCheck {
	if lp == nil {
		lp = exec.LookPath
	}
	return &BinaryCheck{binary: binary, skipMsg: skipMsg, lookPath: lp}
}

// Name returns the check identifier.
func (c *BinaryCheck) Name() string { return c.binary + "-binary" }

// Run checks if the binary is on PATH.
func (c *BinaryCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	if c.skipMsg != "" {
		r.Status = StatusOK
		r.Message = c.skipMsg
		return r
	}
	path, err := c.lookPath(c.binary)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("%s not found in PATH", c.binary)
		r.FixHint = fmt.Sprintf("install %s and ensure it's in PATH", c.binary)
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("found %s", path)
	return r
}

// CanFix returns false.
func (c *BinaryCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *BinaryCheck) Fix(_ *CheckContext) error { return nil }

// --- Session checks (skipped when controller is running) ---

// AgentSessionsCheck verifies non-suspended agents have running sessions.
type AgentSessionsCheck struct {
	cfg             *config.City
	cityName        string
	sessionTemplate string
	sp              session.Provider
}

// NewAgentSessionsCheck creates a check for agent session liveness.
func NewAgentSessionsCheck(cfg *config.City, cityName, sessionTemplate string, sp session.Provider) *AgentSessionsCheck {
	return &AgentSessionsCheck{cfg: cfg, cityName: cityName, sessionTemplate: sessionTemplate, sp: sp}
}

// Name returns the check identifier.
func (c *AgentSessionsCheck) Name() string { return "agent-sessions" }

// Run checks that each non-suspended agent has a running session.
func (c *AgentSessionsCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	var missing []string
	for _, a := range c.cfg.Agents {
		if a.Suspended {
			continue
		}
		sn := agent.SessionNameFor(c.cityName, a.QualifiedName(), c.sessionTemplate)
		if !c.sp.IsRunning(sn) {
			missing = append(missing, a.QualifiedName())
		}
	}
	if len(missing) == 0 {
		r.Status = StatusOK
		r.Message = "all agent sessions running"
		return r
	}
	r.Status = StatusWarning
	r.Message = fmt.Sprintf("%d agent(s) without sessions", len(missing))
	r.Details = missing
	r.FixHint = "run gc start to reconcile sessions"
	return r
}

// CanFix returns false.
func (c *AgentSessionsCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *AgentSessionsCheck) Fix(_ *CheckContext) error { return nil }

// ZombieSessionsCheck finds sessions that are alive but the agent process is dead.
type ZombieSessionsCheck struct {
	cfg             *config.City
	cityName        string
	sessionTemplate string
	sp              session.Provider
}

// NewZombieSessionsCheck creates a check for zombie sessions.
func NewZombieSessionsCheck(cfg *config.City, cityName, sessionTemplate string, sp session.Provider) *ZombieSessionsCheck {
	return &ZombieSessionsCheck{cfg: cfg, cityName: cityName, sessionTemplate: sessionTemplate, sp: sp}
}

// Name returns the check identifier.
func (c *ZombieSessionsCheck) Name() string { return "zombie-sessions" }

// Run checks for sessions where the session exists but the agent process is dead.
func (c *ZombieSessionsCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	var zombies []string
	for _, a := range c.cfg.Agents {
		if a.Suspended || len(a.ProcessNames) == 0 {
			continue
		}
		sn := agent.SessionNameFor(c.cityName, a.QualifiedName(), c.sessionTemplate)
		if c.sp.IsRunning(sn) && !c.sp.ProcessAlive(sn, a.ProcessNames) {
			zombies = append(zombies, sn)
		}
	}
	if len(zombies) == 0 {
		r.Status = StatusOK
		r.Message = "no zombie sessions"
		return r
	}
	r.Status = StatusWarning
	r.Message = fmt.Sprintf("%d zombie session(s)", len(zombies))
	r.Details = zombies
	return r
}

// CanFix returns true — zombie sessions can be killed.
func (c *ZombieSessionsCheck) CanFix() bool { return true }

// Fix kills all zombie sessions.
func (c *ZombieSessionsCheck) Fix(_ *CheckContext) error {
	for _, a := range c.cfg.Agents {
		if a.Suspended || len(a.ProcessNames) == 0 {
			continue
		}
		sn := agent.SessionNameFor(c.cityName, a.QualifiedName(), c.sessionTemplate)
		if c.sp.IsRunning(sn) && !c.sp.ProcessAlive(sn, a.ProcessNames) {
			if err := c.sp.Stop(sn); err != nil {
				return fmt.Errorf("killing zombie session %q: %w", sn, err)
			}
		}
	}
	return nil
}

// OrphanSessionsCheck finds sessions with the city prefix not in config.
type OrphanSessionsCheck struct {
	cfg             *config.City
	cityName        string
	sessionTemplate string
	sp              session.Provider
}

// NewOrphanSessionsCheck creates a check for orphaned sessions.
func NewOrphanSessionsCheck(cfg *config.City, cityName, sessionTemplate string, sp session.Provider) *OrphanSessionsCheck {
	return &OrphanSessionsCheck{cfg: cfg, cityName: cityName, sessionTemplate: sessionTemplate, sp: sp}
}

// Name returns the check identifier.
func (c *OrphanSessionsCheck) Name() string { return "orphan-sessions" }

// Run finds sessions with the city prefix that don't match any configured agent.
func (c *OrphanSessionsCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	prefix := "gc-" + c.cityName + "-"
	running, err := c.sp.ListRunning(prefix)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("listing sessions: %v", err)
		return r
	}

	// Build set of expected session names.
	expected := make(map[string]bool)
	for _, a := range c.cfg.Agents {
		sn := agent.SessionNameFor(c.cityName, a.QualifiedName(), c.sessionTemplate)
		expected[sn] = true
	}

	var orphans []string
	for _, s := range running {
		if !expected[s] {
			orphans = append(orphans, s)
		}
	}

	if len(orphans) == 0 {
		r.Status = StatusOK
		r.Message = "no orphaned sessions"
		return r
	}
	r.Status = StatusWarning
	r.Message = fmt.Sprintf("%d orphaned session(s)", len(orphans))
	r.Details = orphans
	return r
}

// CanFix returns true — orphan sessions can be killed.
func (c *OrphanSessionsCheck) CanFix() bool { return true }

// Fix kills all orphaned sessions.
func (c *OrphanSessionsCheck) Fix(_ *CheckContext) error {
	prefix := "gc-" + c.cityName + "-"
	running, err := c.sp.ListRunning(prefix)
	if err != nil {
		return err
	}
	expected := make(map[string]bool)
	for _, a := range c.cfg.Agents {
		sn := agent.SessionNameFor(c.cityName, a.QualifiedName(), c.sessionTemplate)
		expected[sn] = true
	}
	for _, s := range running {
		if !expected[s] {
			if err := c.sp.Stop(s); err != nil {
				return fmt.Errorf("killing orphan session %q: %w", s, err)
			}
		}
	}
	return nil
}

// --- Data checks ---

// BeadsStoreCheck verifies the bead store opens and List succeeds.
type BeadsStoreCheck struct {
	cityPath string
	newStore func(cityPath string) (beads.Store, error)
}

// NewBeadsStoreCheck creates a check for the bead store.
// newStore is a factory that opens a store from the city path.
func NewBeadsStoreCheck(cityPath string, newStore func(string) (beads.Store, error)) *BeadsStoreCheck {
	return &BeadsStoreCheck{cityPath: cityPath, newStore: newStore}
}

// Name returns the check identifier.
func (c *BeadsStoreCheck) Name() string { return "beads-store" }

// Run opens the store and calls List.
func (c *BeadsStoreCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	store, err := c.newStore(c.cityPath)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("store open failed: %v", err)
		return r
	}
	list, err := store.List()
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("store list failed: %v", err)
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("store accessible (%d beads)", len(list))
	return r
}

// CanFix returns false.
func (c *BeadsStoreCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *BeadsStoreCheck) Fix(_ *CheckContext) error { return nil }

// DoltServerCheck verifies the dolt server is running and reachable.
type DoltServerCheck struct {
	cityPath string
	skip     bool
}

// NewDoltServerCheck creates a check for the dolt server.
// If skip is true, the check returns OK (dolt not needed).
func NewDoltServerCheck(cityPath string, skip bool) *DoltServerCheck {
	return &DoltServerCheck{cityPath: cityPath, skip: skip}
}

// Name returns the check identifier.
func (c *DoltServerCheck) Name() string { return "dolt-server" }

// Run checks if the dolt server is running and reachable via TCP.
func (c *DoltServerCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	if c.skip {
		r.Status = StatusOK
		r.Message = "skipped (file backend or GC_DOLT=skip)"
		return r
	}
	running, pid, err := dolt.IsRunningCity(c.cityPath)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("check failed: %v", err)
		return r
	}
	if !running {
		r.Status = StatusError
		r.Message = "dolt server not running"
		r.FixHint = "run gc start to start the dolt server"
		return r
	}

	// Verify TCP reachability.
	cfg := dolt.GasCityConfig(c.cityPath)
	addr := cfg.HostPort()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("server running (PID %d) but not reachable at %s", pid, addr)
		return r
	}
	conn.Close() //nolint:errcheck // best-effort close

	r.Status = StatusOK
	r.Message = fmt.Sprintf("running (PID %d) on %s", pid, addr)
	return r
}

// CanFix returns false.
func (c *DoltServerCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *DoltServerCheck) Fix(_ *CheckContext) error { return nil }

// EventsLogCheck verifies .gc/events.jsonl exists and is writable.
type EventsLogCheck struct{}

// Name returns the check identifier.
func (c *EventsLogCheck) Name() string { return "events-log" }

// Run checks the events log file.
func (c *EventsLogCheck) Run(ctx *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	path := filepath.Join(ctx.CityPath, ".gc", "events.jsonl")
	fi, err := os.Stat(path)
	if err != nil {
		r.Status = StatusWarning
		r.Message = "events.jsonl not found (events will not be logged)"
		return r
	}
	// Check writable by opening for append.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, fi.Mode())
	if err != nil {
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("events.jsonl not writable: %v", err)
		return r
	}
	f.Close() //nolint:errcheck // best-effort close
	r.Status = StatusOK
	r.Message = "events.jsonl exists and writable"
	return r
}

// CanFix returns false.
func (c *EventsLogCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *EventsLogCheck) Fix(_ *CheckContext) error { return nil }

// --- Controller check (informational) ---

// ControllerCheck reports whether the controller is running.
type ControllerCheck struct {
	cityPath string
	running  bool // pre-computed by caller
}

// NewControllerCheck creates an informational controller status check.
func NewControllerCheck(cityPath string, running bool) *ControllerCheck {
	return &ControllerCheck{cityPath: cityPath, running: running}
}

// Name returns the check identifier.
func (c *ControllerCheck) Name() string { return "controller" }

// Run reports controller status. Always returns OK — both states are valid.
func (c *ControllerCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name(), Status: StatusOK}
	if c.running {
		r.Message = "controller running (sessions managed)"
	} else {
		r.Message = "controller not running (one-shot mode)"
	}
	return r
}

// CanFix returns false.
func (c *ControllerCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *ControllerCheck) Fix(_ *CheckContext) error { return nil }

// --- Per-rig checks ---

// RigPathCheck verifies a rig's path exists and is a directory.
type RigPathCheck struct {
	rig config.Rig
}

// NewRigPathCheck creates a rig path existence check.
func NewRigPathCheck(rig config.Rig) *RigPathCheck {
	return &RigPathCheck{rig: rig}
}

// Name returns the check identifier.
func (c *RigPathCheck) Name() string { return "rig:" + c.rig.Name + ":path" }

// Run checks the rig path exists and is a directory.
func (c *RigPathCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	fi, err := os.Stat(c.rig.Path)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("path %q not found", c.rig.Path)
		return r
	}
	if !fi.IsDir() {
		r.Status = StatusError
		r.Message = fmt.Sprintf("path %q is not a directory", c.rig.Path)
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("path %q exists", c.rig.Path)
	return r
}

// CanFix returns false.
func (c *RigPathCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *RigPathCheck) Fix(_ *CheckContext) error { return nil }

// RigGitCheck verifies a rig's path is a git repository. Non-git is a warning, not error.
type RigGitCheck struct {
	rig config.Rig
}

// NewRigGitCheck creates a rig git repo check.
func NewRigGitCheck(rig config.Rig) *RigGitCheck {
	return &RigGitCheck{rig: rig}
}

// Name returns the check identifier.
func (c *RigGitCheck) Name() string { return "rig:" + c.rig.Name + ":git" }

// Run checks if the rig path is a git repository.
func (c *RigGitCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	gitDir := filepath.Join(c.rig.Path, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		r.Status = StatusWarning
		r.Message = "not a git repository"
		return r
	}
	r.Status = StatusOK
	r.Message = "git repository"
	return r
}

// CanFix returns false.
func (c *RigGitCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *RigGitCheck) Fix(_ *CheckContext) error { return nil }

// RigBeadsCheck verifies a rig's beads store is accessible.
type RigBeadsCheck struct {
	rig      config.Rig
	newStore func(rigPath string) (beads.Store, error)
}

// NewRigBeadsCheck creates a rig beads store accessibility check.
func NewRigBeadsCheck(rig config.Rig, newStore func(string) (beads.Store, error)) *RigBeadsCheck {
	return &RigBeadsCheck{rig: rig, newStore: newStore}
}

// Name returns the check identifier.
func (c *RigBeadsCheck) Name() string { return "rig:" + c.rig.Name + ":beads" }

// Run opens the rig's bead store and calls List.
func (c *RigBeadsCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	store, err := c.newStore(c.rig.Path)
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("store open failed: %v", err)
		return r
	}
	list, err := store.List()
	if err != nil {
		r.Status = StatusError
		r.Message = fmt.Sprintf("store list failed: %v", err)
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("store accessible (%d beads)", len(list))
	return r
}

// CanFix returns false.
func (c *RigBeadsCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *RigBeadsCheck) Fix(_ *CheckContext) error { return nil }

// --- Topology cache checks ---

// TopologyCacheCheck verifies all remote topology caches are present.
type TopologyCacheCheck struct {
	topologies map[string]config.TopologySource
	cityPath   string
}

// NewTopologyCacheCheck creates a check for topology cache completeness.
func NewTopologyCacheCheck(topologies map[string]config.TopologySource, cityPath string) *TopologyCacheCheck {
	return &TopologyCacheCheck{topologies: topologies, cityPath: cityPath}
}

// Name returns the check identifier.
func (c *TopologyCacheCheck) Name() string { return "topology-cache" }

// Run checks that each configured topology has a cached topology.toml.
func (c *TopologyCacheCheck) Run(_ *CheckContext) *CheckResult {
	r := &CheckResult{Name: c.Name()}
	var missing []string
	for name, src := range c.topologies {
		cachePath := config.TopologyCachePath(c.cityPath, name, src)
		topoFile := filepath.Join(cachePath, "topology.toml")
		if _, err := os.Stat(topoFile); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		r.Status = StatusOK
		r.Message = fmt.Sprintf("all %d topology cache(s) present", len(c.topologies))
		return r
	}
	r.Status = StatusError
	r.Message = fmt.Sprintf("%d topology cache(s) missing", len(missing))
	r.Details = missing
	r.FixHint = "run gc topology fetch"
	return r
}

// CanFix returns false — use gc topology fetch to populate caches.
func (c *TopologyCacheCheck) CanFix() bool { return false }

// Fix is a no-op.
func (c *TopologyCacheCheck) Fix(_ *CheckContext) error { return nil }

// IsControllerRunning probes the controller lock file to determine if a
// controller is currently running. It tries to acquire the flock — if it
// fails with EWOULDBLOCK, the controller holds the lock.
func IsControllerRunning(cityPath string) bool {
	path := filepath.Join(cityPath, ".gc", "controller.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // probe only

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// EWOULDBLOCK means the lock is held — controller is running.
		return true
	}
	// We got the lock, release immediately — no controller running.
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck // best-effort unlock
	return false
}
