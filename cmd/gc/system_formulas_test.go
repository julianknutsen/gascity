package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
)

// --- MaterializeSystemFormulas ---

func TestMaterializeEmpty(t *testing.T) {
	cityPath := t.TempDir()
	fs := fstest.MapFS{}

	dir, err := MaterializeSystemFormulas(fs, ".", cityPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "" {
		t.Errorf("expected empty dir, got %q", dir)
	}
	// .gc/system-formulas/ should not exist.
	sysDir := filepath.Join(cityPath, ".gc", "system-formulas")
	if _, err := os.Stat(sysDir); !os.IsNotExist(err) {
		t.Errorf("system-formulas dir should not exist for empty FS")
	}
}

func TestMaterializeWritesFiles(t *testing.T) {
	cityPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cityPath, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	fs := fstest.MapFS{
		"sysformulas/hello.formula.toml": &fstest.MapFile{Data: []byte("[formula]\nname = \"hello\"\n")},
	}

	dir, err := MaterializeSystemFormulas(fs, "sysformulas", cityPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(cityPath, ".gc", "system-formulas")
	if dir != expected {
		t.Errorf("dir = %q, want %q", dir, expected)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.formula.toml"))
	if err != nil {
		t.Fatalf("reading materialized file: %v", err)
	}
	if string(data) != "[formula]\nname = \"hello\"\n" {
		t.Errorf("content = %q", string(data))
	}
}

func TestMaterializeOverwrites(t *testing.T) {
	cityPath := t.TempDir()
	sysDir := filepath.Join(cityPath, ".gc", "system-formulas")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "hello.formula.toml"), []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := fstest.MapFS{
		"sf/hello.formula.toml": &fstest.MapFile{Data: []byte("new content")},
	}

	dir, err := MaterializeSystemFormulas(fs, "sf", cityPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.formula.toml"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("content = %q, want %q", string(data), "new content")
	}
}

func TestMaterializeCleansRemoved(t *testing.T) {
	cityPath := t.TempDir()
	sysDir := filepath.Join(cityPath, ".gc", "system-formulas")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing file that is NOT in the new embedded FS.
	if err := os.WriteFile(filepath.Join(sysDir, "stale.formula.toml"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also a non-formula file that should be left alone.
	if err := os.WriteFile(filepath.Join(sysDir, "readme.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := fstest.MapFS{
		"sf/fresh.formula.toml": &fstest.MapFile{Data: []byte("fresh")},
	}

	_, err := MaterializeSystemFormulas(fs, "sf", cityPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stale.formula.toml should be removed.
	if _, err := os.Stat(filepath.Join(sysDir, "stale.formula.toml")); !os.IsNotExist(err) {
		t.Error("stale formula file was not removed")
	}
	// fresh.formula.toml should exist.
	if _, err := os.Stat(filepath.Join(sysDir, "fresh.formula.toml")); err != nil {
		t.Error("fresh formula file missing")
	}
	// readme.txt should still exist.
	if _, err := os.Stat(filepath.Join(sysDir, "readme.txt")); err != nil {
		t.Error("non-formula file was removed")
	}
}

func TestMaterializeIdempotent(t *testing.T) {
	cityPath := t.TempDir()

	fs := fstest.MapFS{
		"sf/a.formula.toml": &fstest.MapFile{Data: []byte("aaa")},
	}

	dir1, err := MaterializeSystemFormulas(fs, "sf", cityPath)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	dir2, err := MaterializeSystemFormulas(fs, "sf", cityPath)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if dir1 != dir2 {
		t.Errorf("dir changed: %q vs %q", dir1, dir2)
	}
	data, err := os.ReadFile(filepath.Join(dir2, "a.formula.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "aaa" {
		t.Errorf("content after second call = %q", string(data))
	}
}

func TestMaterializeWithAutomations(t *testing.T) {
	cityPath := t.TempDir()

	fs := fstest.MapFS{
		"sf/basic.formula.toml":              &fstest.MapFile{Data: []byte("basic")},
		"sf/automations/foo/automation.toml": &fstest.MapFile{Data: []byte("foo automation")},
		"sf/automations/bar/automation.toml": &fstest.MapFile{Data: []byte("bar automation")},
	}

	dir, err := MaterializeSystemFormulas(fs, "sf", cityPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check basic formula.
	data, err := os.ReadFile(filepath.Join(dir, "basic.formula.toml"))
	if err != nil {
		t.Fatalf("reading basic: %v", err)
	}
	if string(data) != "basic" {
		t.Errorf("basic content = %q", string(data))
	}

	// Check automation files.
	data, err = os.ReadFile(filepath.Join(dir, "automations", "foo", "automation.toml"))
	if err != nil {
		t.Fatalf("reading foo automation: %v", err)
	}
	if string(data) != "foo automation" {
		t.Errorf("foo automation content = %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(dir, "automations", "bar", "automation.toml"))
	if err != nil {
		t.Fatalf("reading bar automation: %v", err)
	}
	if string(data) != "bar automation" {
		t.Errorf("bar automation content = %q", string(data))
	}
}

// --- ListEmbeddedSystemFormulas ---

func TestListEmbeddedEmpty(t *testing.T) {
	fs := fstest.MapFS{}
	got := ListEmbeddedSystemFormulas(fs, ".")
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestListEmbeddedWithFiles(t *testing.T) {
	fs := fstest.MapFS{
		"sf/a.formula.toml":                &fstest.MapFile{Data: []byte("a")},
		"sf/b.formula.toml":                &fstest.MapFile{Data: []byte("b")},
		"sf/automations/p/automation.toml": &fstest.MapFile{Data: []byte("p")},
		"sf/readme.txt":                    &fstest.MapFile{Data: []byte("skip")},
	}

	got := ListEmbeddedSystemFormulas(fs, "sf")
	sort.Strings(got)
	want := []string{"a.formula.toml", "automations/p/automation.toml", "b.formula.toml"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
