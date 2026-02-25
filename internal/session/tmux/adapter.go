package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/session"
)

// Provider adapts [Tmux] to the [session.Provider] interface.
type Provider struct {
	tm *Tmux
}

// Compile-time check.
var _ session.Provider = (*Provider)(nil)

// NewProvider returns a [Provider] backed by a real tmux installation.
func NewProvider() *Provider {
	return &Provider{tm: NewTmux()}
}

// Start creates a new detached tmux session and performs a multi-step
// startup sequence to ensure agent readiness. The sequence handles zombie
// detection, command launch verification, permission warning dismissal,
// and runtime readiness polling. Steps are conditional on Config fields
// being set; an agent with no startup hints gets fire-and-forget.
func (p *Provider) Start(name string, cfg session.Config) error {
	return doStartSession(&tmuxStartOps{tm: p.tm}, name, cfg)
}

// Stop destroys the named session and kills its entire process tree.
// Returns nil if it doesn't exist (idempotent).
func (p *Provider) Stop(name string) error {
	err := p.tm.KillSessionWithProcesses(name)
	if err != nil && (errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrNoServer)) {
		return nil // idempotent
	}
	return err
}

// Interrupt sends Ctrl-C to the named tmux session.
// Best-effort: returns nil if the session doesn't exist.
func (p *Provider) Interrupt(name string) error {
	return p.tm.SendKeysRaw(name, "C-c")
}

// IsRunning reports whether the named session exists.
func (p *Provider) IsRunning(name string) bool {
	has, err := p.tm.HasSession(name)
	return err == nil && has
}

// ProcessAlive reports whether the named session has a live agent
// process matching one of the given names in its process tree.
// Returns true if processNames is empty (no check possible).
func (p *Provider) ProcessAlive(name string, processNames []string) bool {
	if len(processNames) == 0 {
		return true
	}
	return p.tm.IsRuntimeRunning(name, processNames)
}

// Nudge sends a message to the named session to wake or redirect the agent.
// Delegates to [Tmux.NudgeSession] which handles per-session locking,
// multi-pane resolution, retry with backoff, and SIGWINCH wake.
func (p *Provider) Nudge(name, message string) error {
	return p.tm.NudgeSession(name, message)
}

// SetMeta stores a key-value pair in the named session's tmux environment.
func (p *Provider) SetMeta(name, key, value string) error {
	return p.tm.SetEnvironment(name, key, value)
}

// GetMeta retrieves a value from the named session's tmux environment.
// Returns ("", nil) if the key is not set.
func (p *Provider) GetMeta(name, key string) (string, error) {
	val, err := p.tm.GetEnvironment(name, key)
	if err != nil {
		return "", nil // not set
	}
	return val, nil
}

// RemoveMeta removes a key from the named session's tmux environment.
func (p *Provider) RemoveMeta(name, key string) error {
	return p.tm.RemoveEnvironment(name, key)
}

// Peek captures the last N lines of output from the named session.
// If lines <= 0, captures all available scrollback.
func (p *Provider) Peek(name string, lines int) (string, error) {
	if lines <= 0 {
		return p.tm.CapturePaneAll(name)
	}
	return p.tm.CapturePane(name, lines)
}

// ListRunning returns all tmux session names matching the given prefix.
func (p *Provider) ListRunning(prefix string) ([]string, error) {
	all, err := p.tm.ListSessions()
	if err != nil {
		return nil, err
	}
	var matched []string
	for _, name := range all {
		if strings.HasPrefix(name, prefix) {
			matched = append(matched, name)
		}
	}
	return matched, nil
}

