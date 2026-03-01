package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestParseSessionSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[session]
provider = "subprocess"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Session.Provider != "subprocess" {
		t.Errorf("Session.Provider = %q, want %q", cfg.Session.Provider, "subprocess")
	}
}

func TestParseNoSessionSection(t *testing.T) {
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
	if cfg.Session.Provider != "" {
		t.Errorf("Session.Provider = %q, want empty", cfg.Session.Provider)
	}
}

func TestMarshalOmitsEmptySessionSection(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[session]") {
		t.Errorf("Marshal output should not contain '[session]' when empty:\n%s", data)
	}
}

func TestParseMailSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[mail]
provider = "exec:/usr/local/bin/mail-bridge"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Mail.Provider != "exec:/usr/local/bin/mail-bridge" {
		t.Errorf("Mail.Provider = %q, want %q", cfg.Mail.Provider, "exec:/usr/local/bin/mail-bridge")
	}
}

func TestParseNoMailSection(t *testing.T) {
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
	if cfg.Mail.Provider != "" {
		t.Errorf("Mail.Provider = %q, want empty", cfg.Mail.Provider)
	}
}

func TestMarshalOmitsEmptyMailSection(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[mail]") {
		t.Errorf("Marshal output should not contain '[mail]' when empty:\n%s", data)
	}
}

