package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFormulaFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFormulas_SingleLayer(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "formula a")
	writeFormulaFile(t, layer, "mol-b.formula.toml", "formula b")

	target := filepath.Join(dir, "rig")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("ResolveFormulas: %v", err)
	}

	symlinkDir := filepath.Join(target, ".beads", "formulas")
	for _, name := range []string{"mol-a.formula.toml", "mol-b.formula.toml"} {
		linkPath := filepath.Join(symlinkDir, name)
		fi, err := os.Lstat(linkPath)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: not a symlink", name)
		}
		dest, err := os.Readlink(linkPath)
		if err != nil {
			t.Errorf("%s: readlink: %v", name, err)
			continue
		}
		want := filepath.Join(layer, name)
		if dest != want {
			t.Errorf("%s: link target = %q, want %q", name, dest, want)
		}
	}
}

func TestResolveFormulas_Shadow(t *testing.T) {
	dir := t.TempDir()
	layer1 := filepath.Join(dir, "layer1")
	layer2 := filepath.Join(dir, "layer2")

	writeFormulaFile(t, layer1, "mol-a.formula.toml", "layer1 version")
	writeFormulaFile(t, layer1, "mol-b.formula.toml", "layer1 only")
	writeFormulaFile(t, layer2, "mol-a.formula.toml", "layer2 version")
	writeFormulaFile(t, layer2, "mol-c.formula.toml", "layer2 only")

	target := filepath.Join(dir, "rig")
	os.MkdirAll(target, 0o755) //nolint:errcheck

	if err := ResolveFormulas(target, []string{layer1, layer2}); err != nil {
		t.Fatalf("ResolveFormulas: %v", err)
	}

	symlinkDir := filepath.Join(target, ".beads", "formulas")

	// mol-a should point to layer2 (higher priority shadow).
	dest, err := os.Readlink(filepath.Join(symlinkDir, "mol-a.formula.toml"))
	if err != nil {
		t.Fatalf("mol-a readlink: %v", err)
	}
	if dest != filepath.Join(layer2, "mol-a.formula.toml") {
		t.Errorf("mol-a target = %q, want layer2 version", dest)
	}

	// mol-b should point to layer1 (only source).
	dest, err = os.Readlink(filepath.Join(symlinkDir, "mol-b.formula.toml"))
	if err != nil {
		t.Fatalf("mol-b readlink: %v", err)
	}
	if dest != filepath.Join(layer1, "mol-b.formula.toml") {
		t.Errorf("mol-b target = %q, want layer1 version", dest)
	}

	// mol-c should point to layer2 (only source).
	dest, err = os.Readlink(filepath.Join(symlinkDir, "mol-c.formula.toml"))
	if err != nil {
		t.Fatalf("mol-c readlink: %v", err)
	}
	if dest != filepath.Join(layer2, "mol-c.formula.toml") {
		t.Errorf("mol-c target = %q, want layer2 version", dest)
	}
}

func TestResolveFormulas_Idempotent(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "formula a")

	target := filepath.Join(dir, "rig")
	os.MkdirAll(target, 0o755) //nolint:errcheck

	// Run twice — should not error.
	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("first ResolveFormulas: %v", err)
	}
	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("second ResolveFormulas: %v", err)
	}

	// Symlink should still be correct.
	dest, err := os.Readlink(filepath.Join(target, ".beads", "formulas", "mol-a.formula.toml"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if dest != filepath.Join(layer, "mol-a.formula.toml") {
		t.Errorf("symlink target = %q, want %q", dest, filepath.Join(layer, "mol-a.formula.toml"))
	}
}

func TestResolveFormulas_StaleCleanup(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "formula a")
	writeFormulaFile(t, layer, "mol-b.formula.toml", "formula b")

	target := filepath.Join(dir, "rig")
	os.MkdirAll(target, 0o755) //nolint:errcheck

	// First pass: both formulas.
	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("first ResolveFormulas: %v", err)
	}

	// Remove mol-b from the layer.
	os.Remove(filepath.Join(layer, "mol-b.formula.toml")) //nolint:errcheck

	// Second pass: only mol-a.
	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("second ResolveFormulas: %v", err)
	}

	symlinkDir := filepath.Join(target, ".beads", "formulas")

	// mol-a should still exist.
	if _, err := os.Lstat(filepath.Join(symlinkDir, "mol-a.formula.toml")); err != nil {
		t.Errorf("mol-a should still exist: %v", err)
	}

	// mol-b should be removed (stale).
	if _, err := os.Lstat(filepath.Join(symlinkDir, "mol-b.formula.toml")); !os.IsNotExist(err) {
		t.Error("mol-b should have been removed (stale symlink)")
	}
}

func TestResolveFormulas_RealFileNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "layer version")

	target := filepath.Join(dir, "rig")
	symlinkDir := filepath.Join(target, ".beads", "formulas")
	os.MkdirAll(symlinkDir, 0o755) //nolint:errcheck

	// Create a real file (not a symlink) in the target.
	realFile := filepath.Join(symlinkDir, "mol-a.formula.toml")
	os.WriteFile(realFile, []byte("real file"), 0o644) //nolint:errcheck

	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("ResolveFormulas: %v", err)
	}

	// The real file should be preserved (not replaced with symlink).
	fi, err := os.Lstat(realFile)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("real file should not have been replaced with symlink")
	}
	content, err := os.ReadFile(realFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "real file" {
		t.Errorf("real file content = %q, want %q", content, "real file")
	}
}

func TestResolveFormulas_EmptyLayers(t *testing.T) {
	if err := ResolveFormulas("/tmp/nonexistent", nil); err != nil {
		t.Errorf("nil layers should be no-op: %v", err)
	}
	if err := ResolveFormulas("/tmp/nonexistent", []string{}); err != nil {
		t.Errorf("empty layers should be no-op: %v", err)
	}
}

func TestResolveFormulas_MissingLayerDir(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "formula a")

	target := filepath.Join(dir, "rig")
	os.MkdirAll(target, 0o755) //nolint:errcheck

	// Include a missing dir — should be skipped, not error.
	if err := ResolveFormulas(target, []string{"/nonexistent", layer}); err != nil {
		t.Fatalf("ResolveFormulas: %v", err)
	}

	// mol-a from the existing layer should still be resolved.
	if _, err := os.Lstat(filepath.Join(target, ".beads", "formulas", "mol-a.formula.toml")); err != nil {
		t.Errorf("mol-a should exist: %v", err)
	}
}

func TestResolveFormulas_NonFormulaFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "formulas")
	writeFormulaFile(t, layer, "mol-a.formula.toml", "formula")
	writeFormulaFile(t, layer, "readme.md", "not a formula")
	writeFormulaFile(t, layer, "config.toml", "not a formula")

	target := filepath.Join(dir, "rig")
	os.MkdirAll(target, 0o755) //nolint:errcheck

	if err := ResolveFormulas(target, []string{layer}); err != nil {
		t.Fatalf("ResolveFormulas: %v", err)
	}

	symlinkDir := filepath.Join(target, ".beads", "formulas")
	entries, err := os.ReadDir(symlinkDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (only .formula.toml files)", len(entries))
	}
}
