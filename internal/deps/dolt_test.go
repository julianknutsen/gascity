package deps

import "testing"

func TestParseDoltVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dolt version 1.83.1\n", "1.83.1"},
		{"dolt version 1.82.4\n", "1.82.4"},
		{"dolt version 1.83.1 (abc1234)\n", "1.83.1"},
		{"garbage output\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseDoltVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseDoltVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