func TestParseEventsSection(t *testing.T) {
	data := []byte(`
[workspace]
name = "test-city"

[events]
provider = "fake"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Events.Provider != "fake" {
		t.Errorf("Events.Provider = %q, want %q", cfg.Events.Provider, "fake")
	}
}

func TestParseNoEventsSection(t *testing.T) {
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
	if cfg.Events.Provider != "" {
		t.Errorf("Events.Provider = %q, want empty", cfg.Events.Provider)
	}
}

func TestMarshalOmitsEmptyEventsSection(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[events]") {
		t.Errorf("Marshal output should not contain '[events]' when empty:\n%s", data)
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
	// Round-trip parse.
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Workspace.Provider != "claude" {
		t.Errorf("Workspace.Provider = %q, want %q", got.Workspace.Provider, "claude")
	}
	if len(got.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(got.Agents))
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
	if len(c.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(c.Agents))
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

func TestParseAgentDir(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "worker"
dir = "projects/frontend"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Dir != "projects/frontend" {
		t.Errorf("Agents[0].Dir = %q, want %q", cfg.Agents[0].Dir, "projects/frontend")
	}
	if cfg.Agents[1].Dir != "" {
		t.Errorf("Agents[1].Dir = %q, want empty", cfg.Agents[1].Dir)
	}
}

func TestParseAgentPreStart(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "worker"
dir = "/repo"
pre_start = ["mkdir -p /tmp/work", "git worktree add /tmp/work"]

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if len(cfg.Agents[0].PreStart) != 2 {
		t.Errorf("Agents[0].PreStart len = %d, want 2", len(cfg.Agents[0].PreStart))
	}
	if len(cfg.Agents[1].PreStart) != 0 {
		t.Errorf("Agents[1].PreStart len = %d, want 0", len(cfg.Agents[1].PreStart))
	}
}

func TestPreStartRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker", Dir: "/repo", PreStart: []string{"echo hello"}}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if len(got.Agents[0].PreStart) != 1 || got.Agents[0].PreStart[0] != "echo hello" {
		t.Errorf("PreStart after round-trip = %v, want [echo hello]", got.Agents[0].PreStart)
	}
}

func TestMarshalOmitsEmptyPreStart(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "pre_start") {
		t.Errorf("Marshal output should not contain 'pre_start' when empty:\n%s", data)
	}
}

func TestMarshalOmitsEmptyDir(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "dir") {
		t.Errorf("Marshal output should not contain 'dir' when empty:\n%s", data)
	}
}

func TestDirRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "worker", Dir: "projects/backend"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Agents[0].Dir != "projects/backend" {
		t.Errorf("Dir after round-trip = %q, want %q", got.Agents[0].Dir, "projects/backend")
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

// --- Pool-in-agent tests ---

func TestParseAgentWithPool(t *testing.T) {
	data := []byte(`
[workspace]
name = "pool-city"

[[agents]]
name = "worker"
prompt_template = "prompts/pool-worker.md"
start_command = "echo hello"

[agents.pool]
min = 0
max = 5
check = "echo 3"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	a := cfg.Agents[0]
	if a.Pool == nil {
		t.Fatal("Pool is nil, want non-nil")
	}
	if a.Pool.Min != 0 {
		t.Errorf("Pool.Min = %d, want 0", a.Pool.Min)
	}
	if a.Pool.Max != 5 {
		t.Errorf("Pool.Max = %d, want 5", a.Pool.Max)
	}
	if a.Pool.Check != "echo 3" {
		t.Errorf("Pool.Check = %q, want %q", a.Pool.Check, "echo 3")
	}
}

func TestParseAgentWithoutPool(t *testing.T) {
	data := []byte(`
[workspace]
name = "simple"

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
	if cfg.Agents[0].Pool != nil {
		t.Errorf("Pool = %+v, want nil", cfg.Agents[0].Pool)
	}
}

func TestPoolRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents: []Agent{{
			Name: "worker",
			Pool: &PoolConfig{Min: 1, Max: 5, Check: "echo 3"},
		}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(got.Agents))
	}
	a := got.Agents[0]
	if a.Pool == nil {
		t.Fatal("Pool is nil after round-trip")
	}
	if a.Pool.Min != 1 {
		t.Errorf("Pool.Min = %d, want 1", a.Pool.Min)
	}
	if a.Pool.Max != 5 {
		t.Errorf("Pool.Max = %d, want 5", a.Pool.Max)
	}
	if a.Pool.Check != "echo 3" {
		t.Errorf("Pool.Check = %q, want %q", a.Pool.Check, "echo 3")
	}
}

func TestEffectiveWorkQueryDefault(t *testing.T) {
	a := Agent{Name: "mayor"}
	got := a.EffectiveWorkQuery()
	want := "bd ready --assignee=mayor"
	if got != want {
		t.Errorf("EffectiveWorkQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveWorkQueryCustom(t *testing.T) {
	a := Agent{Name: "mayor", WorkQuery: "bd ready --label=pool:polecats"}
	got := a.EffectiveWorkQuery()
	want := "bd ready --label=pool:polecats"
	if got != want {
		t.Errorf("EffectiveWorkQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveWorkQueryWithDir(t *testing.T) {
	a := Agent{Name: "polecat", Dir: "hello-world"}
	got := a.EffectiveWorkQuery()
	want := "bd ready --assignee=hello-world/polecat"
	if got != want {
		t.Errorf("EffectiveWorkQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveWorkQueryPoolDefault(t *testing.T) {
	a := Agent{Name: "polecat", Dir: "hello-world", Pool: &PoolConfig{Min: 1, Max: 3}}
	got := a.EffectiveWorkQuery()
	want := "bd ready --label=pool:hello-world/polecat --limit=1"
	if got != want {
		t.Errorf("EffectiveWorkQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveSlingQueryFixedAgent(t *testing.T) {
	a := Agent{Name: "mayor"}
	got := a.EffectiveSlingQuery()
	want := "bd update {} --assignee=mayor"
	if got != want {
		t.Errorf("EffectiveSlingQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveSlingQueryFixedAgentWithDir(t *testing.T) {
	a := Agent{Name: "refinery", Dir: "hello-world"}
	got := a.EffectiveSlingQuery()
	want := "bd update {} --assignee=hello-world/refinery"
	if got != want {
		t.Errorf("EffectiveSlingQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveSlingQueryPoolDefault(t *testing.T) {
	a := Agent{Name: "polecat", Dir: "hello-world", Pool: &PoolConfig{Min: 1, Max: 3}}
	got := a.EffectiveSlingQuery()
	want := "bd update {} --label=pool:hello-world/polecat"
	if got != want {
		t.Errorf("EffectiveSlingQuery() = %q, want %q", got, want)
	}
}

func TestEffectiveSlingQueryCustom(t *testing.T) {
	a := Agent{Name: "worker", SlingQuery: "custom-dispatch {} --target=worker"}
	got := a.EffectiveSlingQuery()
	want := "custom-dispatch {} --target=worker"
	if got != want {
		t.Errorf("EffectiveSlingQuery() = %q, want %q", got, want)
	}
}

func TestValidateAgentsPoolMatchedPair(t *testing.T) {
	// Both set: OK
	agents := []Agent{{
		Name:       "polecat",
		Dir:        "rig",
		Pool:       &PoolConfig{Min: 1, Max: 3},
		WorkQuery:  "custom-query",
		SlingQuery: "custom-sling {}",
	}}
	if err := ValidateAgents(agents); err != nil {
		t.Errorf("both set: unexpected error: %v", err)
	}

	// Neither set: OK
	agents = []Agent{{
		Name: "polecat",
		Dir:  "rig",
		Pool: &PoolConfig{Min: 1, Max: 3},
	}}
	if err := ValidateAgents(agents); err != nil {
		t.Errorf("neither set: unexpected error: %v", err)
	}

	// Only sling_query set: error
	agents = []Agent{{
		Name:       "polecat",
		Dir:        "rig",
		Pool:       &PoolConfig{Min: 1, Max: 3},
		SlingQuery: "custom-sling {}",
	}}
	if err := ValidateAgents(agents); err == nil {
		t.Error("only sling_query set: expected error")
	}

	// Only work_query set: error
	agents = []Agent{{
		Name:      "polecat",
		Dir:       "rig",
		Pool:      &PoolConfig{Min: 1, Max: 3},
		WorkQuery: "custom-query",
	}}
	if err := ValidateAgents(agents); err == nil {
		t.Error("only work_query set: expected error")
	}
}

func TestValidateAgentsFixedAgentUnpairedOK(t *testing.T) {
	// Fixed agents don't require matched pairs.
	agents := []Agent{{
		Name:       "mayor",
		SlingQuery: "custom-sling {}",
	}}
	if err := ValidateAgents(agents); err != nil {
		t.Errorf("fixed agent with only sling_query: unexpected error: %v", err)
	}
}

func TestEffectivePoolNil(t *testing.T) {
	a := Agent{Name: "mayor"}
	p := a.EffectivePool()
	if p.Min != 1 {
		t.Errorf("Min = %d, want 1", p.Min)
	}
	if p.Max != 1 {
		t.Errorf("Max = %d, want 1", p.Max)
	}
	if p.Check != "echo 1" {
		t.Errorf("Check = %q, want %q", p.Check, "echo 1")
	}
}

func TestEffectivePoolExplicit(t *testing.T) {
	a := Agent{
		Name: "worker",
		Pool: &PoolConfig{Min: 2, Max: 10, Check: "echo 5"},
	}
	p := a.EffectivePool()
	if p.Min != 2 {
		t.Errorf("Min = %d, want 2", p.Min)
	}
	if p.Max != 10 {
		t.Errorf("Max = %d, want 10", p.Max)
	}
	if p.Check != "echo 5" {
		t.Errorf("Check = %q, want %q", p.Check, "echo 5")
	}
}

func TestEffectivePoolDefaults(t *testing.T) {
	// Pool present but check empty → default uses qualified name.
	a := Agent{
		Name: "refinery",
		Pool: &PoolConfig{Min: 0, Max: 1},
	}
	p := a.EffectivePool()
	if p.Min != 0 {
		t.Errorf("Min = %d, want 0", p.Min)
	}
	if p.Max != 1 {
		t.Errorf("Max = %d, want 1", p.Max)
	}
	want := "bd list --label=pool:refinery --status=open --json 2>/dev/null | jq length 2>/dev/null || echo 0"
	if p.Check != want {
		t.Errorf("Check = %q, want %q (default)", p.Check, want)
	}
}

func TestEffectivePoolDefaultsQualified(t *testing.T) {
	// Rig-scoped pool: default check uses qualified name (dir/name).
	a := Agent{
		Name: "polecat",
		Dir:  "myproject",
		Pool: &PoolConfig{Min: 0, Max: 5},
	}
	p := a.EffectivePool()
	want := "bd list --label=pool:myproject/polecat --status=open --json 2>/dev/null | jq length 2>/dev/null || echo 0"
	if p.Check != want {
		t.Errorf("Check = %q, want %q", p.Check, want)
	}
}

func TestIsPool(t *testing.T) {
	a := Agent{Name: "worker", Pool: &PoolConfig{Min: 0, Max: 5}}
	if !a.IsPool() {
		t.Error("IsPool() = false, want true")
	}

	b := Agent{Name: "mayor"}
	if b.IsPool() {
		t.Error("IsPool() = true, want false")
	}
}

func TestMarshalOmitsNilPool(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "pool") {
		t.Errorf("Marshal output should not contain 'pool' when nil:\n%s", data)
	}
}

func TestMixedAgentsWithAndWithoutPool(t *testing.T) {
	data := []byte(`
[workspace]
name = "mixed"

[[agents]]
name = "mayor"

[[agents]]
name = "worker"
start_command = "echo hello"

[agents.pool]
min = 0
max = 5
check = "echo 2"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Pool != nil {
		t.Errorf("mayor.Pool = %+v, want nil", cfg.Agents[0].Pool)
	}
	if cfg.Agents[1].Pool == nil {
		t.Fatal("worker.Pool is nil, want non-nil")
	}
	if cfg.Agents[1].Pool.Max != 5 {
		t.Errorf("worker.Pool.Max = %d, want 5", cfg.Agents[1].Pool.Max)
	}
}

func TestValidateAgentsDupName(t *testing.T) {
	agents := []Agent{
		{Name: "worker"},
		{Name: "worker"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate'", err)
	}
}

func TestValidatePoolMinGtMax(t *testing.T) {
	agents := []Agent{{
		Name: "worker",
		Pool: &PoolConfig{Min: 10, Max: 5},
	}}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for min > max")
	}
	if !strings.Contains(err.Error(), "min") && !strings.Contains(err.Error(), "max") {
		t.Errorf("error = %q, want mention of min/max", err)
	}
}

func TestValidatePoolMaxZero(t *testing.T) {
	// Max=0 is valid (disabled agent).
	agents := []Agent{{
		Name: "worker",
		Pool: &PoolConfig{Min: 0, Max: 0},
	}}
	err := ValidateAgents(agents)
	if err != nil {
		t.Errorf("ValidateAgents: unexpected error: %v", err)
	}
}

func TestValidateAgentsValid(t *testing.T) {
	agents := []Agent{
		{Name: "mayor"},
		{Name: "worker", Pool: &PoolConfig{Min: 0, Max: 10, Check: "echo 3"}},
	}
	if err := ValidateAgents(agents); err != nil {
		t.Errorf("ValidateAgents: unexpected error: %v", err)
	}
}

func TestValidateAgentsMissingName(t *testing.T) {
	agents := []Agent{{Pool: &PoolConfig{Min: 0, Max: 5}}}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want 'name is required'", err)
	}
}

// --- DaemonConfig tests ---

func TestDaemonPatrolIntervalDefault(t *testing.T) {
	d := DaemonConfig{}
	got := d.PatrolIntervalDuration()
	if got != 30*time.Second {
		t.Errorf("PatrolIntervalDuration() = %v, want 30s", got)
	}
}

func TestDaemonPatrolIntervalCustom(t *testing.T) {
	d := DaemonConfig{PatrolInterval: "10s"}
	got := d.PatrolIntervalDuration()
	if got != 10*time.Second {
		t.Errorf("PatrolIntervalDuration() = %v, want 10s", got)
	}
}

func TestDaemonPatrolIntervalInvalid(t *testing.T) {
	d := DaemonConfig{PatrolInterval: "not-a-duration"}
	got := d.PatrolIntervalDuration()
	if got != 30*time.Second {
		t.Errorf("PatrolIntervalDuration() = %v, want 30s (default for invalid)", got)
	}
}

func TestParseDaemonConfig(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[daemon]
patrol_interval = "15s"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Daemon.PatrolInterval != "15s" {
		t.Errorf("Daemon.PatrolInterval = %q, want %q", cfg.Daemon.PatrolInterval, "15s")
	}
	got := cfg.Daemon.PatrolIntervalDuration()
	if got != 15*time.Second {
		t.Errorf("PatrolIntervalDuration() = %v, want 15s", got)
	}
}

func TestParseDaemonConfigMissing(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Daemon.PatrolInterval != "" {
		t.Errorf("Daemon.PatrolInterval = %q, want empty", cfg.Daemon.PatrolInterval)
	}
	// Should still default to 30s.
	got := cfg.Daemon.PatrolIntervalDuration()
	if got != 30*time.Second {
		t.Errorf("PatrolIntervalDuration() = %v, want 30s", got)
	}
}

func TestDaemonMaxRestartsDefault(t *testing.T) {
	d := DaemonConfig{}
	got := d.MaxRestartsOrDefault()
	if got != 5 {
		t.Errorf("MaxRestartsOrDefault() = %d, want 5", got)
	}
}

func TestDaemonMaxRestartsExplicit(t *testing.T) {
	v := 3
	d := DaemonConfig{MaxRestarts: &v}
	got := d.MaxRestartsOrDefault()
	if got != 3 {
		t.Errorf("MaxRestartsOrDefault() = %d, want 3", got)
	}
}

func TestDaemonMaxRestartsZero(t *testing.T) {
	v := 0
	d := DaemonConfig{MaxRestarts: &v}
	got := d.MaxRestartsOrDefault()
	if got != 0 {
		t.Errorf("MaxRestartsOrDefault() = %d, want 0 (unlimited)", got)
	}
}

func TestDaemonRestartWindowDefault(t *testing.T) {
	d := DaemonConfig{}
	got := d.RestartWindowDuration()
	if got != time.Hour {
		t.Errorf("RestartWindowDuration() = %v, want 1h", got)
	}
}

func TestDaemonRestartWindowCustom(t *testing.T) {
	d := DaemonConfig{RestartWindow: "30m"}
	got := d.RestartWindowDuration()
	if got != 30*time.Minute {
		t.Errorf("RestartWindowDuration() = %v, want 30m", got)
	}
}

func TestDaemonRestartWindowInvalid(t *testing.T) {
	d := DaemonConfig{RestartWindow: "not-a-duration"}
	got := d.RestartWindowDuration()
	if got != time.Hour {
		t.Errorf("RestartWindowDuration() = %v, want 1h (default for invalid)", got)
	}
}

func TestParseDaemonCrashLoopConfig(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[daemon]
patrol_interval = "15s"
max_restarts = 3
restart_window = "30m"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Daemon.MaxRestarts == nil || *cfg.Daemon.MaxRestarts != 3 {
		t.Errorf("Daemon.MaxRestarts = %v, want 3", cfg.Daemon.MaxRestarts)
	}
	if cfg.Daemon.RestartWindow != "30m" {
		t.Errorf("Daemon.RestartWindow = %q, want %q", cfg.Daemon.RestartWindow, "30m")
	}
	if got := cfg.Daemon.MaxRestartsOrDefault(); got != 3 {
		t.Errorf("MaxRestartsOrDefault() = %d, want 3", got)
	}
	if got := cfg.Daemon.RestartWindowDuration(); got != 30*time.Minute {
		t.Errorf("RestartWindowDuration() = %v, want 30m", got)
	}
}

func TestParseDaemonMaxRestartsZero(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[daemon]
max_restarts = 0

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Daemon.MaxRestarts == nil {
		t.Fatal("Daemon.MaxRestarts is nil, want 0")
	}
	if *cfg.Daemon.MaxRestarts != 0 {
		t.Errorf("Daemon.MaxRestarts = %d, want 0", *cfg.Daemon.MaxRestarts)
	}
	if got := cfg.Daemon.MaxRestartsOrDefault(); got != 0 {
		t.Errorf("MaxRestartsOrDefault() = %d, want 0 (unlimited)", got)
	}
}

func TestMarshalOmitsEmptyDaemonSection(t *testing.T) {
	c := DefaultCity("test")
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "[daemon]") {
		t.Errorf("Marshal output should not contain '[daemon]' when empty:\n%s", data)
	}
}