// Attach connects the user's terminal to the named tmux session.
// This hands stdin/stdout/stderr to tmux and blocks until detach.
func (p *Provider) Attach(name string) error {
	cmd := exec.Command("tmux", "-u", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Tmux returns the underlying [Tmux] instance for advanced operations
// that are not part of the [session.Provider] interface.
func (p *Provider) Tmux() *Tmux {
	return p.tm
}

// ---------------------------------------------------------------------------
// Multi-step startup orchestration
// ---------------------------------------------------------------------------

// startOps abstracts tmux operations needed by the startup sequence.
// This enables unit testing without a real tmux server.
type startOps interface {
	createSession(name, workDir, command string, env map[string]string) error
	isRuntimeRunning(name string, processNames []string) bool
	killSession(name string) error
	waitForCommand(name string, timeout time.Duration) error
	acceptBypassWarning(name string) error
	waitForReady(name string, rc *RuntimeConfig, timeout time.Duration) error
	hasSession(name string) (bool, error)
}

// tmuxStartOps adapts [*Tmux] to the [startOps] interface.
type tmuxStartOps struct{ tm *Tmux }

func (o *tmuxStartOps) createSession(name, workDir, command string, env map[string]string) error {
	if command != "" || len(env) > 0 {
		return o.tm.NewSessionWithCommandAndEnv(name, workDir, command, env)
	}
	return o.tm.NewSession(name, workDir)
}

func (o *tmuxStartOps) isRuntimeRunning(name string, processNames []string) bool {
	return o.tm.IsRuntimeRunning(name, processNames)
}

func (o *tmuxStartOps) killSession(name string) error {
	return o.tm.KillSession(name)
}

func (o *tmuxStartOps) waitForCommand(name string, timeout time.Duration) error {
	return o.tm.WaitForCommand(name, supportedShells, timeout)
}

func (o *tmuxStartOps) acceptBypassWarning(name string) error {
	return o.tm.AcceptBypassPermissionsWarning(name)
}

func (o *tmuxStartOps) waitForReady(name string, rc *RuntimeConfig, timeout time.Duration) error {
	return o.tm.WaitForRuntimeReady(name, rc, timeout)
}

func (o *tmuxStartOps) hasSession(name string) (bool, error) {
	return o.tm.HasSession(name)
}

// doStartSession is the pure startup orchestration logic.
// Testable via fakeStartOps without a real tmux server.
func doStartSession(ops startOps, name string, cfg session.Config) error {
	// Step 1: Ensure fresh session (zombie detection).
	if err := ensureFreshSession(ops, name, cfg); err != nil {
		return err
	}

	hasHints := cfg.ReadyPromptPrefix != "" || cfg.ReadyDelayMs > 0 ||
		len(cfg.ProcessNames) > 0 || cfg.EmitsPermissionWarning

	if !hasHints {
		return nil // fire-and-forget
	}

	// Step 2: Wait for agent command to appear (not still in shell).
	if len(cfg.ProcessNames) > 0 {
		_ = ops.waitForCommand(name, 30*time.Second) // best-effort, non-fatal
	}

	// Step 3: Accept bypass permissions warning if needed.
	if cfg.EmitsPermissionWarning {
		_ = ops.acceptBypassWarning(name) // best-effort
	}

	// Step 4: Wait for runtime readiness.
	if cfg.ReadyPromptPrefix != "" || cfg.ReadyDelayMs > 0 {
		rc := &RuntimeConfig{Tmux: &RuntimeTmuxConfig{
			ReadyPromptPrefix: cfg.ReadyPromptPrefix,
			ReadyDelayMs:      cfg.ReadyDelayMs,
			ProcessNames:      cfg.ProcessNames,
		}}
		_ = ops.waitForReady(name, rc, 60*time.Second) // best-effort
	}

	// Step 5: Verify session survived startup.
	alive, err := ops.hasSession(name)
	if err != nil {
		return fmt.Errorf("verifying session: %w", err)
	}
	if !alive {
		return fmt.Errorf("session %q died during startup", name)
	}

	return nil
}

// ensureFreshSession creates a session, handling zombies.
// If the session already exists with a dead agent, kills and recreates.
// If the session already exists with a live agent, returns nil (idempotent).
func ensureFreshSession(ops startOps, name string, cfg session.Config) error {
	err := ops.createSession(name, cfg.WorkDir, cfg.Command, cfg.Env)
	if err == nil {
		return nil // created successfully
	}
	if !errors.Is(err, ErrSessionExists) {
		return fmt.Errorf("creating session: %w", err)
	}

	// Session exists — check if agent is alive.
	if ops.isRuntimeRunning(name, cfg.ProcessNames) {
		return nil // healthy session, nothing to do
	}

	// Zombie: tmux alive but agent dead. Kill and recreate.
	if err := ops.killSession(name); err != nil {
		return fmt.Errorf("killing zombie session: %w", err)
	}
	err = ops.createSession(name, cfg.WorkDir, cfg.Command, cfg.Env)
	if errors.Is(err, ErrSessionExists) {
		return nil // race: another process created it
	}
	if err != nil {
		return fmt.Errorf("creating session after zombie cleanup: %w", err)
	}
	return nil
}
