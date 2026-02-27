package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/docgen"
)

func TestGenDocProducesMarkdown(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd(&buf, &buf)

	// Render to buffer using the renderer directly (avoids needing repo root
	// for the go.mod check in the RunE handler).
	var md bytes.Buffer
	if err := docgen.RenderCLIMarkdown(&md, root); err != nil {
		t.Fatalf("RenderCLIMarkdown: %v", err)
	}

	out := md.String()
	if out == "" {
		t.Fatal("empty markdown output")
	}

	// Check known visible commands exist.
	for _, cmd := range []string{"gc init", "gc start", "gc stop", "gc agent", "gc rig add", "gc mail"} {
		if !strings.Contains(out, "## "+cmd) {
			t.Errorf("missing command %q in CLI reference", cmd)
		}
	}

	// Check hidden commands are absent.
	if strings.Contains(out, "## gc gen-doc") {
		t.Error("hidden command gen-doc should not appear")
	}

	// Check basic structure.
	if !strings.Contains(out, "# CLI Reference") {
		t.Error("missing CLI Reference header")
	}
	if !strings.Contains(out, "Auto-generated") {
		t.Error("missing auto-generated note")
	}
}
