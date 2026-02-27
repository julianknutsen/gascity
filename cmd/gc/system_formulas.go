package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MaterializeSystemFormulas writes embedded system formula files to
// .gc/system-formulas/ in the city directory. Files are always overwritten
// to stay in sync with the gc binary version. Returns the directory path
// (for use as Layer 0), or "" if there are no embedded system formulas.
// Removes stale files that are no longer in the embedded FS.
// Idempotent: safe to call on every gc start.
func MaterializeSystemFormulas(embedded fs.FS, subdir, cityPath string) (string, error) {
	// Collect all formula files from the embedded FS.
	files := collectFormulaFiles(embedded, subdir)
	if len(files) == 0 {
		return "", nil
	}

	sysDir := filepath.Join(cityPath, ".gc", "system-formulas")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		return "", fmt.Errorf("creating system-formulas dir: %w", err)
	}

	// Write all embedded files (always overwrite to track binary version).
	written := make(map[string]bool)
	for _, relPath := range files {
		data, err := fs.ReadFile(embedded, filepath.Join(subdir, relPath))
		if err != nil {
			return "", fmt.Errorf("reading embedded %s: %w", relPath, err)
		}

		dst := filepath.Join(sysDir, relPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", fmt.Errorf("creating dir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return "", fmt.Errorf("writing %s: %w", relPath, err)
		}
		written[relPath] = true
	}

	// Remove stale formula files not in the embedded FS.
	removeStaleFormulas(sysDir, "", written)

	return sysDir, nil
}

// ListEmbeddedSystemFormulas returns the relative paths of all formula
// files in the embedded FS. Used by doctor check for staleness detection.
func ListEmbeddedSystemFormulas(embedded fs.FS, subdir string) []string {
	return collectFormulaFiles(embedded, subdir)
}

// collectFormulaFiles walks the embedded FS under subdir and returns
// relative paths of *.formula.toml files and plugins/*/plugin.toml files.
func collectFormulaFiles(embedded fs.FS, subdir string) []string {
	var files []string
	_ = fs.WalkDir(embedded, subdir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, subdir+"/")
		if rel == path {
			// path == subdir (root entry, not a file under subdir)
			return nil
		}
		if isFormulaFile(rel) {
			files = append(files, rel)
		}
		return nil
	})
	return files
}

// isFormulaFile returns true if the relative path is a formula or plugin file.
func isFormulaFile(rel string) bool {
	if strings.HasSuffix(rel, ".formula.toml") {
		return true
	}
	// plugins/<name>/plugin.toml
	if strings.HasPrefix(rel, "plugins/") && filepath.Base(rel) == "plugin.toml" {
		return true
	}
	return false
}

// removeStaleFormulas removes formula files in dir that are not in the
// written set. Only removes formula files, not arbitrary files.
func removeStaleFormulas(baseDir, prefix string, written map[string]bool) {
	dir := filepath.Join(baseDir, prefix)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		rel := filepath.Join(prefix, e.Name())
		if e.IsDir() {
			// Recurse into plugins/ subdirectories.
			removeStaleFormulas(baseDir, rel, written)
			continue
		}
		if !isFormulaFile(rel) {
			continue // Not a formula file — leave alone.
		}
		if !written[rel] {
			os.Remove(filepath.Join(baseDir, rel)) //nolint:errcheck // best-effort cleanup
		}
	}
}
