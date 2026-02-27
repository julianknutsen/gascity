package formula

import (
	"fmt"
	"os"
	"path/filepath"
)

// DirResolver returns a Resolver that loads formulas from *.formula.toml
// files in the given directory. The formula name maps to
// <dir>/<name>.formula.toml.
func DirResolver(dir string) Resolver {
	return func(name string) (*Formula, error) {
		path := filepath.Join(dir, name+".formula.toml")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("loading formula %q: %w", name, err)
		}
		f, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parsing formula %q: %w", name, err)
		}
		if err := Validate(f); err != nil {
			return nil, fmt.Errorf("validating formula %q: %w", name, err)
		}
		return f, nil
	}
}
