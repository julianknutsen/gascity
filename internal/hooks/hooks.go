// Package hooks installs provider-specific agent hook files into working
// directories. Each provider (Claude, Gemini, OpenCode, Copilot) has its own
// file format and install location. Hook files are embedded at build time
// and written idempotently — existing files are never overwritten.
package hooks

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gascity/internal/fsys"
)

//go:embed config/*
var configFS embed.FS

// supported lists provider names that have hook support.
var supported = []string{"claude", "gemini", "opencode", "copilot"}

// unsupported lists provider names that have no hook mechanism.
var unsupported = []string{"codex", "cursor", "amp"}

// SupportedProviders returns the list of provider names with hook support.
func SupportedProviders() []string {
	out := make([]string, len(supported))
	copy(out, supported)
	return out
}

// Validate checks that all provider names are supported for hook installation.
// Returns an error listing any unsupported names.
func Validate(providers []string) error {
	sup := make(map[string]bool, len(supported))
	for _, s := range supported {
		sup[s] = true
	}
	noHook := make(map[string]bool, len(unsupported))
	for _, u := range unsupported {
		noHook[u] = true
	}
	var bad []string
	for _, p := range providers {
		if !sup[p] {
			if noHook[p] {
				bad = append(bad, fmt.Sprintf("%s (no hook mechanism)", p))
			} else {
				bad = append(bad, fmt.Sprintf("%s (unknown)", p))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("unsupported install_agent_hooks: %s; supported: %s",
			strings.Join(bad, ", "), strings.Join(supported, ", "))
	}
	return nil
}

// Install writes hook files for the given providers. cityDir is the city root
// (used for city-wide files like Claude settings). workDir is the agent's
// working directory (used for per-project files like Gemini, OpenCode, Copilot).
// Idempotent — existing files are not overwritten.
func Install(fs fsys.FS, cityDir, workDir string, providers []string) error {
	for _, p := range providers {
		var err error
		switch p {
		case "claude":
			err = installClaude(fs, cityDir)
		case "gemini":
			err = installGemini(fs, workDir)
		case "opencode":
			err = installOpenCode(fs, workDir)
		case "copilot":
			err = installCopilot(fs, workDir)
		default:
			return fmt.Errorf("unsupported hook provider %q", p)
		}
		if err != nil {
			return fmt.Errorf("installing %s hooks: %w", p, err)
		}
	}
	return nil
}

// installClaude writes .gc/settings.json in the city directory.
func installClaude(fs fsys.FS, cityDir string) error {
	dst := filepath.Join(cityDir, ".gc", "settings.json")
	return writeEmbedded(fs, "config/claude.json", dst)
}

// installGemini writes .gemini/settings.json in the working directory.
func installGemini(fs fsys.FS, workDir string) error {
	dst := filepath.Join(workDir, ".gemini", "settings.json")
	return writeEmbedded(fs, "config/gemini.json", dst)
}

// installOpenCode writes .opencode/plugins/gascity.js in the working directory.
func installOpenCode(fs fsys.FS, workDir string) error {
	dst := filepath.Join(workDir, ".opencode", "plugins", "gascity.js")
	return writeEmbedded(fs, "config/opencode.js", dst)
}

// installCopilot writes .github/copilot-instructions.md in the working directory.
func installCopilot(fs fsys.FS, workDir string) error {
	dst := filepath.Join(workDir, ".github", "copilot-instructions.md")
	return writeEmbedded(fs, "config/copilot.md", dst)
}

// writeEmbedded reads an embedded file and writes it to dst, creating parent
// directories as needed. Skips if dst already exists.
func writeEmbedded(fs fsys.FS, embedPath, dst string) error {
	// Idempotent: skip if file exists.
	if _, err := fs.Stat(dst); err == nil {
		return nil
	}

	data, err := configFS.ReadFile(embedPath)
	if err != nil {
		return fmt.Errorf("reading embedded %s: %w", embedPath, err)
	}

	dir := filepath.Dir(dst)
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	if err := fs.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return nil
}
