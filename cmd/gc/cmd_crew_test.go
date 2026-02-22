package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// --- doCrewAdd ---

func TestDoCrewAddSuccess(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doCrewAdd(f, "/city", "worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doCrewAdd = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Added crew member 'worker'") {
		t.Errorf("stdout = %q, want 'Added crew member'", stdout.String())
	}

	// Verify the written config has both agents.
	written := f.Files[filepath.Join("/city", "city.toml")]
	got, err := config.Parse(written)
	if err != nil {
		t.Fatalf("parsing written config: %v", err)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(got.Agents))
	}
	if got.Agents[0].Name != "mayor" {
		t.Errorf("Agents[0].Name = %q, want %q", got.Agents[0].Name, "mayor")
	}
	if got.Agents[1].Name != "worker" {
		t.Errorf("Agents[1].Name = %q, want %q", got.Agents[1].Name, "worker")
	}
}

func TestDoCrewAddDuplicate(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stderr bytes.Buffer
	code := doCrewAdd(f, "/city", "mayor", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doCrewAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("stderr = %q, want 'already exists'", stderr.String())
	}
}

func TestDoCrewAddLoadFails(t *testing.T) {
	f := fsys.NewFake()
	// No city.toml → load fails.

	var stderr bytes.Buffer
	code := doCrewAdd(f, "/city", "worker", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doCrewAdd = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc crew add") {
		t.Errorf("stderr = %q, want 'gc crew add' prefix", stderr.String())
	}
}

// --- doCrewList ---

func TestDoCrewListSuccess(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	cfg.Agents = append(cfg.Agents, config.Agent{Name: "worker"})
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doCrewList(f, "/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doCrewList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bright-lights:") {
		t.Errorf("stdout missing 'bright-lights:': %q", out)
	}
	if !strings.Contains(out, "  mayor") {
		t.Errorf("stdout missing '  mayor': %q", out)
	}
	if !strings.Contains(out, "  worker") {
		t.Errorf("stdout missing '  worker': %q", out)
	}
}

func TestDoCrewListSingleAgent(t *testing.T) {
	f := fsys.NewFake()
	cfg := config.DefaultCity("bright-lights")
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	f.Files[filepath.Join("/city", "city.toml")] = data

	var stdout, stderr bytes.Buffer
	code := doCrewList(f, "/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doCrewList = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "bright-lights:") {
		t.Errorf("stdout missing 'bright-lights:': %q", out)
	}
	if !strings.Contains(out, "  mayor") {
		t.Errorf("stdout missing '  mayor': %q", out)
	}
}

func TestDoCrewListLoadFails(t *testing.T) {
	f := fsys.NewFake()
	// No city.toml → load fails.

	var stderr bytes.Buffer
	code := doCrewList(f, "/city", &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("doCrewList = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "gc crew list") {
		t.Errorf("stderr = %q, want 'gc crew list' prefix", stderr.String())
	}
}
