package plugins

import (
	"fmt"
	"path/filepath"

	"github.com/steveyegge/gascity/internal/fsys"
)

// pluginDir is the subdirectory name within formula layers that contains plugins.
const pluginDir = "plugins"

// pluginFile is the expected filename inside each plugin subdirectory.
const pluginFileName = "plugin.toml"

// Scan discovers plugins across formula layers. For each layer dir, it scans
// <layer>/plugins/*/plugin.toml. Higher-priority layers (later in the slice)
// override lower by subdirectory name. Disabled plugins and those in the skip
// list are excluded from results.
func Scan(fs fsys.FS, formulaLayers []string, skip []string) ([]Plugin, error) {
	skipSet := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipSet[s] = true
	}

	// Scan layers lowest → highest priority. Later entries override earlier ones.
	found := make(map[string]Plugin) // name → plugin
	var order []string               // preserve discovery order

	for _, layer := range formulaLayers {
		pluginsRoot := filepath.Join(layer, pluginDir)
		entries, err := fs.ReadDir(pluginsRoot)
		if err != nil {
			continue // layer has no plugins/ directory — skip
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			tomlPath := filepath.Join(pluginsRoot, name, pluginFileName)
			data, err := fs.ReadFile(tomlPath)
			if err != nil {
				continue // no plugin.toml — skip
			}

			p, err := Parse(data)
			if err != nil {
				return nil, fmt.Errorf("plugin %q in %s: %w", name, layer, err)
			}
			p.Name = name
			p.Source = tomlPath

			if _, exists := found[name]; !exists {
				order = append(order, name)
			}
			found[name] = p // higher-priority layer overwrites
		}
	}

	// Collect results, excluding disabled and skipped plugins.
	var result []Plugin
	for _, name := range order {
		p := found[name]
		if !p.IsEnabled() {
			continue
		}
		if skipSet[name] {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}
