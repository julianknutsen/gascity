// Package formula provides parsing and runtime helpers for Gas City formulas.
//
// A formula is a TOML file that defines a sequence of named steps with
// dependency ordering. At runtime, a formula is instantiated as a molecule:
// a root bead plus one step bead per step. The runtime tracks progress by
// closing step beads and computing the current step from dependency state.
package formula

import (
	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/beads"
)

// Formula is a parsed formula definition from a *.formula.toml file.
type Formula struct {
	// Name is the unique identifier for this formula.
	Name string `toml:"formula" jsonschema:"required"`
	// Description explains what this formula does.
	Description string `toml:"description,omitempty"`
	// Steps defines the ordered sequence of work items in this formula.
	Steps []Step `toml:"steps" jsonschema:"minItems=1"`
}

// Step is one step in a formula.
type Step struct {
	// ID is the unique identifier for this step within the formula.
	ID string `toml:"id" jsonschema:"required"`
	// Title is a short human-readable label for this step.
	Title string `toml:"title" jsonschema:"required"`
	// Description provides detailed instructions for this step.
	Description string `toml:"description,omitempty"`
	// Needs lists step IDs that must complete before this step can start.
	Needs []string `toml:"needs,omitempty"`
}

// Parse decodes TOML data into a Formula.
func Parse(data []byte) (*Formula, error) {
	var f Formula
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// CurrentStep returns the first open step bead whose needs are all closed.
// Returns nil when all steps are complete or none are ready.
func CurrentStep(steps []beads.Bead) *beads.Bead {
	// Build a set of closed step refs for dependency checking.
	closed := make(map[string]bool)
	for _, s := range steps {
		if s.Status == "closed" {
			closed[s.Ref] = true
		}
	}

	for i := range steps {
		if steps[i].Status == "closed" {
			continue
		}
		ready := true
		for _, need := range steps[i].Needs {
			if !closed[need] {
				ready = false
				break
			}
		}
		if ready {
			return &steps[i]
		}
	}
	return nil
}

// CompletedCount returns the number of closed step beads.
func CompletedCount(steps []beads.Bead) int {
	n := 0
	for _, s := range steps {
		if s.Status == "closed" {
			n++
		}
	}
	return n
}

// StepIndex returns the 1-based position of a step by ref. Returns 0 if
// not found.
func StepIndex(steps []beads.Bead, ref string) int {
	for i, s := range steps {
		if s.Ref == ref {
			return i + 1
		}
	}
	return 0
}
