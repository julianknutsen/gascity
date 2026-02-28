// Package subprocess implements [session.Provider] using child processes.
//
// Each session runs as a detached child process (via os/exec) with no
// terminal attached. This is the lightweight alternative to the tmux
// provider — useful for CI, testing, and environments where tmux is
// unavailable.
//
// Process tracking uses two layers:
//   - In-memory: for the same gc process (Start followed by Stop/IsRunning)
//   - PID files: for cross-process persistence (gc start → gc stop)
//
// Limitations compared to tmux:
//   - No interactive attach (Attach always returns an error)
//   - No startup hint support (fire-and-forget only)
package subprocess

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/steveyegge/gascity/internal/overlay"
	"github.com/steveyegge/gascity/internal/session"
)

// Provider manages agent sessions as child processes.
type Provider struct {
	mu       sync.Mutex
	dir      string            // PID file directory
	procs    map[string]*proc  // in-process tracking
	workDirs map[string]string // session name → workDir (for CopyTo)
}

// proc tracks a running child process.
type proc struct {
	cmd  *exec.Cmd
	done chan struct{} // closed when process exits
}

// Compile-time check.
var _ session.Provider = (*Provider)(nil)

// NewProvider returns a subprocess [Provider] that stores PID files in
// a default temporary directory. Suitable for production use.
func NewProvider() *Provider {
	dir := filepath.Join(os.TempDir(), "gc-subprocess")
	_ = os.MkdirAll(dir, 0o755)
	return &Provider{dir: dir, procs: make(map[string]*proc), workDirs: make(map[string]string)}
}

// NewProviderWithDir returns a subprocess [Provider] that stores PID files
// in the given directory. Useful for tests that need isolated state.
func NewProviderWithDir(dir string) *Provider {
	_ = os.MkdirAll(dir, 0o755)
	return &Provider{dir: dir, procs: make(map[string]*proc), workDirs: make(map[string]string)}
}

// Start spawns a child process for the given session name and config.
// Returns an error if a session with that name is already running.
// Startup hints (ReadyPromptPrefix, ProcessNames, etc.) are ignored —
// all sessions are fire-and-forget.
func (p *Provider) Start(name string, cfg session.Config) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check in-memory tracking first.
	if existing, ok := p.procs[name]; ok {
		if existing.alive() {
			return fmt.Errorf("session %q already exists", name)
		}
		delete(p.procs, name)
	}

	// Check PID file for cross-process case.
	if p.pidAlive(name) {
		return fmt.Errorf("session %q already exists", name)
	}

	// Store workDir for CopyTo.
	if cfg.WorkDir != "" {
		p.workDirs[name] = cfg.WorkDir
	}

	// Copy overlay and CopyFiles before starting the process.
	if cfg.OverlayDir != "" && cfg.WorkDir != "" {
		_ = overlay.CopyDir(cfg.OverlayDir, cfg.WorkDir, io.Discard)
	}
	for _, cf := range cfg.CopyFiles {
		dst := cfg.WorkDir
		if cf.RelDst != "" {
			dst = filepath.Join(cfg.WorkDir, cf.RelDst)
		}
		if absSrc, err := filepath.Abs(cf.Src); err == nil {
			if absDst, err := filepath.Abs(dst); err == nil && absSrc == absDst {
				continue
			}
		}
		_ = overlay.CopyFileOrDir(cf.Src, dst, io.Discard)
	}

	command := cfg.Command
	if command == "" {
		command = "sh"
	}

	cmd := exec.Command("sh", "-c", command)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	// Build environment: inherit parent env + apply overrides.
	env := os.Environ()
	if len(cfg.Env) > 0 {
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			env = append(env, k+"="+cfg.Env[k])
		}
	}
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting session %q: %w", name, err)
	}

	// Write PID file for cross-process discovery.
	_ = p.writePID(name, cmd.Process.Pid)

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
		// Clean up PID file when process exits.
		_ = os.Remove(p.pidPath(name))
	}()

	p.procs[name] = &proc{cmd: cmd, done: done}
	return nil
}

// Stop terminates the named session. Returns nil if it doesn't exist
// (idempotent). Sends SIGTERM first, then SIGKILL after a grace period.
func (p *Provider) Stop(name string) error {
	p.mu.Lock()
	pr, ok := p.procs[name]
	if ok {
		delete(p.procs, name)
	}
	p.mu.Unlock()

	// Try in-memory process first.
	if ok {
		_ = os.Remove(p.pidPath(name))
		if !pr.alive() {
			return nil
		}
		return terminateProc(pr)
	}

	// Fall back to PID file (cross-process case: gc stop after gc start).
	return p.stopByPID(name)
}

// Interrupt sends SIGINT to the named session's process.
// Best-effort: returns nil if the session doesn't exist.
func (p *Provider) Interrupt(name string) error {
	p.mu.Lock()
	pr, ok := p.procs[name]
	p.mu.Unlock()
	if !ok {
		return nil // idempotent
	}
	return pr.cmd.Process.Signal(syscall.SIGINT)
}

