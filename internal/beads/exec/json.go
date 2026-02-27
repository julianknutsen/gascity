// Package exec implements [beads.Store] by delegating each operation to
// a user-supplied script via fork/exec. This follows the same pattern as
// the session exec provider: a single script receives the operation name
// as its first argument and communicates via stdin/stdout JSON.
package exec

import (
	"encoding/json"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
)

// createRequest is the JSON wire format sent on stdin for create operations.
// Intentionally separate from [beads.Bead] to own the serialization contract.
type createRequest struct {
	Title       string   `json:"title"`
	Type        string   `json:"type,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	ParentID    string   `json:"parent_id,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Needs       []string `json:"needs,omitempty"`
	Description string   `json:"description,omitempty"`
}

// updateRequest is the JSON wire format sent on stdin for update operations.
// Null/missing fields are not applied. Labels appends (does not replace).
type updateRequest struct {
	Description *string  `json:"description,omitempty"`
	ParentID    *string  `json:"parent_id,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// molCookRequest is the JSON wire format sent on stdin for mol-cook.
type molCookRequest struct {
	Formula string   `json:"formula"`
	Title   string   `json:"title,omitempty"`
	Vars    []string `json:"vars,omitempty"`
}

// beadWire is the JSON wire format returned by the script for bead data.
// Matches [beads.Bead] JSON tags — the same shape that bd already produces.
type beadWire struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	Assignee    string    `json:"assignee"`
	ParentID    string    `json:"parent_id"`
	Ref         string    `json:"ref"`
	Needs       []string  `json:"needs"`
	Description string    `json:"description"`
	Labels      []string  `json:"labels"`
}

// marshalCreate converts a Bead to JSON for the exec script's create operation.
func marshalCreate(b beads.Bead) ([]byte, error) {
	r := createRequest{
		Title:       b.Title,
		Type:        b.Type,
		Labels:      b.Labels,
		ParentID:    b.ParentID,
		Ref:         b.Ref,
		Needs:       b.Needs,
		Description: b.Description,
	}
	return json.Marshal(r)
}

// marshalUpdate converts update options to JSON for the exec script.
func marshalUpdate(description, parentID *string, labels []string) ([]byte, error) {
	r := updateRequest{
		Description: description,
		ParentID:    parentID,
		Labels:      labels,
	}
	return json.Marshal(r)
}

// marshalMolCook converts mol-cook parameters to JSON for the exec script.
func marshalMolCook(formula, title string, vars []string) ([]byte, error) {
	r := molCookRequest{
		Formula: formula,
		Title:   title,
		Vars:    vars,
	}
	return json.Marshal(r)
}
