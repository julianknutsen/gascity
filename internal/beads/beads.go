// Package beads provides the bead store abstraction — the universal persistence
// substrate for Gas City work units (tasks, messages, molecules, etc.).
package beads

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a bead ID does not exist in the store.
var ErrNotFound = errors.New("bead not found")

// ErrAlreadyClaimed is returned when a bead is already claimed by a different agent.
var ErrAlreadyClaimed = errors.New("bead already claimed by another agent")

// Bead is a single unit of work in Gas City. Everything is a bead: tasks,
// mail, molecules, convoys.
type Bead struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"` // "open", "in_progress", "closed"
	Type        string    `json:"type"`   // "task" default
	CreatedAt   time.Time `json:"created_at"`
	Assignee    string    `json:"assignee,omitempty"`
	From        string    `json:"from,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`   // step → molecule
	Ref         string    `json:"ref,omitempty"`         // formula step ID or formula name
	Needs       []string  `json:"needs,omitempty"`       // dependency step refs
	Description string    `json:"description,omitempty"` // step instructions
}

// UpdateOpts specifies which fields to change. Nil pointers are skipped.
type UpdateOpts struct {
	Description *string
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

	// Update modifies fields of an existing bead. Only non-nil fields in opts
	// are applied. Returns ErrNotFound if the bead does not exist.
	Update(id string, opts UpdateOpts) error

	// Close sets a bead's status to "closed". Returns ErrNotFound if the ID
	// does not exist. Closing an already-closed bead is a no-op.
	Close(id string) error

	// Claim atomically assigns a bead to an agent. Sets status to
	// "in_progress" and assignee to the given agent name. Returns
	// ErrNotFound if the bead does not exist, or ErrAlreadyClaimed if
	// the bead is already claimed by a different agent. Claiming the
	// same bead by the same agent is idempotent (no-op).
	Claim(id, assignee string) error

	// List returns all beads. In-process stores (MemStore, FileStore)
	// return creation order; external stores (BdStore) may not guarantee
	// order when beads share the same second-precision timestamp.
	List() ([]Bead, error)

	// Ready returns all beads with status "open". Same ordering note
	// as List.
	Ready() ([]Bead, error)

	// Claimed returns the bead currently claimed by the given agent.
	// Returns ErrNotFound (possibly wrapped) if no bead is claimed
	// by this agent.
	Claimed(assignee string) (Bead, error)

	// Children returns all beads whose ParentID matches the given ID,
	// in creation order.
	Children(parentID string) ([]Bead, error)
}