// --- ShutdownTimeout tests ---

func TestDaemonShutdownTimeoutDefault(t *testing.T) {
	d := DaemonConfig{}
	got := d.ShutdownTimeoutDuration()
	if got != 5*time.Second {
		t.Errorf("ShutdownTimeoutDuration() = %v, want 5s", got)
	}
}

func TestDaemonShutdownTimeoutCustom(t *testing.T) {
	d := DaemonConfig{ShutdownTimeout: "3s"}
	got := d.ShutdownTimeoutDuration()
	if got != 3*time.Second {
		t.Errorf("ShutdownTimeoutDuration() = %v, want 3s", got)
	}
}

func TestDaemonShutdownTimeoutZero(t *testing.T) {
	d := DaemonConfig{ShutdownTimeout: "0s"}
	got := d.ShutdownTimeoutDuration()
	if got != 0 {
		t.Errorf("ShutdownTimeoutDuration() = %v, want 0", got)
	}
}

func TestDaemonShutdownTimeoutInvalid(t *testing.T) {
	d := DaemonConfig{ShutdownTimeout: "not-a-duration"}
	got := d.ShutdownTimeoutDuration()
	if got != 5*time.Second {
		t.Errorf("ShutdownTimeoutDuration() = %v, want 5s (default for invalid)", got)
	}
}

