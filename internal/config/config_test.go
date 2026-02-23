package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestDefaultCity(t *testing.T) {
	c := DefaultCity("bright-lights")
	if c.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q", c.Workspace.Name, "bright-lights")
	}
	if len(c.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(c.Agents))
	}
	if c.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", c.Agents[0].Name, "mayor")
	}
	if c.Agents[0].PromptTemplate != "prompts/mayor.md" {
		t.Errorf("Agents[0].PromptTemplate = %q, want %q", c.Agents[0].PromptTemplate, "prompts/mayor.md")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	c := DefaultCity("bright-lights")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q", got.Workspace.Name, "bright-lights")
	}
	if len(got.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(got.Agents))
	}
	if got.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", got.Agents[0].Name, "mayor")
	}
}

func TestMarshalOmitsEmptyFields(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "provider") {
		t.Errorf("Marshal output should not contain 'provider' when empty:\n%s", s)
	}
	if strings.Contains(s, "start_command") {
		t.Errorf("Marshal output should not contain 'start_command' when empty:\n%s", s)
	}
	// prompt_template IS set on the default mayor, so check an agent without it.
	c2 := City{Workspace: Workspace{Name: "test"}, Agents: []Agent{{Name: "bare"}}}
	data2, err := c2.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data2), "prompt_template") {
		t.Errorf("Marshal output should not contain 'prompt_template' when empty:\n%s", data2)
	}
}

func TestMarshalDefaultCityFormat(t *testing.T) {
	c := DefaultCity("bright-lights")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := "[workspace]\nname = \"bright-lights\"\n\n[[agents]]\nname = \"mayor\"\nprompt_template = \"prompts/mayor.md\"\n"
	if string(data) != want {
		t.Errorf("Marshal output:\ngot:\n%s\nwant:\n%s", data, want)
	}
}

func TestParseWithAgentsAndStartCommand(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"

[[agents]]
name = "mayor"
start_command = "claude --dangerously-skip-permissions"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "bright-lights")
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Agents[0].StartCommand != "claude --dangerously-skip-permissions" {
		t.Errorf("Agents[0].StartCommand = %q, want %q", cfg.Agents[0].StartCommand, "claude --dangerously-skip-permissions")
	}
}

func TestParseAgentsNoStartCommand(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].StartCommand != "" {
		t.Errorf("Agents[0].StartCommand = %q, want empty", cfg.Agents[0].StartCommand)
	}
}

func TestParseNoAgents(t *testing.T) {
	data := []byte(`
[workspace]
name = "bare-city"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 0 {
		t.Errorf("len(Agents) = %d, want 0", len(cfg.Agents))
	}
}

func TestParseEmptyFile(t *testing.T) {
	data := []byte("# just a comment\n")
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.Name != "" {
		t.Errorf("Workspace.Name = %q, want empty", cfg.Workspace.Name)
	}
	if len(cfg.Agents) != 0 {
		t.Errorf("len(Agents) = %d, want 0", len(cfg.Agents))
	}
}

func TestParseCorruptTOML(t *testing.T) {
	data := []byte("[[[invalid toml")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for corrupt TOML")
	}
}

func TestLoadSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "city.toml")
	content := `[workspace]
name = "test"

[[agents]]
name = "mayor"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(fsys.OSFS{}, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Workspace.Name != "test" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "test")
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load(fsys.OSFS{}, "/nonexistent/city.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadReadError(t *testing.T) {
	f := fsys.NewFake()
	f.Errors["/city/city.toml"] = fmt.Errorf("permission denied")

	_, err := Load(f, "/city/city.toml")
	if err == nil {
		t.Fatal("expected error when ReadFile fails")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want 'permission denied'", err)
	}
}

func TestLoadWithFake(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/city.toml"] = []byte("[workspace]\nname = \"fake-city\"\n")

	cfg, err := Load(f, "/city/city.toml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Workspace.Name != "fake-city" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "fake-city")
	}
}

func TestLoadCorruptTOML(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/city.toml"] = []byte("[[[invalid toml")

	_, err := Load(f, "/city/city.toml")
	if err == nil {
		t.Fatal("expected error for corrupt TOML")
	}
}

func TestParseWithProvider(t *testing.T) {
	data := []byte(`
[workspace]
name = "multi-provider"

[[agents]]
name = "mayor"
provider = "claude"

[[agents]]
name = "worker"
provider = "codex"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Provider != "claude" {
		t.Errorf("Agents[0].Provider = %q, want %q", cfg.Agents[0].Provider, "claude")
	}
	if cfg.Agents[1].Provider != "codex" {
		t.Errorf("Agents[1].Provider = %q, want %q", cfg.Agents[1].Provider, "codex")
	}
}

func TestParseBeadsSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[beads]
provider = "file"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Beads.Provider != "file" {
		t.Errorf("Beads.Provider = %q, want %q", cfg.Beads.Provider, "file")
	}
}

func TestParseNoBeadsSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Beads.Provider != "" {
		t.Errorf("Beads.Provider = %q, want empty", cfg.Beads.Provider)
	}
}

func TestMarshalOmitsEmptyBeadsSection(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[beads]") {
		t.Errorf("Marshal output should not contain '[beads]' when empty:\n%s", data)
	}
}

func TestParseWithPromptTemplate(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"

[[agents]]
name = "worker"
prompt_template = "prompts/worker.md"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].PromptTemplate != "prompts/mayor.md" {
		t.Errorf("Agents[0].PromptTemplate = %q, want %q", cfg.Agents[0].PromptTemplate, "prompts/mayor.md")
	}
	if cfg.Agents[1].PromptTemplate != "prompts/worker.md" {
		t.Errorf("Agents[1].PromptTemplate = %q, want %q", cfg.Agents[1].PromptTemplate, "prompts/worker.md")
	}
}

func TestMarshalOmitsEmptyPromptTemplate(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "prompt_template") {
		t.Errorf("Marshal output should not contain 'prompt_template' when empty:\n%s", data)
	}
}

func TestParseMultipleAgents(t *testing.T) {
	data := []byte(`
[workspace]
name = "big-city"

[[agents]]
name = "mayor"

[[agents]]
name = "worker"
start_command = "codex --dangerously-bypass-approvals-and-sandbox"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", cfg.Agents[0].Name, "mayor")
	}
	if cfg.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg.Agents[1].Name, "worker")
	}
	if cfg.Agents[1].StartCommand != "codex --dangerously-bypass-approvals-and-sandbox" {
		t.Errorf("Agents[1].StartCommand = %q, want codex command", cfg.Agents[1].StartCommand)
	}
}
