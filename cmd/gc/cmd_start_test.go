package main

import (
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestReadPromptFileReadError(t *testing.T) {
	f := fsys.NewFake()
	f.Errors["/city/prompts/broken.md"] = errExit
	got := readPromptFile(f, "/city", "prompts/broken.md")
	if got != "" {
		t.Errorf("readPromptFile(error) = %q, want empty", got)
	}
}

func TestReadPromptFileLoadsMultilineContent(t *testing.T) {
	// Verify prompt loading preserves multi-line content like our actual prompts.
	f := fsys.NewFake()
	content := "# Loop Worker\n\nYou drain the backlog.\n\n1. Check your hook\n2. Claim work\n3. Do it\n4. Repeat\n"
	f.Files["/city/prompts/loop.md"] = []byte(content)
	got := readPromptFile(f, "/city", "prompts/loop.md")
	if got != content {
		t.Errorf("readPromptFile = %q, want %q", got, content)
	}
}

func TestMergeEnvEmptyMaps(t *testing.T) {
	got := mergeEnv(map[string]string{}, map[string]string{})
	if got != nil {
		t.Errorf("mergeEnv(empty, empty) = %v, want nil", got)
	}
}

func TestMergeEnvNilAndValues(t *testing.T) {
	got := mergeEnv(nil, map[string]string{"A": "1"})
	if got["A"] != "1" {
		t.Errorf("mergeEnv[A] = %q, want %q", got["A"], "1")
	}
}

func TestPassthroughEnvIncludesPath(t *testing.T) {
	// PATH is always set in a normal environment.
	got := passthroughEnv()
	if _, ok := got["PATH"]; !ok {
		t.Error("passthroughEnv() missing PATH")
	}
}

func TestPassthroughEnvPicksUpGCBeads(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	got := passthroughEnv()
	if got["GC_BEADS"] != "file" {
		t.Errorf("passthroughEnv()[GC_BEADS] = %q, want %q", got["GC_BEADS"], "file")
	}
}

func TestPassthroughEnvOmitsUnset(t *testing.T) {
	t.Setenv("GC_DOLT", "")
	got := passthroughEnv()
	if _, ok := got["GC_DOLT"]; ok {
		t.Error("passthroughEnv() should omit empty GC_DOLT")
	}
}

func TestMergeEnvOverrideOrder(t *testing.T) {
	a := map[string]string{"KEY": "first", "A": "a"}
	b := map[string]string{"KEY": "second", "B": "b"}
	got := mergeEnv(a, b)
	if got["KEY"] != "second" {
		t.Errorf("mergeEnv override: KEY = %q, want %q", got["KEY"], "second")
	}
	if got["A"] != "a" {
		t.Errorf("mergeEnv: A = %q, want %q", got["A"], "a")
	}
	if got["B"] != "b" {
		t.Errorf("mergeEnv: B = %q, want %q", got["B"], "b")
	}
}

func TestMergeEnvAllNil(t *testing.T) {
	got := mergeEnv(nil, nil, nil)
	if got != nil {
		t.Errorf("mergeEnv(nil, nil, nil) = %v, want nil", got)
	}
}
