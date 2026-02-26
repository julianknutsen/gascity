package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// PromptContext holds template data for prompt rendering.
type PromptContext struct {
	CityRoot     string
	AgentName    string // qualified: "rig/polecat-1" or "mayor"
	TemplateName string // config name: "polecat" (pool template) or "mayor" (singleton)
	RigName      string
	WorkDir      string
	IssuePrefix  string
	Branch       string
	WorkQuery    string            // command to find available work (from Agent.EffectiveWorkQuery)
	SlingQuery   string            // command template to route work to this agent (from Agent.EffectiveSlingQuery)
	Env          map[string]string // from Agent.Env — custom vars
}

// renderPrompt reads a prompt template file and renders it with the given
// context. cityName is used internally by template functions (e.g. session)
// but not exposed as a template variable. sessionTemplate is the custom
// session naming template (empty = default). Returns empty string if
// templatePath is empty or the file doesn't exist. On parse or execute error,
// logs a warning to stderr and returns the raw text (graceful fallback).
func renderPrompt(fs fsys.FS, cityPath, cityName, templatePath string, ctx PromptContext, sessionTemplate string, stderr io.Writer) string {
	if templatePath == "" {
		return ""
	}
	data, err := fs.ReadFile(filepath.Join(cityPath, templatePath))
	if err != nil {
		return ""
	}
	raw := string(data)

	tmpl := template.New("prompt").
		Funcs(promptFuncMap(cityName, sessionTemplate)).
		Option("missingkey=zero")

	// Load shared templates from sibling shared/ directory.
	sharedDir := filepath.Join(cityPath, filepath.Dir(templatePath), "shared")
	if entries, err := fs.ReadDir(sharedDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md.tmpl") {
				if sdata, err := fs.ReadFile(filepath.Join(sharedDir, e.Name())); err == nil {
					if _, err := tmpl.Parse(string(sdata)); err != nil {
						fmt.Fprintf(stderr, "gc: shared template %q: %v\n", e.Name(), err) //nolint:errcheck // best-effort stderr
					}
				}
			}
		}
	}

	// Parse main template last — its body becomes the "prompt" template.
	tmpl, err = tmpl.Parse(raw)
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
	m["AgentName"] = ctx.AgentName
	m["TemplateName"] = ctx.TemplateName
	m["RigName"] = ctx.RigName
	m["WorkDir"] = ctx.WorkDir
	m["IssuePrefix"] = ctx.IssuePrefix
	m["Branch"] = ctx.Branch
	m["WorkQuery"] = ctx.WorkQuery
	m["SlingQuery"] = ctx.SlingQuery
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
// sessionTemplate is the custom session naming template (empty = default).
func promptFuncMap(cityName, sessionTemplate string) template.FuncMap {
	return template.FuncMap{
		"cmd": func() string {
			return filepath.Base(os.Args[0])
		},
		"session": func(agentName string) string {
			return agent.SessionNameFor(cityName, agentName, sessionTemplate)
		},
		"basename": func(qualifiedName string) string {
			_, name := config.ParseQualifiedName(qualifiedName)
			return name
		},
	}
}
