package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

func TestCityStatusEmptyCity(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "bright-lights"},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/home/user/bright-lights", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "bright-lights") {
		t.Errorf("stdout missing city name, got:\n%s", out)
	}
	if !strings.Contains(out, "/home/user/bright-lights") {
		t.Errorf("stdout missing city path, got:\n%s", out)
	}
	if !strings.Contains(out, "Controller: stopped") {
		t.Errorf("stdout missing controller status, got:\n%s", out)
	}
	if !strings.Contains(out, "Suspended:  no") {
		t.Errorf("stdout missing 'Suspended:  no', got:\n%s", out)
	}
	// No agents section when there are no agents.
	if strings.Contains(out, "Agents:") {
		t.Errorf("stdout should not have Agents section for empty city, got:\n%s", out)
	}
}

func TestCityStatusWithAgents(t *testing.T) {
	sp := session.NewFake()
	// Start one agent session.
	if err := sp.Start("gc-city-mayor", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	dops := newFakeDrainOps()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
		},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/home/user/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()

	if !strings.Contains(out, "Agents:") {
		t.Errorf("stdout missing 'Agents:', got:\n%s", out)
	}
	if !strings.Contains(out, "mayor") {
		t.Errorf("stdout missing 'mayor', got:\n%s", out)
	}
	if !strings.Contains(out, "worker") {
		t.Errorf("stdout missing 'worker', got:\n%s", out)
	}
	if !strings.Contains(out, "1/2 agents running") {
		t.Errorf("stdout missing '1/2 agents running', got:\n%s", out)
	}
}

func TestCityStatusSuspended(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city", Suspended: true},
		Agents:    []config.Agent{{Name: "mayor"}},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/tmp/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Suspended:  yes") {
		t.Errorf("stdout missing 'Suspended:  yes', got:\n%s", out)
	}
}

func TestCityStatusPoolExpansion(t *testing.T) {
	sp := session.NewFake()
	// Start 2 of 3 pool instances.
	if err := sp.Start("gc-city-hw--polecat-1", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if err := sp.Start("gc-city-hw--polecat-2", session.Config{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	dops := newFakeDrainOps()
	dops.draining["gc-city-hw--polecat-2"] = true

	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Agents: []config.Agent{
			{Name: "polecat", Dir: "hw", Pool: &config.PoolConfig{Min: 1, Max: 3, Check: "echo 1"}},
		},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/tmp/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()

	// Pool header line.
	if !strings.Contains(out, "pool (min=1, max=3)") {
		t.Errorf("stdout missing pool header, got:\n%s", out)
	}
	// Instance lines.
	if !strings.Contains(out, "polecat-1") {
		t.Errorf("stdout missing polecat-1, got:\n%s", out)
	}
	if !strings.Contains(out, "polecat-2") {
		t.Errorf("stdout missing polecat-2, got:\n%s", out)
	}
	if !strings.Contains(out, "polecat-3") {
		t.Errorf("stdout missing polecat-3, got:\n%s", out)
	}
	// polecat-2 draining.
	if !strings.Contains(out, "running  (draining)") {
		t.Errorf("stdout missing 'running  (draining)', got:\n%s", out)
	}
	// Summary: 2/3 running.
	if !strings.Contains(out, "2/3 agents running") {
		t.Errorf("stdout missing '2/3 agents running', got:\n%s", out)
	}
}

func TestCityStatusRigs(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Agents:    []config.Agent{{Name: "mayor"}},
		Rigs: []config.Rig{
			{Name: "hello-world", Path: "/home/user/hello-world"},
			{Name: "frontend", Path: "/home/user/frontend", Suspended: true},
		},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/tmp/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Rigs:") {
		t.Errorf("stdout missing 'Rigs:', got:\n%s", out)
	}
	if !strings.Contains(out, "hello-world") {
		t.Errorf("stdout missing 'hello-world', got:\n%s", out)
	}
	if !strings.Contains(out, "/home/user/hello-world") {
		t.Errorf("stdout missing hello-world path, got:\n%s", out)
	}
	if !strings.Contains(out, "frontend") {
		t.Errorf("stdout missing 'frontend', got:\n%s", out)
	}
	if !strings.Contains(out, "(suspended)") {
		t.Errorf("stdout missing '(suspended)' for frontend, got:\n%s", out)
	}
}

func TestCityStatusAgentSuspendedByRig(t *testing.T) {
	sp := session.NewFake()
	dops := newFakeDrainOps()
	cfg := &config.City{
		Workspace: config.Workspace{Name: "city"},
		Agents: []config.Agent{
			{Name: "polecat", Dir: "myrig"},
		},
		Rigs: []config.Rig{
			{Name: "myrig", Path: "/tmp/myrig", Suspended: true},
		},
	}

	var stdout, stderr bytes.Buffer
	code := doCityStatus(sp, dops, cfg, "/tmp/city", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	out := stdout.String()
	// Agent in suspended rig should show "stopped  (suspended)".
	if !strings.Contains(out, "stopped  (suspended)") {
		t.Errorf("stdout missing 'stopped  (suspended)' for rig-suspended agent, got:\n%s", out)
	}
}
