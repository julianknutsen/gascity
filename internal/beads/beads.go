// Package beads provides the bead store abstraction — the universal persistence
// substrate for Gas City work units (tasks, messages, molecules, etc.).
package beads

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a bead ID does not exist in the store.
var ErrNotFound = errors.New("bead not found")

// Bead is a single unit of work in Gas City. Everything is a bead: tasks,
// mail, molecules, convoys.
type Bead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"` // "open", "hooked", "closed"
	Type      string    `json:"type"`   // "task" default
	CreatedAt time.Time `json:"created_at"`
	Assignee  string    `json:"assignee,omitempty"`
}

// Store is the interface for bead persistence. Implementations must assign
// sequential IDs (gc-1, gc-2, ...), default Status to "open", default Type
// to "task", and set CreatedAt on Create.
type Store interface {
	// Create persists a new bead. The caller provides Title and optionally
	// Type; the store fills in ID, Status, and CreatedAt. Returns the
	// complete bead.
	Create(b Bead) (Bead, error)

	// Get retrieves a bead by ID. Returns ErrNotFound (possibly wrapped)
	// if the ID does not exist.
	Get(id string) (Bead, error)

	// Ready returns all beads with status "open", in creation order.
	Ready() ([]Bead, error)
}
