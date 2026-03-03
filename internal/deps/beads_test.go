package deps

import "testing"

func TestParseBeadsVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bd version 0.58.0 (abc1234: abc1234abc12)\n", "0.58.0"},
		{"bd version 0.57.0\n", "0.57.0"},
		{"bd version 0.56.1 (ee75522c: ee75522cc098)\n", "0.56.1"},
		{"garbage output\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseBeadsVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseBeadsVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
