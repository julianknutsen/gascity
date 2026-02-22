package tmux

import (
	"errors"
	"os"
	"os/exec"

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

// Start creates a new detached tmux session.
func (p *Provider) Start(name string, cfg session.Config) error {
	if cfg.Command != "" || len(cfg.Env) > 0 {
		return p.tm.NewSessionWithCommandAndEnv(name, cfg.WorkDir, cfg.Command, cfg.Env)
	}
	return p.tm.NewSession(name, cfg.WorkDir)
}

// Stop destroys the named session. Returns nil if it doesn't exist.
func (p *Provider) Stop(name string) error {
	err := p.tm.KillSession(name)
	if err != nil && (errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrNoServer)) {
		return nil // idempotent
	}
	return err
}

// IsRunning reports whether the named session exists.
func (p *Provider) IsRunning(name string) bool {
	has, err := p.tm.HasSession(name)
	return err == nil && has
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
