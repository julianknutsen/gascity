package beads

import (
	"fmt"
	"sync"
	"time"
)

// MemStore is an in-memory Store implementation backed by a slice. It is
// exported for use as a test double in cross-package tests. It is safe for
// concurrent use.
type MemStore struct {
	mu    sync.Mutex
	beads []Bead
	seq   int
}

// NewMemStore returns a new empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{}
}

// NewMemStoreFrom returns a MemStore seeded with existing beads and sequence
// counter. Used by FileStore to restore state from disk.
func NewMemStoreFrom(seq int, existing []Bead) *MemStore {
	b := make([]Bead, len(existing))
	copy(b, existing)
	return &MemStore{seq: seq, beads: b}
}

// snapshot returns the current sequence counter and a copy of all beads.
// Used by FileStore for serialization. Caller must hold m.mu.
func (m *MemStore) snapshot() (int, []Bead) {
	b := make([]Bead, len(m.beads))
	copy(b, m.beads)
	return m.seq, b
}

// Create persists a new bead in memory with a sequential ID.
func (m *MemStore) Create(b Bead) (Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.seq++
	b.ID = fmt.Sprintf("gc-%d", m.seq)
	b.Status = "open"
	if b.Type == "" {
		b.Type = "task"
	}
	b.CreatedAt = time.Now()

	m.beads = append(m.beads, b)
	return b, nil
}

// Update modifies fields of an existing bead. Only non-nil fields in opts
// are applied. Returns a wrapped ErrNotFound if the ID does not exist.
func (m *MemStore) Update(id string, opts UpdateOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.beads {
		if m.beads[i].ID == id {
			if opts.Description != nil {
				m.beads[i].Description = *opts.Description
			}
			if opts.ParentID != nil {
				m.beads[i].ParentID = *opts.ParentID
			}
			return nil
		}
	}
	return fmt.Errorf("updating bead %q: %w", id, ErrNotFound)
}

// Close sets a bead's status to "closed". Returns a wrapped ErrNotFound if
// the ID does not exist. Closing an already-closed bead is a no-op.
func (m *MemStore) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.beads {
		if m.beads[i].ID == id {
			m.beads[i].Status = "closed"
			return nil
		}
	}
	return fmt.Errorf("closing bead %q: %w", id, ErrNotFound)
}

// List returns all beads in creation order.
func (m *MemStore) List() ([]Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Bead, len(m.beads))
	copy(result, m.beads)
	return result, nil
}

// Ready returns all beads with status "open", in creation order.
func (m *MemStore) Ready() ([]Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Bead
	for _, b := range m.beads {
		if b.Status == "open" {
			result = append(result, b)
		}
	}
	return result, nil
}

// Get retrieves a bead by ID. Returns a wrapped ErrNotFound if the ID does
// not exist.
func (m *MemStore) Get(id string) (Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, b := range m.beads {
		if b.ID == id {
			return b, nil
		}
	}
	return Bead{}, fmt.Errorf("getting bead %q: %w", id, ErrNotFound)
}

// Children returns all beads whose ParentID matches the given ID, in creation
// order.
func (m *MemStore) Children(parentID string) ([]Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []Bead
	for _, b := range m.beads {
		if b.ParentID == parentID {
			result = append(result, b)
		}
	}
	return result, nil
}
