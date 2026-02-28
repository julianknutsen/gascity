//go:build integration

package integration

import (
	"strings"
	"testing"
)

// TestE2E_Hook_NoWork verifies that gc hook exits 1 when the work query
// returns no output.
func TestE2E_Hook_NoWork(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "hooker",
				StartCommand: e2eSleepScript(),
				WorkQuery:    "exit 1",
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	_, err := gc(cityDir, "hook", "hooker")
	if err == nil {
		t.Error("gc hook should exit non-zero when work query fails")
	}
}

// TestE2E_Hook_WithWork verifies that gc hook exits 0 and outputs the
// work query result when work is available.
func TestE2E_Hook_WithWork(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "worker",
				StartCommand: e2eSleepScript(),
				WorkQuery:    "echo 'hook test work available'",
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// gc hook should find work (echo always succeeds).
	out, err := gc(cityDir, "hook", "worker")
	if err != nil {
		t.Fatalf("gc hook should exit 0 with work: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "hook test work available") {
		t.Errorf("hook output should contain work query result:\n%s", out)
	}
}

// TestE2E_Hook_Inject verifies that gc hook --inject wraps output in
// system-reminder tags and always exits 0.
func TestE2E_Hook_Inject(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "injectee",
				StartCommand: e2eSleepScript(),
				WorkQuery:    "echo 'inject hook work items'",
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Hook with --inject should wrap in system-reminder.
	out, err := gc(cityDir, "hook", "--inject", "injectee")
	if err != nil {
		t.Fatalf("gc hook --inject should exit 0: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "<system-reminder>") {
		t.Errorf("expected <system-reminder> in inject output:\n%s", out)
	}
	if !strings.Contains(out, "inject hook work items") {
		t.Errorf("expected work items in inject output:\n%s", out)
	}
}
