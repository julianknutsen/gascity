// Package beads provides the bead store abstraction — the universal persistence
// substrate for Gas City work units (tasks, messages, molecules, etc.).
package beads

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a bead ID does not exist in the store.
var ErrNotFound = errors.New("bead not found")

// ErrConflict is returned when a bead is already hooked to a different agent.
var ErrConflict = errors.New("bead already hooked to another agent")

// ErrAgentBusy is returned when an agent already has a bead on their hook.
var ErrAgentBusy = errors.New("agent already has a hooked bead")

// Bead is a single unit of work in Gas City. Everything is a bead: tasks,
// mail, molecules, convoys.
type Bead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"` // "open", "hooked", "closed"
	Type      string    `json:"type"`   // "task" default
	CreatedAt time.Time `json:"created_at"`
	Assignee  string    `json:"assignee,omitempty"`
	From      string    `json:"from,omitempty"`
}

// Store is the interface for bead persistence. Implementations must assign
// unique non-empty IDs, default Status to "open", default Type to "task",
// and set CreatedAt on Create. The ID format is implementation-specific
// (e.g. "gc-1" for FileStore, "bd-XXXX" for BdStore).
type Store interface {
	// Create persists a new bead. The caller provides Title and optionally
	// Type; the store fills in ID, Status, and CreatedAt. Returns the
	// complete bead.
	Create(b Bead) (Bead, error)

	// Get retrieves a bead by ID. Returns ErrNotFound (possibly wrapped)
	// if the ID does not exist.
	Get(id string) (Bead, error)

	// Close sets a bead's status to "closed". Returns ErrNotFound if the ID
	// does not exist. Closing an already-closed bead is a no-op.
	Close(id string) error

	// Hook assigns a bead to an agent. Returns ErrNotFound if the bead
	// does not exist, ErrConflict if the bead is already hooked to a
	// different agent, or ErrAgentBusy if the agent already has another
	// bead on their hook. Hooking the same bead to the same agent is
	// idempotent (no-op).
	Hook(id, assignee string) error

	// List returns all beads. In-process stores (MemStore, FileStore)
	// return creation order; external stores (BdStore) may not guarantee
	// order when beads share the same second-precision timestamp.
	List() ([]Bead, error)

	// Ready returns all beads with status "open". Same ordering note
	// as List.
	Ready() ([]Bead, error)

	// Hooked returns the bead currently hooked to the given agent.
	// Returns ErrNotFound (possibly wrapped) if no bead is hooked
	// to this agent.
	Hooked(assignee string) (Bead, error)
}
