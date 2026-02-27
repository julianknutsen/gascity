package plugins

import (
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestScan(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/plugins/digest"] = true
	fs.Files["/layer1/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	fs.Dirs["/layer1/plugins/cleanup"] = true
	fs.Files["/layer1/plugins/cleanup/plugin.toml"] = []byte(`
[plugin]
formula = "mol-cleanup"
gate = "cron"
schedule = "0 3 * * *"
`)

	plugins, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("got %d plugins, want 2", len(plugins))
	}
	// Names should be set from directory names.
	names := map[string]bool{}
	for _, p := range plugins {
		names[p.Name] = true
	}
	if !names["digest"] || !names["cleanup"] {
		t.Errorf("expected digest and cleanup, got %v", names)
	}
}

func TestScanEmpty(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1"] = true

	plugins, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("got %d plugins, want 0", len(plugins))
	}
}

func TestScanLayerOverride(t *testing.T) {
	fs := fsys.NewFake()
	// Layer 1 (lower priority): digest with 24h.
	fs.Dirs["/layer1/plugins/digest"] = true
	fs.Files["/layer1/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	// Layer 2 (higher priority): digest with 8h.
	fs.Dirs["/layer2/plugins/digest"] = true
	fs.Files["/layer2/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "cooldown"
interval = "8h"
pool = "dog"
`)

	plugins, err := Scan(fs, []string{"/layer1", "/layer2"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}
	if plugins[0].Interval != "8h" {
		t.Errorf("Interval = %q, want %q (layer 2 overrides)", plugins[0].Interval, "8h")
	}
}

func TestScanSkip(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/plugins/digest"] = true
	fs.Files["/layer1/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
`)
	fs.Dirs["/layer1/plugins/cleanup"] = true
	fs.Files["/layer1/plugins/cleanup/plugin.toml"] = []byte(`
[plugin]
formula = "mol-cleanup"
gate = "manual"
`)

	plugins, err := Scan(fs, []string{"/layer1"}, []string{"digest"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}
	if plugins[0].Name != "cleanup" {
		t.Errorf("Name = %q, want %q", plugins[0].Name, "cleanup")
	}
}

func TestScanDisabled(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/plugins/digest"] = true
	fs.Files["/layer1/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
enabled = false
`)

	plugins, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("got %d plugins, want 0 (disabled)", len(plugins))
	}
}

func TestScanSourcePath(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/plugins/digest"] = true
	fs.Files["/layer1/plugins/digest/plugin.toml"] = []byte(`
[plugin]
formula = "mol-digest"
gate = "manual"
`)

	plugins, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}
	if plugins[0].Source != "/layer1/plugins/digest/plugin.toml" {
		t.Errorf("Source = %q, want %q", plugins[0].Source, "/layer1/plugins/digest/plugin.toml")
	}
}
