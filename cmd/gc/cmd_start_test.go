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