// IsRunning reports whether the named session has a live process.
func (p *Provider) IsRunning(name string) bool {
	p.mu.Lock()
	pr, ok := p.procs[name]
	p.mu.Unlock()

	if ok {
		return pr.alive()
	}

	// Fall back to PID file.
	return p.pidAlive(name)
}

// Attach is not supported by the subprocess provider.
func (p *Provider) Attach(_ string) error {
	return fmt.Errorf("subprocess provider does not support attach")
}

// ProcessAlive reports whether the named session is still running.
// The subprocess provider cannot inspect the process tree, so it
// delegates to IsRunning: if the session is alive, the agent process
// is assumed alive. Returns true when processNames is empty (per
// the Provider contract).
func (p *Provider) ProcessAlive(name string, processNames []string) bool {
	if len(processNames) == 0 {
		return true
	}
	return p.IsRunning(name)
}

// Nudge is not supported by the subprocess provider — there is no
// interactive terminal to send messages to. Returns nil (best-effort).
func (p *Provider) Nudge(_, _ string) error {
	return nil
}

// SendKeys is not supported by the subprocess provider — there is no
// interactive terminal to send keystrokes to. Returns nil (best-effort).
func (p *Provider) SendKeys(_ string, _ ...string) error {
	return nil
}

// Peek is not supported by the subprocess provider — there is no
// terminal with scrollback to capture. Returns an empty string.
func (p *Provider) Peek(_ string, _ int) (string, error) {
	return "", nil
}

// SetMeta stores a key-value pair for the named session in a sidecar file.
func (p *Provider) SetMeta(name, key, value string) error {
	return os.WriteFile(p.metaPath(name, key), []byte(value), 0o644)
}

// GetMeta retrieves a metadata value from a sidecar file.
// Returns ("", nil) if the key is not set.
func (p *Provider) GetMeta(name, key string) (string, error) {
	data, err := os.ReadFile(p.metaPath(name, key))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// RemoveMeta removes a metadata sidecar file.
func (p *Provider) RemoveMeta(name, key string) error {
	err := os.Remove(p.metaPath(name, key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetLastActivity returns zero time — subprocess provider does not
// support activity tracking.
func (p *Provider) GetLastActivity(_ string) (time.Time, error) {
	return time.Time{}, nil
}

// ClearScrollback is a no-op for subprocess sessions (no scrollback buffer).
func (p *Provider) ClearScrollback(_ string) error {
	return nil
}

// CopyTo copies src into the named session's working directory at relDst.
// Best-effort: returns nil if session unknown or src missing.
func (p *Provider) CopyTo(name, src, relDst string) error {
	p.mu.Lock()
	wd := p.workDirs[name]
	p.mu.Unlock()
	if wd == "" {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dst := wd
	if relDst != "" {
		dst = filepath.Join(wd, relDst)
	}
	return overlay.CopyDir(src, dst, io.Discard)
}

// ListRunning returns the names of all running sessions whose names
// match the given prefix, discovered via PID files.
func (p *Provider) ListRunning(prefix string) ([]string, error) {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".pid") {
			continue
		}
		sn := strings.TrimSuffix(n, ".pid")
		if !strings.HasPrefix(sn, prefix) {
			continue
		}
		if p.pidAlive(sn) {
			names = append(names, sn)
		}
	}
	return names, nil
}

func (p *Provider) metaPath(name, key string) string {
	return filepath.Join(p.dir, name+".meta."+key)
}

// --- PID file helpers ---

func (p *Provider) pidPath(name string) string {
	return filepath.Join(p.dir, name+".pid")
}

func (p *Provider) writePID(name string, pid int) error {
	return os.WriteFile(p.pidPath(name), []byte(strconv.Itoa(pid)), 0o644)
}

func (p *Provider) readPID(name string) (int, error) {
	data, err := os.ReadFile(p.pidPath(name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// pidAlive checks if a process tracked by PID file is still alive.
func (p *Provider) pidAlive(name string) bool {
	pid, err := p.readPID(name)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without actually signaling.
	return syscall.Kill(pid, 0) == nil
}

// stopByPID reads a PID file and terminates the process.
func (p *Provider) stopByPID(name string) error {
	pid, err := p.readPID(name)
	if err != nil {
		// No PID file — nothing to stop (idempotent).
		return nil
	}
	defer func() { _ = os.Remove(p.pidPath(name)) }()

	// Check if process is alive.
	if syscall.Kill(pid, 0) != nil {
		return nil // already dead
	}

	// Graceful shutdown: SIGTERM → wait → SIGKILL.
	_ = syscall.Kill(pid, syscall.SIGTERM)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return nil // died
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Force kill.
	_ = syscall.Kill(pid, syscall.SIGKILL)
	// Brief wait for kernel cleanup.
	time.Sleep(100 * time.Millisecond)
	return nil
}

// --- In-memory process helpers ---

// terminateProc sends SIGTERM then SIGKILL to an in-memory tracked process.
func terminateProc(pr *proc) error {
	_ = pr.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-pr.done:
		return nil
	case <-time.After(5 * time.Second):
	}

	_ = pr.cmd.Process.Kill()
	<-pr.done
	return nil
}

// alive reports whether the process is still running.
func (pr *proc) alive() bool {
	select {
	case <-pr.done:
		return false
	default:
		return true
	}
}
