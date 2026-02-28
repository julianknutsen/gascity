package automations

import (
	"fmt"
	"path/filepath"

	"github.com/steveyegge/gascity/internal/fsys"
)

// automationDir is the subdirectory name within formula layers that contains automations.
const automationDir = "automations"

// automationFileName is the expected filename inside each automation subdirectory.
const automationFileName = "automation.toml"

// Scan discovers automations across formula layers. For each layer dir, it scans
// <layer>/automations/*/automation.toml. Higher-priority layers (later in the slice)
// override lower by subdirectory name. Disabled automations and those in the skip
// list are excluded from results.
func Scan(fs fsys.FS, formulaLayers []string, skip []string) ([]Automation, error) {
	skipSet := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipSet[s] = true
	}

	// Scan layers lowest → highest priority. Later entries override earlier ones.
	found := make(map[string]Automation) // name → automation
	var order []string                   // preserve discovery order

	for _, layer := range formulaLayers {
		automationsRoot := filepath.Join(layer, automationDir)
		entries, err := fs.ReadDir(automationsRoot)
		if err != nil {
			continue // layer has no automations/ directory — skip
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			tomlPath := filepath.Join(automationsRoot, name, automationFileName)
			data, err := fs.ReadFile(tomlPath)
			if err != nil {
				continue // no automation.toml — skip
			}

			a, err := Parse(data)
			if err != nil {
				return nil, fmt.Errorf("automation %q in %s: %w", name, layer, err)
			}
			a.Name = name
			a.Source = tomlPath

			if _, exists := found[name]; !exists {
				order = append(order, name)
			}
			found[name] = a // higher-priority layer overwrites
		}
	}

	// Collect results, excluding disabled and skipped automations.
	var result []Automation
	for _, name := range order {
		a := found[name]
		if !a.IsEnabled() {
			continue
		}
		if skipSet[name] {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}
