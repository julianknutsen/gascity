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

func TestPluginLastRunFn(t *testing.T) {
	// Simulate a bead store that returns one result for "plugin-run:digest".
	store := beads.NewBdStore(t.TempDir(), func(_, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--label=plugin-run:digest") {
			return []byte(`[{"id":"bd-aaa","title":"digest wisp","status":"open","issue_type":"task","created_at":"2026-02-27T10:00:00Z","labels":["plugin-run:digest"]}]`), nil
		}
		return []byte(`[]`), nil
	})

	fn := pluginLastRunFn(store)

	// Known plugin — returns CreatedAt.
	got, err := fn("digest")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("lastRun = %v, want %v", got, want)
	}

	// Unknown plugin — returns zero time.
	got, err = fn("unknown")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("lastRun = %v, want zero time", got)
	}
}

func TestPluginCheckWithLastRun(t *testing.T) {
	pp := []plugins.Plugin{
		{Name: "digest", Gate: "cooldown", Interval: "24h", Formula: "mol-digest"},
	}
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.UTC)
	// Last ran 1 hour ago — cooldown of 24h means NOT due.
	recentRun := func(_ string) (time.Time, error) {
		return now.Add(-1 * time.Hour), nil
	}

	var stdout bytes.Buffer
	code := doPluginCheck(pp, now, recentRun, &stdout)
	if code != 1 {
		t.Fatalf("doPluginCheck = %d, want 1 (not due)", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "no") {
		t.Errorf("stdout missing 'no':\n%s", out)
	}
	if !strings.Contains(out, "cooldown") {
		t.Errorf("stdout missing 'cooldown':\n%s", out)
	}
}

// --- gc plugin run ---

func TestPluginRun(t *testing.T) {
	pp := []plugins.Plugin{
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
	code := doPluginRun(pp, "digest", fakeRunner, store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doPluginRun = %d, want 0; stderr: %s", code, stderr.String())
	}

	if len(calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(calls), calls)
	}
	// Should include both plugin-run label and pool label in a single bd update.
	if !strings.Contains(calls[0], "--label=plugin-run:digest") {
		t.Errorf("call[0] = %q, want --label=plugin-run:digest", calls[0])
	}
	if !strings.Contains(calls[0], "--label=pool:dog") {
		t.Errorf("call[0] = %q, want --label=pool:dog", calls[0])
	}
	if !strings.Contains(stdout.String(), "WISP-001") {
		t.Errorf("stdout missing wisp ID: %s", stdout.String())
	}
}

func TestPluginRunNoPool(t *testing.T) {
	pp := []plugins.Plugin{
		{Name: "cleanup", Formula: "mol-cleanup", Gate: "cron", Schedule: "0 3 * * *"},
	}

	store := beads.NewBdStore(t.TempDir(), func(_, _ string, _ ...string) ([]byte, error) {
		return []byte("WISP-002\n"), nil
	})

	calls := []string{}
	fakeRunner := func(cmd string) (string, error) {
		calls = append(calls, cmd)
		return "", nil
	}

	var stdout, stderr bytes.Buffer
	code := doPluginRun(pp, "cleanup", fakeRunner, store, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doPluginRun = %d, want 0; stderr: %s", code, stderr.String())
	}

	// Plugin with no pool still gets a plugin-run label via bd update.
	if len(calls) != 1 {
		t.Fatalf("got %d runner calls, want 1: %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "--label=plugin-run:cleanup") {
		t.Errorf("call[0] = %q, want --label=plugin-run:cleanup", calls[0])
	}
	// Should NOT contain pool label.
	if strings.Contains(calls[0], "--label=pool:") {
		t.Errorf("call[0] = %q, should not contain pool label", calls[0])
	}
	if !strings.Contains(stdout.String(), "WISP-002") {
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
	store := beads.NewBdStore(t.TempDir(), func(_, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--label=plugin-run:digest") {
			return []byte(`[{"id":"WP-42","title":"digest wisp","status":"closed","issue_type":"task","created_at":"2026-02-27T10:00:00Z","labels":["plugin-run:digest"]}]`), nil
		}
		if strings.Contains(joined, "--label=plugin-run:cleanup") {
			return []byte(`[{"id":"WP-99","title":"cleanup wisp","status":"open","issue_type":"task","created_at":"2026-02-27T11:00:00Z","labels":["plugin-run:cleanup"]}]`), nil
		}
		return []byte(`[]`), nil
	})

	pp := []plugins.Plugin{
		{Name: "digest", Formula: "mol-digest"},
		{Name: "cleanup", Formula: "mol-cleanup"},
	}

	var stdout bytes.Buffer
	code := doPluginHistory("", pp, store, &stdout)
	if code != 0 {
		t.Fatalf("doPluginHistory = %d, want 0", code)
	}
	out := stdout.String()
	// Table header.
	if !strings.Contains(out, "PLUGIN") {
		t.Errorf("stdout missing 'PLUGIN':\n%s", out)
	}
	if !strings.Contains(out, "WISP") {
		t.Errorf("stdout missing 'WISP':\n%s", out)
	}
	// Both plugins should appear.
	if !strings.Contains(out, "digest") {
		t.Errorf("stdout missing 'digest':\n%s", out)
	}
	if !strings.Contains(out, "WP-42") {
		t.Errorf("stdout missing 'WP-42':\n%s", out)
	}
	if !strings.Contains(out, "cleanup") {
		t.Errorf("stdout missing 'cleanup':\n%s", out)
	}
	if !strings.Contains(out, "WP-99") {
		t.Errorf("stdout missing 'WP-99':\n%s", out)
	}
}

func TestPluginHistoryNamed(t *testing.T) {
	store := beads.NewBdStore(t.TempDir(), func(_, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--label=plugin-run:digest") {
			return []byte(`[{"id":"WP-42","title":"digest wisp","status":"closed","issue_type":"task","created_at":"2026-02-27T10:00:00Z","labels":["plugin-run:digest"]}]`), nil
		}
		return []byte(`[]`), nil
	})

	pp := []plugins.Plugin{
		{Name: "digest", Formula: "mol-digest"},
		{Name: "cleanup", Formula: "mol-cleanup"},
	}

	var stdout bytes.Buffer
	code := doPluginHistory("digest", pp, store, &stdout)
	if code != 0 {
		t.Fatalf("doPluginHistory = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "digest") {
		t.Errorf("stdout missing 'digest':\n%s", out)
	}
	if !strings.Contains(out, "WP-42") {
		t.Errorf("stdout missing 'WP-42':\n%s", out)
	}
	// Should NOT contain cleanup (filtered by name).
	if strings.Contains(out, "cleanup") {
		t.Errorf("stdout should not contain 'cleanup':\n%s", out)
	}
}

func TestPluginHistoryEmpty(t *testing.T) {
	store := beads.NewBdStore(t.TempDir(), func(_, _ string, _ ...string) ([]byte, error) {
		return []byte(`[]`), nil
	})

	pp := []plugins.Plugin{
		{Name: "digest", Formula: "mol-digest"},
	}

	var stdout bytes.Buffer
	code := doPluginHistory("", pp, store, &stdout)
	if code != 0 {
		t.Fatalf("doPluginHistory = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "No plugin history") {
		t.Errorf("stdout = %q, want 'No plugin history'", stdout.String())
	}
}
