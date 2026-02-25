package main

import (
	"io"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestRenderPromptEmptyPath(t *testing.T) {
	f := fsys.NewFake()
	got := renderPrompt(f, "/city", "", PromptContext{}, io.Discard)
	if got != "" {
		t.Errorf("renderPrompt(empty path) = %q, want empty", got)
	}
}

func TestRenderPromptMissingFile(t *testing.T) {
	f := fsys.NewFake()
	got := renderPrompt(f, "/city", "prompts/missing.md", PromptContext{}, io.Discard)
	if got != "" {
		t.Errorf("renderPrompt(missing) = %q, want empty", got)
	}
}

func TestRenderPromptNoExpressions(t *testing.T) {
	f := fsys.NewFake()
	content := "# Simple Prompt\n\nNo template expressions here.\n"
	f.Files["/city/prompts/plain.md"] = []byte(content)
	got := renderPrompt(f, "/city", "prompts/plain.md", PromptContext{}, io.Discard)
	if got != content {
		t.Errorf("renderPrompt(plain) = %q, want %q", got, content)
	}
}

func TestRenderPromptBasicVars(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("City: {{ .CityRoot }}\nName: {{ .CityName }}\nAgent: {{ .AgentName }}\n")
	ctx := PromptContext{
		CityRoot:  "/home/user/bright-lights",
		CityName:  "bright-lights",
		AgentName: "hello-world/polecat-1",
	}
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", ctx, io.Discard)
	want := "City: /home/user/bright-lights\nName: bright-lights\nAgent: hello-world/polecat-1\n"
	if got != want {
		t.Errorf("renderPrompt(vars) = %q, want %q", got, want)
	}
}

func TestRenderPromptInstanceName(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("Instance: {{ .InstanceName }}")
	ctx := PromptContext{InstanceName: "polecat-3"}
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", ctx, io.Discard)
	if got != "Instance: polecat-3" {
		t.Errorf("renderPrompt(instance) = %q, want %q", got, "Instance: polecat-3")
	}
}

func TestRenderPromptCmdFunction(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("Run `{{ cmd }}` to start")
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", PromptContext{}, io.Discard)
	// cmd returns filepath.Base(os.Args[0]) — in tests this is the test binary name.
	// Just verify it doesn't contain "{{ cmd }}" (i.e., the function was called).
	if strings.Contains(got, "{{ cmd }}") {
		t.Errorf("renderPrompt(cmd) still contains template expression: %q", got)
	}
	if !strings.Contains(got, "Run `") {
		t.Errorf("renderPrompt(cmd) missing prefix: %q", got)
	}
}

func TestRenderPromptSessionFunction(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte(`Session: {{ session "deacon" }}`)
	ctx := PromptContext{CityName: "gastown"}
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", ctx, io.Discard)
	if got != "Session: gc-gastown-deacon" {
		t.Errorf("renderPrompt(session) = %q, want %q", got, "Session: gc-gastown-deacon")
	}
}

func TestRenderPromptMissingKeyEmptyString(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("Branch: {{ .Branch }}")
	// Branch not set → should be empty string (missingkey=zero).
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", PromptContext{}, io.Discard)
	if got != "Branch: " {
		t.Errorf("renderPrompt(missing key) = %q, want %q", got, "Branch: ")
	}
}

func TestRenderPromptEnvMerge(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("Branch: {{ .DefaultBranch }}")
	ctx := PromptContext{
		Env: map[string]string{"DefaultBranch": "main"},
	}
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", ctx, io.Discard)
	if got != "Branch: main" {
		t.Errorf("renderPrompt(env) = %q, want %q", got, "Branch: main")
	}
}

func TestRenderPromptEnvOverridePriority(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/test.md.tmpl"] = []byte("Root: {{ .CityRoot }}")
	ctx := PromptContext{
		CityRoot: "/real/path",
		Env:      map[string]string{"CityRoot": "/env/path"},
	}
	got := renderPrompt(f, "/city", "prompts/test.md.tmpl", ctx, io.Discard)
	// SDK vars take priority over Env.
	if got != "Root: /real/path" {
		t.Errorf("renderPrompt(override) = %q, want %q", got, "Root: /real/path")
	}
}

func TestRenderPromptParseErrorFallback(t *testing.T) {
	f := fsys.NewFake()
	f.Files["/city/prompts/bad.md.tmpl"] = []byte("Bad: {{ .Unclosed")
	var stderr strings.Builder
	got := renderPrompt(f, "/city", "prompts/bad.md.tmpl", PromptContext{}, &stderr)
	// Should return raw text on parse error.
	if got != "Bad: {{ .Unclosed" {
		t.Errorf("renderPrompt(parse error) = %q, want raw text", got)
	}
	if !strings.Contains(stderr.String(), "prompt template") {
		t.Errorf("stderr = %q, want warning about prompt template", stderr.String())
	}
}

func TestRenderPromptReadError(t *testing.T) {
	f := fsys.NewFake()
	f.Errors["/city/prompts/broken.md"] = errExit
	got := renderPrompt(f, "/city", "prompts/broken.md", PromptContext{}, io.Discard)
	if got != "" {
		t.Errorf("renderPrompt(read error) = %q, want empty", got)
	}
}

func TestRenderPromptMultiVariable(t *testing.T) {
	f := fsys.NewFake()
	tmpl := `# {{ .AgentName }} in {{ .RigName }}
Working in {{ .WorkDir }}
City: {{ .CityRoot }} ({{ .CityName }})
Prefix: {{ .IssuePrefix }}
Branch: {{ .Branch }}
Run {{ cmd }} to start
Session: {{ session "deacon" }}
Custom: {{ .DefaultBranch }}
`
	f.Files["/city/prompts/full.md.tmpl"] = []byte(tmpl)
	ctx := PromptContext{
		CityRoot:     "/home/user/city",
		CityName:     "gastown",
		AgentName:    "myrig/polecat-1",
		InstanceName: "polecat-1",
		RigName:      "myrig",
		WorkDir:      "/home/user/city/myrig/polecats/polecat-1",
		IssuePrefix:  "mr-",
		Branch:       "feature/foo",
		Env:          map[string]string{"DefaultBranch": "main"},
	}
	got := renderPrompt(f, "/city", "prompts/full.md.tmpl", ctx, io.Discard)
	if !strings.Contains(got, "# myrig/polecat-1 in myrig") {
		t.Errorf("missing agent/rig: %q", got)
	}
	if !strings.Contains(got, "Working in /home/user/city/myrig/polecats/polecat-1") {
		t.Errorf("missing workdir: %q", got)
	}
	if !strings.Contains(got, "City: /home/user/city (gastown)") {
		t.Errorf("missing city: %q", got)
	}
	if !strings.Contains(got, "Prefix: mr-") {
		t.Errorf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "Branch: feature/foo") {
		t.Errorf("missing branch: %q", got)
	}
	if !strings.Contains(got, "Session: gc-gastown-deacon") {
		t.Errorf("missing session: %q", got)
	}
	if !strings.Contains(got, "Custom: main") {
		t.Errorf("missing env var: %q", got)
	}
}

func TestBuildTemplateData(t *testing.T) {
	ctx := PromptContext{
		CityRoot:     "/city",
		CityName:     "test",
		AgentName:    "a/b",
		InstanceName: "b",
		RigName:      "a",
		WorkDir:      "/city/a",
		IssuePrefix:  "te-",
		Branch:       "main",
		Env:          map[string]string{"Custom": "val", "CityRoot": "override"},
	}
	data := buildTemplateData(ctx)
	// SDK vars override Env.
	if data["CityRoot"] != "/city" {
		t.Errorf("CityRoot = %q, want %q", data["CityRoot"], "/city")
	}
	if data["Custom"] != "val" {
		t.Errorf("Custom = %q, want %q", data["Custom"], "val")
	}
}

func TestBuildTemplateDataEmptyEnv(t *testing.T) {
	ctx := PromptContext{CityName: "test"}
	data := buildTemplateData(ctx)
	if data["CityName"] != "test" {
		t.Errorf("CityName = %q, want %q", data["CityName"], "test")
	}
}