func TestParseShutdownTimeout(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[daemon]
patrol_interval = "15s"
shutdown_timeout = "3s"

[[agents]]
name = "mayor"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Daemon.ShutdownTimeout != "3s" {
		t.Errorf("Daemon.ShutdownTimeout = %q, want %q", cfg.Daemon.ShutdownTimeout, "3s")
	}
	got := cfg.Daemon.ShutdownTimeoutDuration()
	if got != 3*time.Second {
		t.Errorf("ShutdownTimeoutDuration() = %v, want 3s", got)
	}
}

// --- DrainTimeout tests ---

func TestDrainTimeoutDefault(t *testing.T) {
	p := PoolConfig{}
	got := p.DrainTimeoutDuration()
	if got != 5*time.Minute {
		t.Errorf("DrainTimeoutDuration() = %v, want 5m", got)
	}
}

func TestDrainTimeoutCustom(t *testing.T) {
	p := PoolConfig{DrainTimeout: "30s"}
	got := p.DrainTimeoutDuration()
	if got != 30*time.Second {
		t.Errorf("DrainTimeoutDuration() = %v, want 30s", got)
	}
}

func TestDrainTimeoutInvalid(t *testing.T) {
	p := PoolConfig{DrainTimeout: "not-a-duration"}
	got := p.DrainTimeoutDuration()
	if got != 5*time.Minute {
		t.Errorf("DrainTimeoutDuration() = %v, want 5m (default for invalid)", got)
	}
}

