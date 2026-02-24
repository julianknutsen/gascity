// Package docgen generates JSON Schema and markdown documentation from
// Gas City's Go config structs.
package docgen

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/formula"
)

// ModuleRoot finds the repo root by walking up from the current directory
// looking for go.mod. Returns the absolute path.
func ModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent of %s", dir)
		}
		dir = parent
	}
}

// newReflector creates a jsonschema.Reflector configured for TOML field
// names with Go doc comments extracted from the source tree.
//
// AddGoComments requires the path parameter to be "." with the working
// directory set to the module root, so that filepath.Walk produces paths
// like "internal/config" which gopath.Join maps to the correct import path.
func newReflector() (*jsonschema.Reflector, error) {
	root, err := ModuleRoot()
	if err != nil {
		return nil, err
	}

	// Save and restore CWD — AddGoComments needs CWD at module root.
	orig, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}
	if err := os.Chdir(root); err != nil {
		return nil, fmt.Errorf("chdir to module root: %w", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	r := &jsonschema.Reflector{
		FieldNameTag: "toml",
	}
	if err := r.AddGoComments("github.com/steveyegge/gascity", "."); err != nil {
		return nil, fmt.Errorf("extracting Go comments: %w", err)
	}
	return r, nil
}

// GenerateCitySchema produces a JSON Schema for the city.toml config format.
// It reflects the config.City struct using TOML field names and extracts
// doc comments as descriptions.
func GenerateCitySchema() (*jsonschema.Schema, error) {
	r, err := newReflector()
	if err != nil {
		return nil, err
	}
	s := r.Reflect(&config.City{})
	s.Title = "Gas City Configuration"
	s.Description = "Schema for city.toml — the top-level configuration file for a Gas City instance."
	return s, nil
}

// GenerateFormulaSchema produces a JSON Schema for *.formula.toml files.
func GenerateFormulaSchema() (*jsonschema.Schema, error) {
	r, err := newReflector()
	if err != nil {
		return nil, err
	}
	s := r.Reflect(&formula.Formula{})
	s.Title = "Gas City Formula"
	s.Description = "Schema for *.formula.toml — a formula definition file."
	return s, nil
}
