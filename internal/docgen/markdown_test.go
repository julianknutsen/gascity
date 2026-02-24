package docgen

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderMarkdownCitySchema(t *testing.T) {
	s, err := GenerateCitySchema()
	if err != nil {
		t.Fatalf("GenerateCitySchema: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, s); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	md := buf.String()
	if md == "" {
		t.Fatal("empty markdown output")
	}

	// Check for expected section headers.
	for _, section := range []string{"## City", "## Agent", "## Workspace", "## PoolConfig"} {
		if !strings.Contains(md, section) {
			t.Errorf("missing section %q", section)
		}
	}

	// City should come first (before other sections).
	cityIdx := strings.Index(md, "## City")
	agentIdx := strings.Index(md, "## Agent")
	if cityIdx > agentIdx {
		t.Error("City section should come before Agent section")
	}
}

func TestRenderMarkdownTableFormat(t *testing.T) {
	s, err := GenerateCitySchema()
	if err != nil {
		t.Fatalf("GenerateCitySchema: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, s); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	md := buf.String()
	lines := strings.Split(md, "\n")

	// Find table rows (lines starting with |).
	for _, line := range lines {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		// Each table row should have 6 pipe characters (5 columns).
		pipes := strings.Count(line, "|")
		// Account for escaped pipes in descriptions.
		escaped := strings.Count(line, "\\|")
		actual := pipes - escaped
		if actual != 6 {
			t.Errorf("table row has %d columns (expected 5): %s", actual-1, line)
		}
	}
}

func TestRenderMarkdownRequiredFields(t *testing.T) {
	s, err := GenerateCitySchema()
	if err != nil {
		t.Fatalf("GenerateCitySchema: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, s); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	md := buf.String()

	// Agent.name should be marked required.
	if !strings.Contains(md, "| `name` | string | **yes**") {
		t.Error("Agent.name not marked as required in markdown")
	}
}

func TestRenderMarkdownEnumValues(t *testing.T) {
	s, err := GenerateCitySchema()
	if err != nil {
		t.Fatalf("GenerateCitySchema: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, s); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	md := buf.String()

	// Isolation field should show enum values.
	if !strings.Contains(md, "`none`") || !strings.Contains(md, "`worktree`") {
		t.Error("isolation enum values not shown in markdown")
	}
}

func TestRenderMarkdownFormulaSchema(t *testing.T) {
	s, err := GenerateFormulaSchema()
	if err != nil {
		t.Fatalf("GenerateFormulaSchema: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, s); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	md := buf.String()

	for _, section := range []string{"## Formula", "## Step"} {
		if !strings.Contains(md, section) {
			t.Errorf("missing section %q", section)
		}
	}
}