func TestParseDrainTimeout(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "worker"
start_command = "echo hello"

[agents.pool]
min = 0
max = 5
check = "echo 3"
drain_timeout = "2m"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(cfg.Agents))
	}
	a := cfg.Agents[0]
	if a.Pool == nil {
		t.Fatal("Pool is nil, want non-nil")
	}
	if a.Pool.DrainTimeout != "2m" {
		t.Errorf("Pool.DrainTimeout = %q, want %q", a.Pool.DrainTimeout, "2m")
	}
	got := a.Pool.DrainTimeoutDuration()
	if got != 2*time.Minute {
		t.Errorf("DrainTimeoutDuration() = %v, want 2m", got)
	}
}

func TestDrainTimeoutRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents: []Agent{{
			Name: "worker",
			Pool: &PoolConfig{Min: 0, Max: 5, Check: "echo 3", DrainTimeout: "3m"},
		}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Agents[0].Pool.DrainTimeout != "3m" {
		t.Errorf("DrainTimeout after round-trip = %q, want %q", got.Agents[0].Pool.DrainTimeout, "3m")
	}
}

func TestDrainTimeoutOmittedWhenEmpty(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents: []Agent{{
			Name: "worker",
			Pool: &PoolConfig{Min: 0, Max: 5, Check: "echo 3"},
		}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "drain_timeout") {
		t.Errorf("Marshal output should not contain 'drain_timeout' when empty:\n%s", data)
	}
}

func TestRigsParsing(t *testing.T) {
	input := `
[workspace]
name = "my-city"

[[agents]]
name = "mayor"

[[rigs]]
name = "frontend"
path = "/home/user/projects/my-frontend"
prefix = "fe"

[[rigs]]
name = "backend"
path = "/home/user/projects/my-backend"
`
	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Rigs) != 2 {
		t.Fatalf("len(Rigs) = %d, want 2", len(cfg.Rigs))
	}
	if cfg.Rigs[0].Name != "frontend" {
		t.Errorf("Rigs[0].Name = %q, want %q", cfg.Rigs[0].Name, "frontend")
	}
	if cfg.Rigs[0].Path != "/home/user/projects/my-frontend" {
		t.Errorf("Rigs[0].Path = %q, want %q", cfg.Rigs[0].Path, "/home/user/projects/my-frontend")
	}
	if cfg.Rigs[0].Prefix != "fe" {
		t.Errorf("Rigs[0].Prefix = %q, want %q", cfg.Rigs[0].Prefix, "fe")
	}
	if cfg.Rigs[1].Name != "backend" {
		t.Errorf("Rigs[1].Name = %q, want %q", cfg.Rigs[1].Name, "backend")
	}
	if cfg.Rigs[1].Prefix != "" {
		t.Errorf("Rigs[1].Prefix = %q, want empty (derived at runtime)", cfg.Rigs[1].Prefix)
	}
}

func TestRigsRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
		Rigs: []Rig{
			{Name: "frontend", Path: "/home/user/frontend", Prefix: "fe"},
			{Name: "backend", Path: "/home/user/backend"},
		},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if len(got.Rigs) != 2 {
		t.Fatalf("len(Rigs) after round-trip = %d, want 2", len(got.Rigs))
	}
	if got.Rigs[0].Prefix != "fe" {
		t.Errorf("Rigs[0].Prefix after round-trip = %q, want %q", got.Rigs[0].Prefix, "fe")
	}
	if got.Rigs[1].Path != "/home/user/backend" {
		t.Errorf("Rigs[1].Path after round-trip = %q, want %q", got.Rigs[1].Path, "/home/user/backend")
	}
}

