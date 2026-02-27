package formula

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirResolver(t *testing.T) {
	dir := t.TempDir()
	toml := `
formula = "pancakes"
[[steps]]
id = "mix"
title = "Mix ingredients"
`
	if err := os.WriteFile(filepath.Join(dir, "pancakes.formula.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := DirResolver(dir)
	f, err := resolver("pancakes")
	if err != nil {
		t.Fatalf("DirResolver: %v", err)
	}
	if f.Name != "pancakes" {
		t.Errorf("Name = %q, want %q", f.Name, "pancakes")
	}
	if len(f.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(f.Steps))
	}
	if f.Steps[0].ID != "mix" {
		t.Errorf("Steps[0].ID = %q, want %q", f.Steps[0].ID, "mix")
	}
}

func TestDirResolverNotFound(t *testing.T) {
	dir := t.TempDir()
	resolver := DirResolver(dir)

	_, err := resolver("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing formula")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error = %q, want to contain formula name", err)
	}
}

func TestDirResolverInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.formula.toml"), []byte(`{{not toml}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := DirResolver(dir)
	_, err := resolver("bad")
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %q, want to contain 'parsing'", err)
	}
}

func TestDirResolverValidationError(t *testing.T) {
	dir := t.TempDir()
	// Valid TOML but invalid formula (no steps).
	toml := `formula = "empty"`
	if err := os.WriteFile(filepath.Join(dir, "empty.formula.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := DirResolver(dir)
	_, err := resolver("empty")
	if err == nil {
		t.Fatal("expected error for invalid formula")
	}
	if !strings.Contains(err.Error(), "validating") {
		t.Errorf("error = %q, want to contain 'validating'", err)
	}
}
