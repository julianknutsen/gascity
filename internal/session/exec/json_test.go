package exec //nolint:revive // internal package, always imported with alias

import (
	"encoding/json"
	"testing"

	"github.com/steveyegge/gascity/internal/session"
)

func TestMarshalStartConfig(t *testing.T) {
	cfg := session.Config{
		WorkDir:      "/tmp/work",
		Command:      "claude --dangerously-skip-permissions",
		Env:          map[string]string{"FOO": "bar", "BAZ": "qux"},
		ProcessNames: []string{"claude", "node"},
		Nudge:        "hello agent",
	}

	data, err := marshalStartConfig(cfg)
	if err != nil {
		t.Fatalf("marshalStartConfig: %v", err)
	}

	var got startConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if got.WorkDir != cfg.WorkDir {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, cfg.WorkDir)
	}
	if got.Command != cfg.Command {
		t.Errorf("Command = %q, want %q", got.Command, cfg.Command)
	}
	if got.Nudge != cfg.Nudge {
		t.Errorf("Nudge = %q, want %q", got.Nudge, cfg.Nudge)
	}
	if len(got.Env) != 2 || got.Env["FOO"] != "bar" || got.Env["BAZ"] != "qux" {
		t.Errorf("Env = %v, want %v", got.Env, cfg.Env)
	}
	if len(got.ProcessNames) != 2 || got.ProcessNames[0] != "claude" || got.ProcessNames[1] != "node" {
		t.Errorf("ProcessNames = %v, want %v", got.ProcessNames, cfg.ProcessNames)
	}
}

func TestMarshalStartConfig_empty(t *testing.T) {
	data, err := marshalStartConfig(session.Config{})
	if err != nil {
		t.Fatalf("marshalStartConfig: %v", err)
	}

	// Empty config should produce minimal JSON (omitempty).
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// All fields have omitempty, so empty config → empty object.
	if len(got) != 0 {
		t.Errorf("expected empty JSON object, got %v", got)
	}
}

func TestMarshalStartConfig_doesNotLeakSessionFields(t *testing.T) {
	// Fields like ReadyPromptPrefix, ReadyDelayMs, EmitsPermissionWarning,
	// SessionSetup, SessionSetupScript, FingerprintExtra should NOT appear
	// in the JSON — they are gc-internal, not part of the exec protocol.
	cfg := session.Config{
		Command:                "test",
		ReadyPromptPrefix:      "> ",
		ReadyDelayMs:           500,
		EmitsPermissionWarning: true,
		SessionSetup:           []string{"echo setup"},
		SessionSetupScript:     "/tmp/setup.sh",
		FingerprintExtra:       map[string]string{"x": "y"},
	}

	data, err := marshalStartConfig(cfg)
	if err != nil {
		t.Fatalf("marshalStartConfig: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	leaked := []string{
		"ready_prompt_prefix", "ready_delay_ms", "emits_permission_warning",
		"session_setup", "session_setup_script", "fingerprint_extra",
	}
	for _, key := range leaked {
		if _, ok := got[key]; ok {
			t.Errorf("leaked internal field %q in JSON output", key)
		}
	}
}
