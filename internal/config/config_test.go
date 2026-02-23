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

func TestParseWorkspaceProvider(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"
provider = "claude"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", cfg.Workspace.Provider, "claude")
	}
}

func TestParseWorkspaceStartCommand(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"
start_command = "my-agent --flag"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.StartCommand != "my-agent --flag" {
		t.Errorf("Workspace.StartCommand = %q, want %q", cfg.Workspace.StartCommand, "my-agent --flag")
	}
}

func TestWizardCity(t *testing.T) {
	c := WizardCity("bright-lights", "claude", "")
	if c.Workspace.Name != "bright-lights" {
		t.Errorf("Workspace.Name = %q, want %q", c.Workspace.Name, "bright-lights")
	}
	if c.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", c.Workspace.Provider, "claude")
	}
	if len(c.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(c.Agents))
	}
	if c.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", c.Agents[0].Name, "mayor")
	}
	if c.Agents[0].PromptTemplate != "prompts/mayor.md" {
		t.Errorf("Agents[0].PromptTemplate = %q, want %q", c.Agents[0].PromptTemplate, "prompts/mayor.md")
	}
	if c.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", c.Agents[1].Name, "worker")
	}
	if c.Agents[1].PromptTemplate != "prompts/worker.md" {
		t.Errorf("Agents[1].PromptTemplate = %q, want %q", c.Agents[1].PromptTemplate, "prompts/worker.md")
	}
}

func TestWizardCityMarshal(t *testing.T) {
	c := WizardCity("bright-lights", "claude", "")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `provider = "claude"`) {
		t.Errorf("Marshal output missing provider:\n%s", s)
	}
	if !strings.Contains(s, `name = "mayor"`) {
		t.Errorf("Marshal output missing mayor agent:\n%s", s)
	}
	if !strings.Contains(s, `name = "worker"`) {
		t.Errorf("Marshal output missing worker agent:\n%s", s)
	}

	// Round-trip parse.
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", got.Workspace.Provider, "claude")
	}
	if len(got.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(got.Agents))
	}
}

func TestWizardCityEmptyProvider(t *testing.T) {
	c := WizardCity("test", "", "")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	// provider should be omitted when empty.
	idx := strings.Index(s, "[[agents]]")
	if idx == -1 {
		t.Fatal("marshal output missing [[agents]] section")
	}
	wsSection := s[:idx]
	if strings.Contains(wsSection, "provider") {
		t.Errorf("workspace section should not contain 'provider' when empty:\n%s", wsSection)
	}
}

func TestWizardCityStartCommand(t *testing.T) {
	c := WizardCity("bright-lights", "", "my-agent --auto")
	if c.Workspace.StartCommand != "my-agent --auto" {
		t.Errorf("Workspace.StartCommand = %q, want %q", c.Workspace.StartCommand, "my-agent --auto")
	}
	if c.Workspace.Provider != "" {
		t.Errorf("Workspace.Provider = %q, want empty (startCommand takes precedence)", c.Workspace.Provider)
	}
	if len(c.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(c.Agents))
	}

	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `start_command = "my-agent --auto"`) {
		t.Errorf("Marshal output missing start_command:\n%s", s)
	}
	// provider should NOT appear.
	idx := strings.Index(s, "[[agents]]")
	if idx == -1 {
		t.Fatal("marshal output missing [[agents]] section")
	}
	wsSection := s[:idx]
	if strings.Contains(wsSection, "provider") {
		t.Errorf("workspace section should not contain 'provider' when startCommand set:\n%s", wsSection)
	}
}

func TestMarshalOmitsEmptyWorkspaceFields(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	// Workspace provider and start_command should not appear when empty.
	// Check the workspace section specifically (before [[agents]]).
	idx := strings.Index(s, "[[agents]]")
	if idx == -1 {
		t.Fatal("marshal output missing [[agents]] section")
	}
	wsSection := s[:idx]
	if strings.Contains(wsSection, "provider") {
		t.Errorf("workspace section should not contain 'provider' when empty:\n%s", wsSection)
	}
	if strings.Contains(wsSection, "start_command") {
		t.Errorf("workspace section should not contain 'start_command' when empty:\n%s", wsSection)
	}
}

func TestParseProvidersSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"
provider = "claude"

[providers.kiro]
command = "kiro"
args = ["--autonomous"]
prompt_mode = "arg"
ready_delay_ms = 5000
process_names = ["kiro", "node"]

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("len(Providers) = %d, want 1", len(cfg.Providers))
	}
	kiro, ok := cfg.Providers["kiro"]
	if !ok {
		t.Fatal("Providers[kiro] not found")
	}
	if kiro.Command != "kiro" {
		t.Errorf("Command = %q, want %q", kiro.Command, "kiro")
	}
	if len(kiro.Args) != 1 || kiro.Args[0] != "--autonomous" {
		t.Errorf("Args = %v, want [--autonomous]", kiro.Args)
	}
	if kiro.PromptMode != "arg" {
		t.Errorf("PromptMode = %q, want %q", kiro.PromptMode, "arg")
	}
	if kiro.ReadyDelayMs != 5000 {
		t.Errorf("ReadyDelayMs = %d, want 5000", kiro.ReadyDelayMs)
	}
}

