package session

import "testing"

func TestConfigFingerprintDeterministic(t *testing.T) {
	cfg := Config{Command: "claude --skip", Env: map[string]string{"A": "1", "B": "2"}}
	h1 := ConfigFingerprint(cfg)
	h2 := ConfigFingerprint(cfg)
	if h1 != h2 {
		t.Errorf("same config produced different hashes: %q vs %q", h1, h2)
	}
}

func TestConfigFingerprintDifferentCommand(t *testing.T) {
	a := Config{Command: "claude"}
	b := Config{Command: "codex"}
	if ConfigFingerprint(a) == ConfigFingerprint(b) {
		t.Error("different commands should produce different hashes")
	}
}

func TestConfigFingerprintDifferentEnv(t *testing.T) {
	a := Config{Command: "claude", Env: map[string]string{"A": "1"}}
	b := Config{Command: "claude", Env: map[string]string{"A": "2"}}
	if ConfigFingerprint(a) == ConfigFingerprint(b) {
		t.Error("different env values should produce different hashes")
	}
}

func TestConfigFingerprintEnvOrderIndependent(t *testing.T) {
	// Go maps don't guarantee order, but we verify via two configs
	// with the same key-value pairs that the hash is stable.
	a := Config{Command: "claude", Env: map[string]string{"Z": "last", "A": "first", "M": "mid"}}
	b := Config{Command: "claude", Env: map[string]string{"M": "mid", "A": "first", "Z": "last"}}
	if ConfigFingerprint(a) != ConfigFingerprint(b) {
		t.Error("env order should not affect hash")
	}
}

func TestConfigFingerprintIgnoresReadyDelayMs(t *testing.T) {
	a := Config{Command: "claude", ReadyDelayMs: 0}
	b := Config{Command: "claude", ReadyDelayMs: 5000}
	if ConfigFingerprint(a) != ConfigFingerprint(b) {
		t.Error("ReadyDelayMs should not affect hash")
	}
}

func TestConfigFingerprintIgnoresReadyPromptPrefix(t *testing.T) {
	a := Config{Command: "claude", ReadyPromptPrefix: ""}
	b := Config{Command: "claude", ReadyPromptPrefix: "> "}
	if ConfigFingerprint(a) != ConfigFingerprint(b) {
		t.Error("ReadyPromptPrefix should not affect hash")
	}
}

func TestConfigFingerprintNilVsEmptyEnv(t *testing.T) {
	a := Config{Command: "claude", Env: nil}
	b := Config{Command: "claude", Env: map[string]string{}}
	if ConfigFingerprint(a) != ConfigFingerprint(b) {
		t.Error("nil and empty env should produce the same hash")
	}
}

func TestConfigFingerprintEmptyConfig(t *testing.T) {
	h := ConfigFingerprint(Config{})
	if h == "" {
		t.Error("empty config should still produce a hash")
	}
	// Verify stability.
	if h != ConfigFingerprint(Config{}) {
		t.Error("empty config hash not stable")
	}
}
