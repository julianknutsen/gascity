package deps

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.2.0", "1.1.9", 1},
		{"0.58.0", "0.57.0", 1},
		{"0.57.0", "0.58.0", -1},
		{"1.83.1", "1.82.4", 1},
		{"1.82.4", "1.83.1", -1},
		{"2.0.0", "1.99.99", 1},
	}
	for _, tt := range tests {
		got := CompareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.58.0", [3]int{0, 58, 0}},
		{"1.83", [3]int{1, 83, 0}},
		{"1", [3]int{1, 0, 0}},
		{"", [3]int{0, 0, 0}},
		{"abc", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		got := ParseVersion(tt.input)
		if got != tt.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