func TestParseAgentOverrideFields(t *testing.T) {
	data := []byte(`
[workspace]
name = "bright-lights"

[[agents]]
name = "scout"
provider = "claude"
args = ["--dangerously-skip-permissions", "--verbose"]
ready_delay_ms = 15000
prompt_mode = "flag"
prompt_flag = "--prompt"
process_names = ["node"]
emits_permission_warning = false
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	a := cfg.Agents[0]
	if a.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", a.Provider, "claude")
	}
	if len(a.Args) != 2 {
		t.Fatalf("len(Args) = %d, want 2", len(a.Args))
	}
	if a.Args[1] != "--verbose" {
		t.Errorf("Args[1] = %q, want %q", a.Args[1], "--verbose")
	}
	if a.ReadyDelayMs == nil || *a.ReadyDelayMs != 15000 {
		t.Errorf("ReadyDelayMs = %v, want 15000", a.ReadyDelayMs)
	}
	if a.PromptMode != "flag" {
		t.Errorf("PromptMode = %q, want %q", a.PromptMode, "flag")
	}
	if a.PromptFlag != "--prompt" {
		t.Errorf("PromptFlag = %q, want %q", a.PromptFlag, "--prompt")
	}
	if a.EmitsPermissionWarning == nil || *a.EmitsPermissionWarning != false {
		t.Errorf("EmitsPermissionWarning = %v, want false", a.EmitsPermissionWarning)
	}
}

func TestMarshalOmitsEmptyProviders(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[providers") {
		t.Errorf("Marshal output should not contain '[providers' when empty:\n%s", data)
	}
}

func TestMarshalOmitsEmptyAgentOverrideFields(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, field := range []string{"args", "prompt_mode", "prompt_flag", "ready_delay_ms", "ready_prompt_prefix", "process_names", "emits_permission_warning", "env"} {
		if strings.Contains(s, field) {
			t.Errorf("Marshal output should not contain %q when empty:\n%s", field, s)
		}
	}
}

func TestProvidersRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Providers: map[string]ProviderSpec{
			"kiro": {
				Command:    "kiro",
				Args:       []string{"--autonomous"},
				PromptMode: "arg",
			},
		},
		Agents: []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if len(got.Providers) != 1 {
		t.Fatalf("len(Providers) = %d, want 1", len(got.Providers))
	}
	kiro, ok := got.Providers["kiro"]
	if !ok {
		t.Fatal("Providers[kiro] not found after round-trip")
	}
	if kiro.Command != "kiro" {
		t.Errorf("Command = %q, want %q", kiro.Command, "kiro")
	}
	if len(kiro.Args) != 1 || kiro.Args[0] != "--autonomous" {
		t.Errorf("Args = %v, want [--autonomous]", kiro.Args)
	}
	if kiro.PromptMode != "arg" {
		t.Errorf("PromptMode = %q, want %q", kiro.PromptMode, "arg")
	}
}

func TestParseSuspendedAgent(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "worker"
suspended = true
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Suspended == nil {
		t.Fatal("Suspended is nil, want *true")
	}
	if !*cfg.Agents[0].Suspended {
		t.Error("Suspended = false, want true")
	}
}

func TestMarshalOmitsSuspendedNil(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "suspended") {
		t.Errorf("Marshal output should not contain 'suspended' when nil:\n%s", data)
	}
}

func TestMarshalIncludesSuspendedTrue(t *testing.T) {
	tr := true
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker", Suspended: &tr}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), "suspended = true") {
		t.Errorf("Marshal output should contain 'suspended = true':\n%s", data)
	}
}

func TestIsSuspendedTrue(t *testing.T) {
	tr := true
	a := Agent{Name: "worker", Suspended: &tr}
	if !a.IsSuspended() {
		t.Error("IsSuspended() = false, want true")
	}
}

func TestIsSuspendedFalse(t *testing.T) {
	f := false
	a := Agent{Name: "worker", Suspended: &f}
	if a.IsSuspended() {
		t.Error("IsSuspended() = true, want false")
	}
}

func TestIsSuspendedNil(t *testing.T) {
	a := Agent{Name: "worker"}
	if a.IsSuspended() {
		t.Error("IsSuspended() = true, want false (nil)")
	}
}

func TestParseAgentEnv(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "worker"

[agents.env]
EXTRA = "yes"
DEBUG = "1"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	if cfg.Agents[0].Env["EXTRA"] != "yes" {
		t.Errorf("Env[EXTRA] = %q, want %q", cfg.Agents[0].Env["EXTRA"], "yes")
	}
	if cfg.Agents[0].Env["DEBUG"] != "1" {
		t.Errorf("Env[DEBUG] = %q, want %q", cfg.Agents[0].Env["DEBUG"], "1")
	}
}
