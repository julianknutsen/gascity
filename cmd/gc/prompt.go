package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// PromptContext holds template data for prompt rendering.
type PromptContext struct {
	CityRoot     string
	CityName     string
	AgentName    string
	InstanceName string
	RigName      string
	WorkDir      string
	IssuePrefix  string
	Branch       string
	Env          map[string]string // from Agent.Env — custom vars
}

// renderPrompt reads a prompt template file and renders it with the given
// context. Returns empty string if templatePath is empty or the file doesn't
// exist. On parse or execute error, logs a warning to stderr and returns the
// raw text (graceful fallback).
func renderPrompt(fs fsys.FS, cityPath, templatePath string, ctx PromptContext, stderr io.Writer) string {
	if templatePath == "" {
		return ""
	}
	data, err := fs.ReadFile(filepath.Join(cityPath, templatePath))
	if err != nil {
		return ""
	}
	raw := string(data)

	tmpl, err := template.New("prompt").
		Funcs(promptFuncMap(ctx.CityName)).
		Option("missingkey=zero").
		Parse(raw)
	if err != nil {
		fmt.Fprintf(stderr, "gc: prompt template %q: %v\n", templatePath, err) //nolint:errcheck // best-effort stderr
		return raw
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, buildTemplateData(ctx)); err != nil {
		fmt.Fprintf(stderr, "gc: prompt template %q: %v\n", templatePath, err) //nolint:errcheck // best-effort stderr
		return raw
	}
	return buf.String()
}

// buildTemplateData merges Env (lower priority) with SDK fields (higher
// priority) into a single map for template execution.
func buildTemplateData(ctx PromptContext) map[string]string {
	m := make(map[string]string, len(ctx.Env)+8)
	for k, v := range ctx.Env {
		m[k] = v
	}
	// SDK fields override Env.
	m["CityRoot"] = ctx.CityRoot
	m["CityName"] = ctx.CityName
	m["AgentName"] = ctx.AgentName
	m["InstanceName"] = ctx.InstanceName
	m["RigName"] = ctx.RigName
	m["WorkDir"] = ctx.WorkDir
	m["IssuePrefix"] = ctx.IssuePrefix
	m["Branch"] = ctx.Branch
	return m
}

// findRigPrefix returns the effective bead ID prefix for the named rig.
// Returns empty string if rigName is empty or not found.
func findRigPrefix(rigName string, rigs []config.Rig) string {
	for i := range rigs {
		if rigs[i].Name == rigName {
			return rigs[i].EffectivePrefix()
		}
	}
	return ""
}

// promptFuncMap returns template functions available in prompt templates.
func promptFuncMap(cityName string) template.FuncMap {
	return template.FuncMap{
		"cmd": func() string {
			return filepath.Base(os.Args[0])
		},
		"session": func(agentName string) string {
			return agent.SessionNameFor(cityName, agentName)
		},
	}
}
