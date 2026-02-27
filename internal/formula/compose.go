package formula

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gascity/internal/beads"
)

// Resolver loads a formula by name. Implementations typically read from
// a directory of *.formula.toml files.
type Resolver func(name string) (*Formula, error)

// SubstituteVars replaces {{key}} placeholders in step descriptions
// with values from vars (format: "key=value"). Returns a shallow copy
// of the formula with substituted descriptions; the original is not modified.
func SubstituteVars(f *Formula, vars []string) *Formula {
	if len(vars) == 0 {
		return f
	}

	// Parse key=value pairs.
	kv := make(map[string]string, len(vars))
	for _, v := range vars {
		if i := strings.IndexByte(v, '='); i >= 0 {
			kv[v[:i]] = v[i+1:]
		}
	}
	if len(kv) == 0 {
		return f
	}

	// Copy formula with substituted step descriptions.
	out := *f
	out.Steps = make([]Step, len(f.Steps))
	for i, s := range f.Steps {
		out.Steps[i] = s
		for k, val := range kv {
			out.Steps[i].Description = strings.ReplaceAll(out.Steps[i].Description, "{{"+k+"}}", val)
		}
	}
	return &out
}

// ComposeMolCook creates a molecule (root bead + step beads) using only
// Store.Create calls. Returns the root bead ID.
//
// Steps:
//  1. Resolve the formula by name.
//  2. Apply variable substitution to step descriptions.
//  3. Create a root bead (type "molecule", Ref = formula name).
//  4. Create one child bead per step (type "task", ParentID = root, Ref = step ID).
func ComposeMolCook(store beads.Store, resolver Resolver, formulaName, title string, vars []string) (string, error) {
	f, err := resolver(formulaName)
	if err != nil {
		return "", fmt.Errorf("composing molecule %q: %w", formulaName, err)
	}

	f = SubstituteVars(f, vars)

	if title == "" {
		title = formulaName
	}

	root, err := store.Create(beads.Bead{
		Title: title,
		Type:  "molecule",
		Ref:   formulaName,
	})
	if err != nil {
		return "", fmt.Errorf("composing molecule %q: creating root: %w", formulaName, err)
	}

	for _, step := range f.Steps {
		_, err := store.Create(beads.Bead{
			Title:       step.Title,
			Type:        "task",
			ParentID:    root.ID,
			Ref:         step.ID,
			Needs:       step.Needs,
			Description: step.Description,
		})
		if err != nil {
			return "", fmt.Errorf("composing molecule %q: creating step %q: %w", formulaName, step.ID, err)
		}
	}

	return root.ID, nil
}
