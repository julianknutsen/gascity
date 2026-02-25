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

	// Interrupt sends a soft interrupt signal (e.g., Ctrl-C / SIGINT) to
	// the named session. Best-effort: returns nil if the session doesn't
	// exist. Used for graceful shutdown before Stop.
	Interrupt(name string) error

	// IsRunning reports whether the named session exists and has a
	// live process.
	IsRunning(name string) bool

	// Attach connects the user's terminal to the named session for
	// interactive use. Blocks until the user detaches.
	Attach(name string) error

	// ProcessAlive reports whether the named session has a live agent
	// process matching one of the given names in its process tree.
	// Returns true if processNames is empty (no check possible).
	ProcessAlive(name string, processNames []string) bool

	// Nudge sends a message to the named session to wake or redirect
	// the agent. Returns nil if the session does not exist (best-effort).
	Nudge(name, message string) error

	// SetMeta stores a key-value pair associated with the named session.
	// Used for drain signaling and config fingerprint storage.
	SetMeta(name, key, value string) error

	// GetMeta retrieves a previously stored metadata value.
	// Returns ("", nil) if the key is not set.
	GetMeta(name, key string) (string, error)

	// RemoveMeta removes a metadata key from the named session.
	RemoveMeta(name, key string) error

	// Peek captures the last N lines of output from the named session.
	// If lines <= 0, captures all available scrollback.
	Peek(name string, lines int) (string, error)

	// ListRunning returns the names of all running sessions whose names
	// have the given prefix. Used for orphan detection.
	ListRunning(prefix string) ([]string, error)
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

	// Startup reliability hints (all optional — zero values skip).

	// ReadyPromptPrefix is the prompt prefix for readiness detection (e.g. "> ").
	ReadyPromptPrefix string

	// ReadyDelayMs is a fallback fixed delay when no prompt prefix is available.
	ReadyDelayMs int

	// ProcessNames lists expected process names for liveness checks.
	ProcessNames []string

	// EmitsPermissionWarning is true if the agent shows a bypass-permissions dialog.
	EmitsPermissionWarning bool

	// Nudge is text typed into the session after the agent is ready.
	// Used for CLI agents that don't accept command-line prompts.
	Nudge string
}
