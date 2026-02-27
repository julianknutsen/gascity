// Package gastown_test validates the Gas Town example configuration.
//
// This test ensures the example stays valid as the SDK evolves:
// city.toml parses and validates, all formulas parse, and all
// prompt template files referenced by agents exist on disk.
package gastown_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/formula"
	"github.com/steveyegge/gascity/internal/fsys"
)

func exampleDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

func TestCityTomlParses(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Workspace.Name != "gastown" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "gastown")
	}
	if len(cfg.Agents) == 0 {
		t.Fatal("no agents configured")
	}
}

func TestCityTomlValidates(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if err := config.ValidateAgents(cfg.Agents); err != nil {
		t.Errorf("ValidateAgents: %v", err)
	}
}

func TestPromptFilesExist(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	for _, a := range cfg.Agents {
		if a.PromptTemplate == "" {
			continue
		}
		path := filepath.Join(dir, a.PromptTemplate)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("agent %q: prompt_template %q: %v", a.Name, a.PromptTemplate, err)
		}
	}
}

func TestAllFormulasParseAndValidate(t *testing.T) {
	dir := exampleDir()
	formulaDir := filepath.Join(dir, "formulas")

	entries, err := os.ReadDir(formulaDir)
	if err != nil {
		t.Fatalf("reading formulas dir: %v", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".formula.toml") {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(formulaDir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}
			f, err := formula.Parse(data)
			if err != nil {
				t.Fatalf("Parse(%s): %v", e.Name(), err)
			}
			if err := formula.Validate(f); err != nil {
				t.Errorf("Validate(%s): %v", e.Name(), err)
			}
		})
	}

	if count != 8 {
		t.Errorf("found %d formula files, want 8", count)
	}
}

func TestAllPromptTemplatesExist(t *testing.T) {
	dir := exampleDir()
	promptDir := filepath.Join(dir, "prompts")

	entries, err := os.ReadDir(promptDir)
	if err != nil {
		t.Fatalf("reading prompts dir: %v", err)
	}

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md.tmpl") {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(promptDir, e.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}
			if len(data) == 0 {
				t.Errorf("%s is empty", e.Name())
			}
		})
	}

	if count != 8 {
		t.Errorf("found %d prompt template files, want 8", count)
	}
}

func TestAgentNudgeField(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	// Verify nudge is populated for agents that have it.
	nudgeCounts := 0
	for _, a := range cfg.Agents {
		if a.Nudge != "" {
			nudgeCounts++
		}
	}
	if nudgeCounts == 0 {
		t.Error("no agents have nudge configured")
	}
}

func TestFormulasDir(t *testing.T) {
	dir := exampleDir()
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(dir, "city.toml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Formulas.Dir != "formulas" {
		t.Errorf("Formulas.Dir = %q, want %q", cfg.Formulas.Dir, "formulas")
	}
}
