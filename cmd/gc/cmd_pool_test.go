package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/session"
)

func TestPoolStatusNoPools(t *testing.T) {
	var stdout bytes.Buffer
	sp := session.NewFake()
	doPoolStatus(nil, "city", sp, &stdout)
	if !strings.Contains(stdout.String(), "No pools configured.") {
		t.Errorf("output = %q, want 'No pools configured.'", stdout.String())
	}
}

func TestPoolStatusWithRunning(t *testing.T) {
	sp := session.NewFake()
	// Start some fake sessions.
	_ = sp.Start("gc-city-worker-1", session.Config{Command: "echo"})
	_ = sp.Start("gc-city-worker-2", session.Config{Command: "echo"})

	pools := []config.Pool{{
		Name:       "worker",
		Min:        0,
		Max:        5,
		ScaleCheck: "echo 2",
	}}
	var stdout bytes.Buffer
	doPoolStatus(pools, "city", sp, &stdout)

	out := stdout.String()
	if !strings.Contains(out, "Pool 'worker'") {
		t.Errorf("output missing pool header: %q", out)
	}
	if !strings.Contains(out, "worker-1: running") {
		t.Errorf("output missing worker-1: %q", out)
	}
	if !strings.Contains(out, "worker-2: running") {
		t.Errorf("output missing worker-2: %q", out)
	}
	if !strings.Contains(out, "total: 2 running") {
		t.Errorf("output missing total: %q", out)
	}
}

func TestPoolStatusNoRunning(t *testing.T) {
	sp := session.NewFake()
	pools := []config.Pool{{
		Name:       "worker",
		Min:        0,
		Max:        5,
		ScaleCheck: "echo 0",
	}}
	var stdout bytes.Buffer
	doPoolStatus(pools, "city", sp, &stdout)

	out := stdout.String()
	if !strings.Contains(out, "Pool 'worker'") {
		t.Errorf("output missing pool header: %q", out)
	}
	if !strings.Contains(out, "(no agents running)") {
		t.Errorf("output missing '(no agents running)': %q", out)
	}
}
