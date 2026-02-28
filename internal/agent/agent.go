// Package agent provides the Agent interface for managed agent lifecycle.
//
// An Agent encapsulates identity (name, session name) and lifecycle
// operations (start, stop, attach) backed by a [session.Provider].
// The CLI layer builds agents from config; the do* functions operate
// on them without knowing how sessions are implemented.
package agent

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/steveyegge/gascity/internal/session"
)

// Agent represents a managed agent in the city.
type Agent interface {
	// Name returns the agent's configured name.
	Name() string

	// SessionName returns the session identifier for this agent.
	SessionName() string

	// IsRunning reports whether the agent's session is active.
	IsRunning() bool

	// Start creates the agent's session.
	Start() error

	// Stop destroys the agent's session.
	Stop() error

	// Attach connects the user's terminal to the agent's session.
	Attach() error

	// Nudge sends a message to wake or redirect the agent.
	Nudge(message string) error

	// Peek captures the last N lines of the agent's session output.
	Peek(lines int) (string, error)

	// SessionConfig returns the session.Config this agent would use
	// when starting. Used by reconciliation to compute config fingerprints
	// without actually starting the agent.
	SessionConfig() session.Config
}

// StartupHints carries provider startup behavior from config resolution
// through to session.Config. All fields are optional — zero values mean
// no special startup handling (fire-and-forget).
type StartupHints struct {
	ReadyPromptPrefix      string
	ReadyDelayMs           int
	ProcessNames           []string
	EmitsPermissionWarning bool
	// Nudge is text typed into the session after the agent is ready.
	// Used for CLI agents that don't accept command-line prompts.
	Nudge string
	// PreStart is a list of shell commands run before session creation.
	// Already template-expanded by the caller.
	PreStart []string
	// SessionSetup is a list of shell commands run after session creation.
	// Already template-expanded by the caller.
	SessionSetup []string
	// SessionSetupScript is a script path run after session_setup commands.
	SessionSetupScript string
	// OverlayDir is the resolved overlay directory path on the host.
	// Passed through to the exec session provider for remote copy.
	OverlayDir string
}

// sessionData holds template variables for custom session naming.
type sessionData struct {
	City  string // workspace name
	Agent string // tmux-safe qualified name (/ → --)
	Dir   string // rig/dir component (empty for singletons)
	Name  string // bare agent name
}

// SessionNameFor returns the session name for a city agent.
// This is the single source of truth for the naming convention.
// sessionTemplate is a Go text/template string; empty means use the
// default pattern "gc-{city}-{agent}".
//
// For rig-scoped agents (name contains "/"), the dir and name
// components are joined with "--" to avoid tmux naming issues:
//
//	"mayor"               → "gc-bright-lights-mayor"
//	"hello-world/polecat" → "gc-bright-lights-hello-world--polecat"
func SessionNameFor(cityName, agentName, sessionTemplate string) string {
	// Pre-sanitize: replace "/" with "--" for tmux safety.
	sanitized := strings.ReplaceAll(agentName, "/", "--")

	if sessionTemplate == "" {
		// Default: reproduce hardcoded pattern.
		return "gc-" + cityName + "-" + sanitized
	}

	// Parse dir/name components for template variables.
	var dir, name string
	if i := strings.LastIndex(agentName, "/"); i >= 0 {
		dir = agentName[:i]
		name = agentName[i+1:]
	} else {
		name = agentName
	}

	tmpl, err := template.New("session").Parse(sessionTemplate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gc: session_template parse error: %v (using default)\n", err)
		return "gc-" + cityName + "-" + sanitized
	}

	var buf bytes.Buffer
	data := sessionData{
		City:  cityName,
		Agent: sanitized,
		Dir:   dir,
		Name:  name,
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		fmt.Fprintf(os.Stderr, "gc: session_template execute error: %v (using default)\n", err)
		return "gc-" + cityName + "-" + sanitized
	}
	return buf.String()
}

// New creates an Agent backed by the given session provider.
// name is the agent's configured name (from TOML). cityName is the city's
// workspace name — used to derive the session name. prompt is the agent's
// initial prompt content (appended to command via shell quoting). env is
// additional environment variables for the session. hints carries provider
// startup behavior for session readiness detection. workDir is the working
// directory for the agent's session (empty means provider default).
// sessionTemplate is a Go text/template for session naming (empty = default).
// fpExtra carries additional data for config fingerprinting (e.g.
// pool config) that isn't part of the session command.
func New(name, cityName, command, prompt string,
	env map[string]string, hints StartupHints, workDir string,
	sessionTemplate string,
	fpExtra map[string]string,
	sp session.Provider,
) Agent {
	return &managed{
		name:        name,
		sessionName: SessionNameFor(cityName, name, sessionTemplate),
		command:     command,
		prompt:      prompt,
		env:         env,
		hints:       hints,
		workDir:     workDir,
		fpExtra:     fpExtra,
		sp:          sp,
	}
}

// managed is the concrete Agent implementation that delegates to a
// session.Provider using the agent's session name.
type managed struct {
	name        string
	sessionName string
	command     string
	prompt      string
	env         map[string]string
	hints       StartupHints
	workDir     string
	fpExtra     map[string]string
	sp          session.Provider
}

func (a *managed) Name() string        { return a.name }
func (a *managed) SessionName() string { return a.sessionName }
func (a *managed) IsRunning() bool {
	if !a.sp.IsRunning(a.sessionName) {
		return false
	}
	return a.sp.ProcessAlive(a.sessionName, a.hints.ProcessNames)
}
func (a *managed) Stop() error                { return a.sp.Stop(a.sessionName) }
func (a *managed) Attach() error              { return a.sp.Attach(a.sessionName) }
func (a *managed) Nudge(message string) error { return a.sp.Nudge(a.sessionName, message) }
func (a *managed) Peek(lines int) (string, error) {
	return a.sp.Peek(a.sessionName, lines)
}

// SessionConfig returns the session.Config this agent would use when starting.
func (a *managed) SessionConfig() session.Config {
	cmd := a.command
	if a.prompt != "" {
		cmd = cmd + " " + shellQuote(a.prompt)
	}
	return session.Config{
		Command:                cmd,
		Env:                    a.env,
		WorkDir:                a.workDir,
		ReadyPromptPrefix:      a.hints.ReadyPromptPrefix,
		ReadyDelayMs:           a.hints.ReadyDelayMs,
		ProcessNames:           a.hints.ProcessNames,
		EmitsPermissionWarning: a.hints.EmitsPermissionWarning,
		Nudge:                  a.hints.Nudge,
		PreStart:               a.hints.PreStart,
		SessionSetup:           a.hints.SessionSetup,
		SessionSetupScript:     a.hints.SessionSetupScript,
		OverlayDir:             a.hints.OverlayDir,
		FingerprintExtra:       a.fpExtra,
	}
}

func (a *managed) Start() error {
	return a.sp.Start(a.sessionName, a.SessionConfig())
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// using the standard shell idiom: replace ' with '\”.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
