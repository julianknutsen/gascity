package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

// ---------------------------------------------------------------------------
// doAgentStatus tests
// ---------------------------------------------------------------------------

func TestDoAgentStatus(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	dops := newFakeDrainOps()
	cfgAgent := config.Agent{Name: "worker"}

	var stdout, stderr bytes.Buffer
	code := doAgentStatus(sp, dops, cfgAgent, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Running:    yes") {
		t.Errorf("stdout missing 'Running:    yes', got:\n%s", out)
	}
	if !strings.Contains(out, "Suspended:  no") {
		t.Errorf("stdout missing 'Suspended:  no', got:\n%s", out)
	}
	if !strings.Contains(out, "Draining:   no") {
		t.Errorf("stdout missing 'Draining:   no', got:\n%s", out)
	}
	if !strings.Contains(out, "Session:    gc-city-worker") {
		t.Errorf("stdout missing 'Session:    gc-city-worker', got:\n%s", out)
	}
}

func TestDoAgentStatusStopped(t *testing.T) {
	sp := session.NewFake() // no sessions started
	dops := newFakeDrainOps()
	cfgAgent := config.Agent{Name: "worker"}

	var stdout, stderr bytes.Buffer
	code := doAgentStatus(sp, dops, cfgAgent, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Running:    no") {
		t.Errorf("stdout missing 'Running:    no', got:\n%s", out)
	}
}

func TestDoAgentStatusDraining(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-city-worker", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	dops := newFakeDrainOps()
	dops.draining["gc-city-worker"] = true
	cfgAgent := config.Agent{Name: "worker"}

	var stdout, stderr bytes.Buffer
	code := doAgentStatus(sp, dops, cfgAgent, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Running:    yes") {
		t.Errorf("stdout missing 'Running:    yes', got:\n%s", out)
	}
	if !strings.Contains(out, "Draining:   yes") {
		t.Errorf("stdout missing 'Draining:   yes', got:\n%s", out)
	}
}

func TestDoAgentStatusSuspended(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	cfgAgent := config.Agent{Name: "worker", Suspended: true}

	var stdout, stderr bytes.Buffer
	code := doAgentStatus(sp, dops, cfgAgent, "worker", "gc-city-worker", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Suspended:  yes") {
		t.Errorf("stdout missing 'Suspended:  yes', got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// doRigStatus tests
// ---------------------------------------------------------------------------

func TestDoRigStatus(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-city-frontend--polecat", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	// worker is NOT running.

	dops := newFakeDrainOps()
	rig := config.Rig{Name: "frontend", Path: "/home/user/projects/frontend"}
	agents := []config.Agent{
		{Name: "polecat", Dir: "frontend"},
		{Name: "worker", Dir: "frontend"},
	}

	var stdout, stderr bytes.Buffer
	code := doRigStatus(sp, dops, rig, agents, "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()

	// Rig header.
	if !strings.Contains(out, "frontend:") {
		t.Errorf("stdout missing 'frontend:', got:\n%s", out)
	}
	if !strings.Contains(out, "Path:       /home/user/projects/frontend") {
		t.Errorf("stdout missing path, got:\n%s", out)
	}
	if !strings.Contains(out, "Suspended:  no") {
		t.Errorf("stdout missing 'Suspended:  no', got:\n%s", out)
	}

	// Agent status lines.
	if !strings.Contains(out, "polecat") && !strings.Contains(out, "running") {
		t.Errorf("stdout missing polecat running status, got:\n%s", out)
	}
	if !strings.Contains(out, "worker") && !strings.Contains(out, "stopped") {
		t.Errorf("stdout missing worker stopped status, got:\n%s", out)
	}
}

func TestDoRigStatusSuspendedRig(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	rig := config.Rig{Name: "frontend", Path: "/tmp/frontend", Suspended: true}
	agents := []config.Agent{
		{Name: "polecat", Dir: "frontend", Suspended: true},
	}

	var stdout, stderr bytes.Buffer
	code := doRigStatus(sp, dops, rig, agents, "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Suspended:  yes") {
		t.Errorf("stdout missing 'Suspended:  yes', got:\n%s", out)
	}
}

func TestDoRigStatusWithDraining(t *testing.T) {
	sp := session.NewFake()
	if err := sp.Start("gc-city-frontend--worker-1", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	dops := newFakeDrainOps()
	dops.draining["gc-city-frontend--worker-1"] = true

	rig := config.Rig{Name: "frontend", Path: "/tmp/frontend"}
	agents := []config.Agent{
		{Name: "worker", Dir: "frontend", Pool: &config.PoolConfig{Min: 1, Max: 2, Check: "echo 1"}},
	}

	var stdout, stderr bytes.Buffer
	code := doRigStatus(sp, dops, rig, agents, "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "running  (draining)") {
		t.Errorf("stdout missing 'running  (draining)', got:\n%s", out)
	}
	if !strings.Contains(out, "stopped") {
		t.Errorf("stdout missing 'stopped' for worker-2, got:\n%s", out)
	}
}

func TestDoRigStatusSuspendedAgent(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	rig := config.Rig{Name: "frontend", Path: "/tmp/frontend"}
	agents := []config.Agent{
		{Name: "worker", Dir: "frontend", Suspended: true},
	}

	var stdout, stderr bytes.Buffer
	code := doRigStatus(sp, dops, rig, agents, "city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "stopped  (suspended)") {
		t.Errorf("stdout missing 'stopped  (suspended)', got:\n%s", out)
	}
}
