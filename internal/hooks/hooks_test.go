package hooks

import (
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestSupportedProviders(t *testing.T) {
	got := SupportedProviders()
	if len(got) != 4 {
		t.Fatalf("SupportedProviders() = %v, want 4 entries", got)
	}
	want := map[string]bool{"claude": true, "gemini": true, "opencode": true, "copilot": true}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected provider %q", p)
		}
	}
}

func TestValidateAcceptsSupported(t *testing.T) {
	if err := Validate([]string{"claude", "gemini"}); err != nil {
		t.Errorf("Validate([claude gemini]) = %v, want nil", err)
	}
}

func TestValidateRejectsUnsupported(t *testing.T) {
	err := Validate([]string{"claude", "codex", "bogus"})
	if err == nil {
		t.Fatal("Validate should reject codex and bogus")
	}
	if !strings.Contains(err.Error(), "codex (no hook mechanism)") {
		t.Errorf("error should mention codex: %v", err)
	}
	if !strings.Contains(err.Error(), "bogus (unknown)") {
		t.Errorf("error should mention bogus: %v", err)
	}
}

func TestValidateEmpty(t *testing.T) {
	if err := Validate(nil); err != nil {
		t.Errorf("Validate(nil) = %v, want nil", err)
	}
}

func TestInstallClaude(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"claude"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, ok := fs.Files["/city/.gc/settings.json"]
	if !ok {
		t.Fatal("expected /city/.gc/settings.json to be written")
	}
	if !strings.Contains(string(data), "SessionStart") {
		t.Error("claude settings should contain SessionStart hook")
	}
	if !strings.Contains(string(data), "gc prime") {
		t.Error("claude settings should contain gc prime")
	}
}

func TestInstallGemini(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"gemini"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, ok := fs.Files["/work/.gemini/settings.json"]
	if !ok {
		t.Fatal("expected /work/.gemini/settings.json to be written")
	}
	if !strings.Contains(string(data), "PreCompress") {
		t.Error("gemini settings should contain PreCompress hook")
	}
	if !strings.Contains(string(data), "BeforeAgent") {
		t.Error("gemini settings should contain BeforeAgent hook")
	}
}

func TestInstallOpenCode(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"opencode"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, ok := fs.Files["/work/.opencode/plugins/gascity.js"]
	if !ok {
		t.Fatal("expected /work/.opencode/plugins/gascity.js to be written")
	}
	if !strings.Contains(string(data), "gc prime") {
		t.Error("opencode plugin should contain gc prime")
	}
}

func TestInstallCopilot(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"copilot"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, ok := fs.Files["/work/.github/copilot-instructions.md"]
	if !ok {
		t.Fatal("expected /work/.github/copilot-instructions.md to be written")
	}
	if !strings.Contains(string(data), "gc prime") {
		t.Error("copilot instructions should contain gc prime")
	}
}

func TestInstallMultipleProviders(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"claude", "gemini", "copilot"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, ok := fs.Files["/city/.gc/settings.json"]; !ok {
		t.Error("missing claude settings")
	}
	if _, ok := fs.Files["/work/.gemini/settings.json"]; !ok {
		t.Error("missing gemini settings")
	}
	if _, ok := fs.Files["/work/.github/copilot-instructions.md"]; !ok {
		t.Error("missing copilot instructions")
	}
}

func TestInstallIdempotent(t *testing.T) {
	fs := fsys.NewFake()
	// Pre-populate with custom content.
	fs.Files["/city/.gc/settings.json"] = []byte(`{"custom": true}`)

	err := Install(fs, "/city", "/work", []string{"claude"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Should not overwrite existing file.
	got := string(fs.Files["/city/.gc/settings.json"])
	if got != `{"custom": true}` {
		t.Errorf("Install overwrote existing file: got %q", got)
	}
}

func TestInstallUnknownProvider(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", []string{"bogus"})
	if err == nil {
		t.Fatal("Install should reject unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention unsupported: %v", err)
	}
}

func TestInstallEmpty(t *testing.T) {
	fs := fsys.NewFake()
	err := Install(fs, "/city", "/work", nil)
	if err != nil {
		t.Fatalf("Install(nil) = %v, want nil", err)
	}
}
