// Package docsync verifies that tutorial prose and testscript txtar files
// cover the same set of gc commands. Every `$ gc <verb>` in a tutorial
// markdown must have a corresponding `exec gc <verb>` in the txtar.
package docsync

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/docgen"
)

func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// gcVerbsFromMarkdown extracts unique gc subcommands from code blocks.
// Only matches unindented `$ gc ...` lines to skip agent conversations.
func gcVerbsFromMarkdown(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	verbs := make(map[string]bool)
	inCodeBlock := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if !inCodeBlock {
			continue
		}
		if !strings.HasPrefix(line, "$ gc ") {
			continue
		}
		verb := extractVerb(line[len("$ gc "):])
		if verb != "" {
			verbs[verb] = true
		}
	}
	return verbs, scanner.Err()
}

// gcVerbsFromTxtar extracts unique gc subcommands from exec lines.
// Recognizes both active ("exec gc ...") and commented-out ("# exec gc ...")
// lines so that planned-but-unimplemented commands count as covered.
func gcVerbsFromTxtar(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	verbs := make(map[string]bool)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		after, ok := strings.CutPrefix(line, "exec gc ")
		if !ok {
			after, ok = strings.CutPrefix(line, "# exec gc ")
			if !ok {
				continue
			}
		}
		verb := extractVerb(after)
		if verb != "" {
			verbs[verb] = true
		}
	}
	return verbs, scanner.Err()
}

// extractVerb pulls the subcommand (up to 2 lowercase words) from args.
// "rig add ~/foo" → "rig add", "bead show gc-1" → "bead show",
// "start $WORK/x" → "start".
func extractVerb(args string) string {
	words := strings.Fields(args)
	var parts []string
	for i, w := range words {
		if i >= 2 {
			break
		}
		if !isLowerAlpha(w) {
			break
		}
		parts = append(parts, w)
	}
	return strings.Join(parts, " ")
}

func isLowerAlpha(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func TestTutorial01CommandSync(t *testing.T) {
	root := repoRoot()
	tutorial := filepath.Join(root, "docs", "tutorials", "01-hello-gas-city.md")
	txtar := filepath.Join(root, "cmd", "gc", "testdata", "01-hello-gas-city.txtar")

	mdVerbs, err := gcVerbsFromMarkdown(tutorial)
	if err != nil {
		t.Fatalf("parsing tutorial: %v", err)
	}

	txtarVerbs, err := gcVerbsFromTxtar(txtar)
	if err != nil {
		t.Fatalf("parsing txtar: %v", err)
	}

	// Every tutorial command must have txtar coverage.
	var missing []string
	for verb := range mdVerbs {
		if !txtarVerbs[verb] {
			missing = append(missing, verb)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("gc commands in tutorial but not in txtar:")
		for _, v := range missing {
			t.Errorf("  gc %s", v)
		}
	}

	// Every txtar command must have tutorial coverage.
	var extra []string
	for verb := range txtarVerbs {
		if !mdVerbs[verb] {
			extra = append(extra, verb)
		}
	}

	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf("gc commands in txtar but not in tutorial:")
		for _, v := range extra {
			t.Errorf("  gc %s", v)
		}
	}
}

func TestSchemaFreshness(t *testing.T) {
	root := repoRoot()

	// Generate schemas in memory and compare against committed files.
	tests := []struct {
		name     string
		generate func() ([]byte, error)
		path     string
	}{
		{
			name: "city-schema.json",
			generate: func() ([]byte, error) {
				s, err := docgen.GenerateCitySchema()
				if err != nil {
					return nil, err
				}
				data, err := json.MarshalIndent(s, "", "  ")
				if err != nil {
					return nil, err
				}
				return append(data, '\n'), nil
			},
			path: filepath.Join(root, "docs", "schema", "city-schema.json"),
		},
		{
			name: "formula-schema.json",
			generate: func() ([]byte, error) {
				s, err := docgen.GenerateFormulaSchema()
				if err != nil {
					return nil, err
				}
				data, err := json.MarshalIndent(s, "", "  ")
				if err != nil {
					return nil, err
				}
				return append(data, '\n'), nil
			},
			path: filepath.Join(root, "docs", "schema", "formula-schema.json"),
		},
		{
			name: "config.md",
			generate: func() ([]byte, error) {
				s, err := docgen.GenerateCitySchema()
				if err != nil {
					return nil, err
				}
				var buf bytes.Buffer
				if err := docgen.RenderMarkdown(&buf, s); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			},
			path: filepath.Join(root, "docs", "reference", "config.md"),
		},
		{
			name: "formula.md",
			generate: func() ([]byte, error) {
				s, err := docgen.GenerateFormulaSchema()
				if err != nil {
					return nil, err
				}
				var buf bytes.Buffer
				if err := docgen.RenderMarkdown(&buf, s); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			},
			path: filepath.Join(root, "docs", "reference", "formula.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generated, err := tt.generate()
			if err != nil {
				t.Fatalf("generating %s: %v", tt.name, err)
			}

			committed, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("reading %s: %v\nRun: go run ./cmd/genschema", tt.path, err)
			}

			if !bytes.Equal(generated, committed) {
				t.Errorf("%s is stale. Run: go run ./cmd/genschema", tt.name)
			}
		})
	}
}