// --- DeriveBeadsPrefix tests ---

func TestDeriveBeadsPrefix(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"my-frontend", "mf"},
		{"my-backend", "mb"},
		{"backend", "ba"},
		{"frontend", "fr"},
		{"tower-of-hanoi", "toh"},
		{"api", "api"},
		{"db", "db"},
		{"x", "x"},
		{"myFrontend", "mf"},
		{"GasCity", "gc"},
		{"my-project-go", "mp"}, // strip -go suffix
		{"my-project-py", "mp"}, // strip -py suffix
		{"hello_world", "hw"},
		{"a-b-c-d", "abcd"},
		{"longname", "lo"},
	}
	for _, tt := range tests {
		got := DeriveBeadsPrefix(tt.name)
		if got != tt.want {
			t.Errorf("DeriveBeadsPrefix(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSplitCompoundWord(t *testing.T) {
	tests := []struct {
		word string
		want []string
	}{
		{"myFrontend", []string{"my", "Frontend"}},
		{"GasCity", []string{"Gas", "City"}},
		{"simple", []string{"simple"}},
		{"ABC", []string{"ABC"}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		got := splitCompoundWord(tt.word)
		if len(got) != len(tt.want) {
			t.Errorf("splitCompoundWord(%q) = %v, want %v", tt.word, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCompoundWord(%q)[%d] = %q, want %q", tt.word, i, got[i], tt.want[i])
			}
		}
	}
}

func TestEffectivePrefix_Explicit(t *testing.T) {
	r := Rig{Name: "frontend", Path: "/path", Prefix: "fe"}
	if got := r.EffectivePrefix(); got != "fe" {
		t.Errorf("EffectivePrefix() = %q, want %q", got, "fe")
	}
}

func TestEffectivePrefix_Derived(t *testing.T) {
	r := Rig{Name: "my-frontend", Path: "/path"}
	if got := r.EffectivePrefix(); got != "mf" {
		t.Errorf("EffectivePrefix() = %q, want %q", got, "mf")
	}
}

// --- ValidateRigs tests ---

func TestValidateRigs_Valid(t *testing.T) {
	rigs := []Rig{
		{Name: "frontend", Path: "/home/user/frontend", Prefix: "fe"},
		{Name: "backend", Path: "/home/user/backend"},
	}
	if err := ValidateRigs(rigs, "my-city"); err != nil {
		t.Errorf("ValidateRigs: unexpected error: %v", err)
	}
}

func TestValidateRigs_Empty(t *testing.T) {
	if err := ValidateRigs(nil, "my-city"); err != nil {
		t.Errorf("ValidateRigs(nil): unexpected error: %v", err)
	}
}

func TestValidateRigs_MissingName(t *testing.T) {
	rigs := []Rig{{Path: "/path"}}
	err := ValidateRigs(rigs, "city")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want 'name is required'", err)
	}
}

func TestValidateRigs_MissingPath(t *testing.T) {
	rigs := []Rig{{Name: "frontend"}}
	err := ValidateRigs(rigs, "city")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("error = %q, want 'path is required'", err)
	}
}

func TestValidateRigs_DuplicateName(t *testing.T) {
	rigs := []Rig{
		{Name: "frontend", Path: "/a"},
		{Name: "frontend", Path: "/b"},
	}
	err := ValidateRigs(rigs, "city")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate'", err)
	}
}

// Regression: Bug 3 — prefix collisions between rigs must be detected.
func TestValidateRigs_PrefixCollision(t *testing.T) {
	rigs := []Rig{
		{Name: "my-frontend", Path: "/a"}, // prefix "mf"
		{Name: "my-foo", Path: "/b"},      // prefix "mf" — collision!
	}
	err := ValidateRigs(rigs, "city")
	if err == nil {
		t.Fatal("expected error for prefix collision")
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Errorf("error = %q, want 'collides'", err)
	}
}

// Regression: Bug 3 — prefix collision with HQ must also be detected.
func TestValidateRigs_PrefixCollidesWithHQ(t *testing.T) {
	// City name "my-city" → HQ prefix "mc"
	rigs := []Rig{
		{Name: "my-cloud", Path: "/path"}, // prefix "mc" — collides with HQ!
	}
	err := ValidateRigs(rigs, "my-city")
	if err == nil {
		t.Fatal("expected error for prefix collision with HQ")
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Errorf("error = %q, want 'collides'", err)
	}
	if !strings.Contains(err.Error(), "HQ") {
		t.Errorf("error = %q, want mention of HQ", err)
	}
}

func TestValidateRigs_ExplicitPrefixAvoidsCollision(t *testing.T) {
	// Same derived prefix but explicit override avoids collision.
	rigs := []Rig{
		{Name: "my-frontend", Path: "/a"},            // derived "mf"
		{Name: "my-foo", Path: "/b", Prefix: "mfoo"}, // explicit — no collision
	}
	if err := ValidateRigs(rigs, "city"); err != nil {
		t.Errorf("ValidateRigs: unexpected error: %v", err)
	}
}

// --- Suspended field tests ---

