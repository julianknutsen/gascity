// Package agent provides the Agent interface for managed agent lifecycle.
//
// An Agent encapsulates identity (name, session name) and lifecycle
// operations (start, stop, attach) backed by a [session.Provider].
// The CLI layer builds agents from config; the do* functions operate
// on them without knowing how sessions are implemented.
package agent

import "github.com/steveyegge/gascity/internal/session"

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
func New(name, sessionName, command string, sp session.Provider) Agent {
	return &managed{
		name:        name,
		sessionName: sessionName,
		command:     command,
		sp:          sp,
	}
}

// managed is the concrete Agent implementation that delegates to a
// session.Provider using the agent's session name.
type managed struct {
	name        string
	sessionName string
	command     string
	sp          session.Provider
}

func (a *managed) Name() string        { return a.name }
func (a *managed) SessionName() string { return a.sessionName }
func (a *managed) IsRunning() bool     { return a.sp.IsRunning(a.sessionName) }
func (a *managed) Stop() error         { return a.sp.Stop(a.sessionName) }
func (a *managed) Attach() error       { return a.sp.Attach(a.sessionName) }

func (a *managed) Start() error {
	return a.sp.Start(a.sessionName, session.Config{Command: a.command})
}
