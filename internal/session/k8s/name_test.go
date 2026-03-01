package k8s

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gc-bright-mayor", "gc-bright-mayor"},
		{"simple", "simple"},
		{"UPPER-CASE", "upper-case"},
		{"has spaces", "has-spaces"},
		{"has/slashes/in/path", "has-slashes-in-path"},
		{"under_scores", "under-scores"},
		{"--leading-dashes", "leading-dashes"},
		{"trailing-dashes--", "trailing-dashes"},
		{"dots.are.replaced", "dots-are-replaced"},
		{"a", "a"},
		{"", ""},
		// 70 chars should be truncated to 63 then trailing dashes trimmed.
		{"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01234567", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify result is valid K8s name (if non-empty).
			if got != "" {
				if len(got) > 63 {
					t.Errorf("result too long: %d chars", len(got))
				}
				if got[0] == '-' || got[len(got)-1] == '-' {
					t.Errorf("result starts/ends with dash: %q", got)
				}
			}
		})
	}
}

func TestSanitizeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gc-bright-mayor", "gc-bright-mayor"},
		{"simple", "simple"},
		{"has spaces", "has-spaces"},
		{"has/slashes", "has-slashes"},
		{"UPPER-Preserved", "UPPER-Preserved"},
		{"dots.ok", "dots.ok"},
		{"under_scores_ok", "under_scores_ok"},
		{"--leading", "leading"},
		{"trailing--", "trailing"},
		{"", "unknown"},
		{"///", "unknown"},
		{"a", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeLabel(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Verify result is valid K8s label value.
			if len(got) > 63 {
				t.Errorf("result too long: %d chars", len(got))
			}
			if got != "" && got != "unknown" {
				first, last := rune(got[0]), rune(got[len(got)-1])
				if !isAlphanumeric(first) || !isAlphanumeric(last) {
					t.Errorf("result starts/ends with non-alphanumeric: %q", got)
				}
			}
		})
	}
}
