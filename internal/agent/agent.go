// Package agent provides the Agent interface for managed agent lifecycle.
//
// An Agent encapsulates identity (name, session name) and lifecycle
// operations (start, stop, attach) backed by a [session.Provider].
// The CLI layer builds agents from config; the do* functions operate
// on them without knowing how sessions are implemented.
package agent

import (
	"strings"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

// Agent represents a managed agent in the city.
type Agent interface {
	// Name returns the agent's configured name.
	Name() string

	// SessionName returns the session identifier for this agent.
	SessionName() string

	// IsRunning reports whether the agent's session is active.
	IsRunning() bool

	// Start creates the agent's session.
	Start() error

	// Stop destroys the agent's session.
	Stop() error

	// Attach connects the user's terminal to the agent's session.
	Attach() error
}

// New creates an Agent backed by the given session provider.
// The config.Agent flows through from TOML so the agent has access to all
// configured fields. The resolved command and session name are runtime-derived.
// prompt is the agent's initial prompt content (appended to command via shell
// quoting). env is additional environment variables for the session.
func New(ac config.Agent, sessionName, command, prompt string, env map[string]string, sp session.Provider) Agent {
	return &managed{
		cfg:         ac,
		sessionName: sessionName,
		command:     command,
		prompt:      prompt,
		env:         env,
		sp:          sp,
	}
}

// managed is the concrete Agent implementation that delegates to a
// session.Provider using the agent's session name.
type managed struct {
	cfg         config.Agent
	sessionName string
	command     string
	prompt      string
	env         map[string]string
	sp          session.Provider
}

func (a *managed) Name() string        { return a.cfg.Name }
func (a *managed) SessionName() string { return a.sessionName }
func (a *managed) IsRunning() bool     { return a.sp.IsRunning(a.sessionName) }
func (a *managed) Stop() error         { return a.sp.Stop(a.sessionName) }
func (a *managed) Attach() error       { return a.sp.Attach(a.sessionName) }

func (a *managed) Start() error {
	cmd := a.command
	if a.prompt != "" {
		cmd = cmd + " " + shellQuote(a.prompt)
	}
	return a.sp.Start(a.sessionName, session.Config{Command: cmd, Env: a.env})
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// using the standard shell idiom: replace ' with '\”.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
