// Package session defines the interface for agent session management.
//
// Callers depend on [Provider] for lifecycle and attach operations.
// The tmux subpackage provides the production implementation;
// [Fake] provides a test double with spy capabilities.
package session

// Provider manages agent sessions. Implementations handle the details
// of creating, destroying, and connecting to running agent processes.
type Provider interface {
	// Start creates a new session with the given name and configuration.
	// Returns an error if a session with that name already exists.
	Start(name string, cfg Config) error

	// Stop destroys the named session and cleans up its resources.
	// Returns nil if the session does not exist (idempotent).
	Stop(name string) error

	// IsRunning reports whether the named session exists and has a
	// live process.
	IsRunning(name string) bool

	// Attach connects the user's terminal to the named session for
	// interactive use. Blocks until the user detaches.
	Attach(name string) error
}

// Config holds the parameters for starting a new session.
type Config struct {
	// WorkDir is the working directory for the session process.
	WorkDir string

	// Command is the shell command to run in the session.
	// If empty, a default shell is started.
	Command string

	// Env is additional environment variables set in the session.
	Env map[string]string
}