func TestParseSuspended(t *testing.T) {
	data := []byte(`
[workspace]
name = "test"

[[agents]]
name = "mayor"

[[agents]]
name = "builder"
suspended = true
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Suspended {
		t.Error("Agents[0].Suspended = true, want false")
	}
	if !cfg.Agents[1].Suspended {
		t.Error("Agents[1].Suspended = false, want true")
	}
}

func TestMarshalOmitsSuspendedFalse(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "suspended") {
		t.Errorf("Marshal output should not contain 'suspended' when false:\n%s", data)
	}
}

func TestMarshalIncludesSuspendedTrue(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "builder", Suspended: true}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), "suspended = true") {
		t.Errorf("Marshal output should contain 'suspended = true':\n%s", data)
	}
}

func TestSuspendedRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents: []Agent{
			{Name: "mayor"},
			{Name: "builder", Suspended: true},
		},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal output): %v", err)
	}
	if got.Agents[0].Suspended {
		t.Error("Agents[0].Suspended after round-trip = true, want false")
	}
	if !got.Agents[1].Suspended {
		t.Error("Agents[1].Suspended after round-trip = false, want true")
	}
}

func TestRigsOmittedWhenEmpty(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "rigs") {
		t.Errorf("Marshal output should not contain 'rigs' when empty:\n%s", data)
	}
}

// --- QualifiedName tests ---

func TestQualifiedName(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "mayor", dir: "", want: "mayor"},
		{name: "polecat", dir: "hello-world", want: "hello-world/polecat"},
		{name: "worker-1", dir: "backend", want: "backend/worker-1"},
	}
	for _, tt := range tests {
		a := Agent{Name: tt.name, Dir: tt.dir}
		got := a.QualifiedName()
		if got != tt.want {
			t.Errorf("Agent{Name:%q, Dir:%q}.QualifiedName() = %q, want %q",
				tt.name, tt.dir, got, tt.want)
		}
	}
}

func TestParseQualifiedName(t *testing.T) {
	tests := []struct {
		input   string
		wantDir string
		wantN   string
	}{
		{"mayor", "", "mayor"},
		{"hello-world/polecat", "hello-world", "polecat"},
		{"backend/worker-1", "backend", "worker-1"},
		{"deep/nested/name", "deep/nested", "name"},
	}
	for _, tt := range tests {
		dir, name := ParseQualifiedName(tt.input)
		if dir != tt.wantDir || name != tt.wantN {
			t.Errorf("ParseQualifiedName(%q) = (%q, %q), want (%q, %q)",
				tt.input, dir, name, tt.wantDir, tt.wantN)
		}
	}
}

func TestValidateAgentsSameNameDifferentDir(t *testing.T) {
	agents := []Agent{
		{Name: "polecat", Dir: "frontend"},
		{Name: "polecat", Dir: "backend"},
	}
	if err := ValidateAgents(agents); err != nil {
		t.Errorf("ValidateAgents: unexpected error for same name different dir: %v", err)
	}
}

func TestValidateAgentsSameNameSameDir(t *testing.T) {
	agents := []Agent{
		{Name: "polecat", Dir: "frontend"},
		{Name: "polecat", Dir: "frontend"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for same name same dir")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate'", err)
	}
}

func TestValidateAgentsSameNameCityWide(t *testing.T) {
	// Two city-wide agents with the same name should still be rejected.
	agents := []Agent{
		{Name: "worker"},
		{Name: "worker"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for duplicate city-wide name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate'", err)
	}
}

func TestValidateAgentsDupNameWithProvenance(t *testing.T) {
	// When both agents have SourceDir set, the error should include provenance.
	agents := []Agent{
		{Name: "worker", Dir: "myrig", SourceDir: "topologies/base"},
		{Name: "worker", Dir: "myrig", SourceDir: "topologies/extras"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "topologies/base") {
		t.Errorf("error should include first source dir, got: %s", errStr)
	}
	if !strings.Contains(errStr, "topologies/extras") {
		t.Errorf("error should include second source dir, got: %s", errStr)
	}
}

func TestValidateAgentsDupNameMixedProvenance(t *testing.T) {
	// Inline agent (no SourceDir) colliding with topology agent (has SourceDir)
	// should still include the available provenance.
	agents := []Agent{
		{Name: "worker"},
		{Name: "worker", SourceDir: "topologies/extras"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "topologies/extras") {
		t.Errorf("error should include source dir, got: %s", errStr)
	}
}

func TestValidateAgentsDupNameNoProvenance(t *testing.T) {
	// Two inline agents with no SourceDir — plain error without provenance.
	agents := []Agent{
		{Name: "worker"},
		{Name: "worker"},
	}
	err := ValidateAgents(agents)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "duplicate name") {
		t.Errorf("error should say 'duplicate name', got: %s", errStr)
	}
	// Should NOT contain "from" when neither has provenance.
	if strings.Contains(errStr, "from") {
		t.Errorf("error should not include provenance when neither has SourceDir, got: %s", errStr)
	}
}

// --- IdleTimeout tests ---

func TestIdleTimeoutDurationEmpty(t *testing.T) {
	a := Agent{Name: "mayor"}
	if got := a.IdleTimeoutDuration(); got != 0 {
		t.Errorf("IdleTimeoutDuration() = %v, want 0", got)
	}
}

func TestIdleTimeoutDurationValid(t *testing.T) {
	a := Agent{Name: "mayor", IdleTimeout: "15m"}
	if got := a.IdleTimeoutDuration(); got != 15*time.Minute {
		t.Errorf("IdleTimeoutDuration() = %v, want 15m", got)
	}
}

func TestIdleTimeoutDurationInvalid(t *testing.T) {
	a := Agent{Name: "mayor", IdleTimeout: "bogus"}
	if got := a.IdleTimeoutDuration(); got != 0 {
		t.Errorf("IdleTimeoutDuration() = %v, want 0 for invalid", got)
	}
}

func TestIdleTimeoutRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor", IdleTimeout: "30m"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Agents[0].IdleTimeout != "30m" {
		t.Errorf("IdleTimeout after round-trip = %q, want %q", got.Agents[0].IdleTimeout, "30m")
	}
}

func TestIdleTimeoutOmittedWhenEmpty(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "idle_timeout") {
		t.Errorf("TOML output should omit idle_timeout when empty, got:\n%s", data)
	}
}

// --- install_agent_hooks ---

func TestParseInstallAgentHooksWorkspace(t *testing.T) {
	toml := `
[workspace]
name = "test"
install_agent_hooks = ["claude", "gemini"]

[[agents]]
name = "mayor"
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Workspace.InstallAgentHooks) != 2 {
		t.Fatalf("Workspace.InstallAgentHooks = %v, want 2 entries", cfg.Workspace.InstallAgentHooks)
	}
	if cfg.Workspace.InstallAgentHooks[0] != "claude" || cfg.Workspace.InstallAgentHooks[1] != "gemini" {
		t.Errorf("Workspace.InstallAgentHooks = %v, want [claude gemini]", cfg.Workspace.InstallAgentHooks)
	}
}

