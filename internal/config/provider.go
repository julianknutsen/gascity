package config

import "strings"

// ProviderSpec defines a named provider's startup parameters.
// Built-in presets are returned by BuiltinProviders(). Users can override
// or define new providers via [providers.xxx] in city.toml.
type ProviderSpec struct {
	DisplayName            string            `toml:"display_name,omitempty"`
	Command                string            `toml:"command,omitempty"`
	Args                   []string          `toml:"args,omitempty"`
	PromptMode             string            `toml:"prompt_mode,omitempty"` // "arg", "flag", "none"
	PromptFlag             string            `toml:"prompt_flag,omitempty"` // e.g. "--prompt"
	ReadyDelayMs           int               `toml:"ready_delay_ms,omitempty"`
	ReadyPromptPrefix      string            `toml:"ready_prompt_prefix,omitempty"`
	ProcessNames           []string          `toml:"process_names,omitempty"`
	EmitsPermissionWarning bool              `toml:"emits_permission_warning,omitempty"`
	Env                    map[string]string `toml:"env,omitempty"`
}

// ResolvedProvider is the fully-merged, ready-to-use provider config.
// All fields are populated after resolution (built-in + city override + agent override).
type ResolvedProvider struct {
	Name                   string
	Command                string
	Args                   []string
	PromptMode             string
	PromptFlag             string
	ReadyDelayMs           int
	ReadyPromptPrefix      string
	ProcessNames           []string
	EmitsPermissionWarning bool
	Env                    map[string]string
}

// CommandString returns the full command line: command followed by args.
func (rp *ResolvedProvider) CommandString() string {
	if len(rp.Args) == 0 {
		return rp.Command
	}
	return rp.Command + " " + strings.Join(rp.Args, " ")
}

// builtinProviderOrder is the priority order for provider detection and
// wizard display. Claude is first (default), followed by major providers
// in rough popularity order.
var builtinProviderOrder = []string{
	"claude", "codex", "gemini", "cursor", "copilot",
	"amp", "opencode", "auggie", "pi",
}

// BuiltinProviderOrder returns the provider names in their canonical order.
// Used by the wizard for display and by auto-detection for priority.
func BuiltinProviderOrder() []string {
	out := make([]string, len(builtinProviderOrder))
	copy(out, builtinProviderOrder)
	return out
}

// BuiltinProviders returns the built-in provider presets.
// These are available without any [providers] section in city.toml.
// Lifted from gastown's AgentPresetInfo table — only startup-relevant
// fields are included (session resume, hooks, etc. are future work).
func BuiltinProviders() map[string]ProviderSpec {
	return map[string]ProviderSpec{
		"claude": {
			DisplayName:            "Claude Code",
			Command:                "claude",
			Args:                   []string{"--dangerously-skip-permissions"},
			PromptMode:             "arg",
			ReadyDelayMs:           10000,
			ReadyPromptPrefix:      "\u276f ", // ❯
			ProcessNames:           []string{"node", "claude"},
			EmitsPermissionWarning: true,
		},
		"codex": {
			DisplayName:  "Codex CLI",
			Command:      "codex",
			Args:         []string{"--dangerously-bypass-approvals-and-sandbox"},
			PromptMode:   "none",
			ReadyDelayMs: 3000,
			ProcessNames: []string{"codex"},
		},
		"gemini": {
			DisplayName:  "Gemini CLI",
			Command:      "gemini",
			Args:         []string{"--approval-mode", "yolo"},
			PromptMode:   "arg",
			ReadyDelayMs: 5000,
			ProcessNames: []string{"gemini"},
		},
		"cursor": {
			DisplayName:  "Cursor Agent",
			Command:      "cursor-agent",
			Args:         []string{"-f"},
			PromptMode:   "arg",
			ProcessNames: []string{"cursor-agent"},
		},
		"copilot": {
			DisplayName:       "GitHub Copilot",
			Command:           "copilot",
			Args:              []string{"--yolo"},
			PromptMode:        "arg",
			ReadyPromptPrefix: "\u276f ", // ❯
			ReadyDelayMs:      5000,
			ProcessNames:      []string{"copilot"},
		},
		"amp": {
			DisplayName:  "Sourcegraph AMP",
			Command:      "amp",
			Args:         []string{"--dangerously-allow-all", "--no-ide"},
			PromptMode:   "arg",
			ProcessNames: []string{"amp"},
		},
		"opencode": {
			DisplayName:  "OpenCode",
			Command:      "opencode",
			Args:         []string{},
			PromptMode:   "arg",
			ReadyDelayMs: 8000,
			ProcessNames: []string{"opencode", "node", "bun"},
			Env:          map[string]string{"OPENCODE_PERMISSION": `{"*":"allow"}`},
		},
		"auggie": {
			DisplayName:  "Auggie CLI",
			Command:      "auggie",
			Args:         []string{"--allow-indexing"},
			PromptMode:   "arg",
			ProcessNames: []string{"auggie"},
		},
		"pi": {
			DisplayName:  "Pi Agent",
			Command:      "pi",
			Args:         []string{},
			PromptMode:   "arg",
			ProcessNames: []string{"pi", "node", "bun"},
		},
	}
}
