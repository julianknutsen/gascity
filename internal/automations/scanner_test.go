package automations

import (
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestScan(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/automations/digest"] = true
	fs.Files["/layer1/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	fs.Dirs["/layer1/automations/cleanup"] = true
	fs.Files["/layer1/automations/cleanup/automation.toml"] = []byte(`
[automation]
formula = "mol-cleanup"
gate = "cron"
schedule = "0 3 * * *"
`)

	automations, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 2 {
		t.Fatalf("got %d automations, want 2", len(automations))
	}
	// Names should be set from directory names.
	names := map[string]bool{}
	for _, a := range automations {
		names[a.Name] = true
	}
	if !names["digest"] || !names["cleanup"] {
		t.Errorf("expected digest and cleanup, got %v", names)
	}
}

func TestScanEmpty(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1"] = true

	automations, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 0 {
		t.Fatalf("got %d automations, want 0", len(automations))
	}
}

func TestScanLayerOverride(t *testing.T) {
	fs := fsys.NewFake()
	// Layer 1 (lower priority): digest with 24h.
	fs.Dirs["/layer1/automations/digest"] = true
	fs.Files["/layer1/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
pool = "dog"
`)
	// Layer 2 (higher priority): digest with 8h.
	fs.Dirs["/layer2/automations/digest"] = true
	fs.Files["/layer2/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "cooldown"
interval = "8h"
pool = "dog"
`)

	automations, err := Scan(fs, []string{"/layer1", "/layer2"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("got %d automations, want 1", len(automations))
	}
	if automations[0].Interval != "8h" {
		t.Errorf("Interval = %q, want %q (layer 2 overrides)", automations[0].Interval, "8h")
	}
}

func TestScanSkip(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/automations/digest"] = true
	fs.Files["/layer1/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
`)
	fs.Dirs["/layer1/automations/cleanup"] = true
	fs.Files["/layer1/automations/cleanup/automation.toml"] = []byte(`
[automation]
formula = "mol-cleanup"
gate = "manual"
`)

	automations, err := Scan(fs, []string{"/layer1"}, []string{"digest"})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("got %d automations, want 1", len(automations))
	}
	if automations[0].Name != "cleanup" {
		t.Errorf("Name = %q, want %q", automations[0].Name, "cleanup")
	}
}

func TestScanDisabled(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/automations/digest"] = true
	fs.Files["/layer1/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "cooldown"
interval = "24h"
enabled = false
`)

	automations, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 0 {
		t.Fatalf("got %d automations, want 0 (disabled)", len(automations))
	}
}

func TestScanSourcePath(t *testing.T) {
	fs := fsys.NewFake()
	fs.Dirs["/layer1/automations/digest"] = true
	fs.Files["/layer1/automations/digest/automation.toml"] = []byte(`
[automation]
formula = "mol-digest"
gate = "manual"
`)

	automations, err := Scan(fs, []string{"/layer1"}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("got %d automations, want 1", len(automations))
	}
	if automations[0].Source != "/layer1/automations/digest/automation.toml" {
		t.Errorf("Source = %q, want %q", automations[0].Source, "/layer1/automations/digest/automation.toml")
	}
}
