package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/plugins"
)

// --- gc plugin list ---

func TestPluginListEmpty(t *testing.T) {
	var stdout bytes.Buffer
	code := doPluginList(nil, &stdout)
	if code != 0 {
		t.Fatalf("doPluginList = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No plugins found") {
		t.Errorf("stdout = %q, want 'No plugins found'", stdout.String())
	}
}

func TestPluginList(t *testing.T) {
	plugins := []plugins.Plugin{
		{Name: "digest", Gate: "cooldown", Interval: "24h", Pool: "dog", Formula: "mol-digest"},
		{Name: "cleanup", Gate: "cron", Schedule: "0 3 * * *", Formula: "mol-cleanup"},
		{Name: "deploy", Gate: "manual", Formula: "mol-deploy"},
	}

	var stdout bytes.Buffer
	code := doPluginList(plugins, &stdout)
	if code != 0 {
		t.Fatalf("doPluginList = %d, want 0", code)
	}
	out := stdout.String()
	for _, want := range []string{"digest", "cooldown", "24h", "dog", "cleanup", "cron", "deploy", "manual"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

// --- gc plugin show ---

func TestPluginShow(t *testing.T) {
	plugins := []plugins.Plugin{
		{
			Name:        "digest",
			Description: "Generate daily digest",
			Formula:     "mol-digest",
			Gate:        "cooldown",
			Interval:    "24h",
			Pool:        "dog",
			Source:      "/city/formulas/plugins/digest/plugins.toml",
		},
	}

	var stdout, stderr bytes.Buffer
	code := doPluginShow(plugins, "digest", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doPluginShow = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"digest", "Generate daily digest", "mol-digest", "cooldown", "24h", "dog", "plugins.toml"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestPluginShowNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doPluginShow(nil, "nonexistent", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doPluginShow = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want 'not found'", stderr.String())
	}
}

// --- gc plugin check ---

func TestPluginCheck(t *testing.T) {
	plugins := []plugins.Plugin{
		{Name: "digest", Gate: "cooldown", Interval: "24h", Formula: "mol-digest"},
	}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	neverRan := func(_ string) (time.Time, error) { return time.Time{}, nil }

	var stdout bytes.Buffer
	code := doPluginCheck(plugins, now, neverRan, &stdout)
	if code != 0 {
		t.Fatalf("doPluginCheck = %d, want 0 (due)", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "digest") {
		t.Errorf("stdout missing 'digest':\n%s", out)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("stdout missing 'yes':\n%s", out)
	}
}

func TestPluginCheckNoneDue(t *testing.T) {
	plugins := []plugins.Plugin{
		{Name: "deploy", Gate: "manual", Formula: "mol-deploy"},
	}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	neverRan := func(_ string) (time.Time, error) { return time.Time{}, nil }

	var stdout bytes.Buffer
	code := doPluginCheck(plugins, now, neverRan, &stdout)
	if code != 1 {
		t.Fatalf("doPluginCheck = %d, want 1 (none due)", code)
	}
}

func TestPluginCheckEmpty(t *testing.T) {
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	neverRan := func(_ string) (time.Time, error) { return time.Time{}, nil }

	var stdout bytes.Buffer
	code := doPluginCheck(nil, now, neverRan, &stdout)
	if code != 1 {
		t.Fatalf("doPluginCheck = %d, want 1 (empty)", code)
	}
}

// --- gc plugin run ---

func TestPluginRun(t *testing.T) {
	plugins := []plugins.Plugin{
		{Name: "digest", Formula: "mol-digest", Gate: "cooldown", Interval: "24h", Pool: "dog"},
	}

	// BdStore handles mol cook now.
	store := beads.NewBdStore(t.TempDir(), func(_, _ string, _ ...string) ([]byte, error) {
		return []byte("WISP-001\n"), nil
	})

	// SlingRunner still handles the route command.
	calls := []string{}
	fakeRunner := func(cmd string) (string, error) {
		calls = append(calls, cmd)
		return "", nil
	}

	var stdout, stderr bytes.Buffer
	code := doPluginRun(plugins, "digest", fakeRunner, store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doPluginRun = %d, want 0; stderr: %s", code, stderr.String())
	}

	if len(calls) != 1 {
		t.Fatalf("got %d runner calls, want 1 (route only): %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "bd update WISP-001 --label=pool:dog") {
		t.Errorf("call[0] = %q, want bd update with pool label", calls[0])
	}
	if !strings.Contains(stdout.String(), "WISP-001") {
		t.Errorf("stdout missing wisp ID: %s", stdout.String())
	}
}

func TestPluginRunNotFound(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := doPluginRun(nil, "nonexistent", nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doPluginRun = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want 'not found'", stderr.String())
	}
}

// --- gc plugin history ---

func TestPluginHistory(t *testing.T) {
	var stdout bytes.Buffer
	code := doPluginHistory("", &stdout)
	if code != 0 {
		t.Fatalf("doPluginHistory = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No plugin history") {
		t.Errorf("stdout = %q, want 'No plugin history'", stdout.String())
	}
}

func TestPluginHistoryNamed(t *testing.T) {
	var stdout bytes.Buffer
	code := doPluginHistory("digest", &stdout)
	if code != 0 {
		t.Fatalf("doPluginHistory = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "digest") {
		t.Errorf("stdout = %q, want 'digest'", stdout.String())
	}
}
