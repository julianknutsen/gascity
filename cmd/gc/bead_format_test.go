package main

import (
	"testing"
)

func TestParseBeadFormat(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantFmt  string
		wantRest []string
	}{
		{"nil args", nil, "text", nil},
		{"empty args", []string{}, "text", nil},
		{"no format flag", []string{"gc-1"}, "text", []string{"gc-1"}},
		{"--format json", []string{"--format", "json", "gc-1"}, "json", []string{"gc-1"}},
		{"--format toon", []string{"--format", "toon"}, "toon", nil},
		{"--format=json", []string{"--format=json", "gc-1"}, "json", []string{"gc-1"}},
		{"--format=toon", []string{"--format=toon"}, "toon", nil},
		{"--format text", []string{"--format", "text"}, "text", nil},
		{"--json shorthand", []string{"--json", "gc-1"}, "json", []string{"gc-1"}},
		{"format after positional", []string{"gc-1", "--format", "json"}, "json", []string{"gc-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFmt, gotRest := parseBeadFormat(tt.args)
			if gotFmt != tt.wantFmt {
				t.Errorf("format = %q, want %q", gotFmt, tt.wantFmt)
			}
			if len(gotRest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", gotRest, tt.wantRest)
			} else {
				for i := range gotRest {
					if gotRest[i] != tt.wantRest[i] {
						t.Errorf("rest[%d] = %q, want %q", i, gotRest[i], tt.wantRest[i])
					}
				}
			}
		})
	}
}

func TestToonVal(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"has,comma", `"has,comma"`},
		{`has"quote`, `"has""quote"`},
		{"has\nnewline", `"has` + "\n" + `newline"`},
		{"", ""},
	}
	for _, tt := range tests {
		got := toonVal(tt.in)
		if got != tt.want {
			t.Errorf("toonVal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