func TestParseInstallAgentHooksAgent(t *testing.T) {
	toml := `
[workspace]
name = "test"
install_agent_hooks = ["claude"]

[[agents]]
name = "polecat"
install_agent_hooks = ["gemini", "copilot"]
`
	cfg, err := Parse([]byte(toml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Agents[0].InstallAgentHooks) != 2 {
		t.Fatalf("Agent.InstallAgentHooks = %v, want 2 entries", cfg.Agents[0].InstallAgentHooks)
	}
	if cfg.Agents[0].InstallAgentHooks[0] != "gemini" {
		t.Errorf("Agent.InstallAgentHooks[0] = %q, want gemini", cfg.Agents[0].InstallAgentHooks[0])
	}
}

func TestInstallAgentHooksRoundTrip(t *testing.T) {
	c := City{
		Workspace: Workspace{
			Name:              "test",
			InstallAgentHooks: []string{"claude", "copilot"},
		},
		Agents: []Agent{{
			Name:              "mayor",
			InstallAgentHooks: []string{"gemini"},
		}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	cfg2, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse roundtrip: %v", err)
	}
	if len(cfg2.Workspace.InstallAgentHooks) != 2 {
		t.Errorf("roundtrip workspace hooks = %v", cfg2.Workspace.InstallAgentHooks)
	}
	if len(cfg2.Agents[0].InstallAgentHooks) != 1 || cfg2.Agents[0].InstallAgentHooks[0] != "gemini" {
		t.Errorf("roundtrip agent hooks = %v", cfg2.Agents[0].InstallAgentHooks)
	}
}

func TestInstallAgentHooksOmittedWhenEmpty(t *testing.T) {
	c := City{
		Workspace: Workspace{Name: "test"},
		Agents:    []Agent{{Name: "mayor"}},
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "install_agent_hooks") {
		t.Errorf("TOML output should omit install_agent_hooks when empty, got:\n%s", data)
	}
}

// --- WispGC config tests ---

func TestDaemonConfig_WispGCDisabledByDefault(t *testing.T) {
	d := DaemonConfig{}
	if d.WispGCEnabled() {
		t.Error("wisp GC should be disabled by default")
	}
	if d.WispGCIntervalDuration() != 0 {
		t.Errorf("WispGCIntervalDuration = %v, want 0", d.WispGCIntervalDuration())
	}
	if d.WispTTLDuration() != 0 {
		t.Errorf("WispTTLDuration = %v, want 0", d.WispTTLDuration())
	}
}

func TestDaemonConfig_WispGCEnabled(t *testing.T) {
	d := DaemonConfig{
		WispGCInterval: "5m",
		WispTTL:        "24h",
	}
	if !d.WispGCEnabled() {
		t.Error("wisp GC should be enabled when both fields are set")
	}
	if d.WispGCIntervalDuration() != 5*time.Minute {
		t.Errorf("WispGCIntervalDuration = %v, want 5m", d.WispGCIntervalDuration())
	}
	if d.WispTTLDuration() != 24*time.Hour {
		t.Errorf("WispTTLDuration = %v, want 24h", d.WispTTLDuration())
	}
}

func TestDaemonConfig_WispGCPartialNotEnabled(t *testing.T) {
	// Only interval set.
	d := DaemonConfig{WispGCInterval: "5m"}
	if d.WispGCEnabled() {
		t.Error("wisp GC should not be enabled with only interval set")
	}

	// Only TTL set.
	d = DaemonConfig{WispTTL: "24h"}
	if d.WispGCEnabled() {
		t.Error("wisp GC should not be enabled with only TTL set")
	}

	// Invalid duration.
	d = DaemonConfig{WispGCInterval: "bad", WispTTL: "24h"}
	if d.WispGCEnabled() {
		t.Error("wisp GC should not be enabled with invalid interval")
	}
}
