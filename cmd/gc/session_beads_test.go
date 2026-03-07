package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/runtime"
)

func TestSyncSessionBeads_CreatesNewBeads(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	_ = sp.Start(context.Background(), "mayor", runtime.Config{Command: "claude"})
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "mayor",
			FakeSessionName:   "mayor",
			Running:           true,
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer
	syncSessionBeads(store, agents, sp, clk, &stderr)

	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	all, err := store.ListByLabel(sessionBeadLabel, 0)
	if err != nil {
		t.Fatalf("listing beads: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(all))
	}

	b := all[0]
	if b.Type != sessionBeadType {
		t.Errorf("type = %q, want %q", b.Type, sessionBeadType)
	}
	if b.Metadata["session_name"] != "mayor" {
		t.Errorf("session_name = %q, want %q", b.Metadata["session_name"], "mayor")
	}
	if b.Metadata["state"] != "active" {
		t.Errorf("state = %q, want %q", b.Metadata["state"], "active")
	}
	if b.Metadata["generation"] != "1" {
		t.Errorf("generation = %q, want %q", b.Metadata["generation"], "1")
	}
	if b.Metadata["instance_token"] == "" {
		t.Error("instance_token is empty")
	}
	if b.Metadata["config_hash"] == "" {
		t.Error("config_hash is empty")
	}
}

func TestSyncSessionBeads_Idempotent(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	_ = sp.Start(context.Background(), "mayor", runtime.Config{Command: "claude"})
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "mayor",
			FakeSessionName:   "mayor",
			Running:           true,
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer
	syncSessionBeads(store, agents, sp, clk, &stderr)

	// Get the created bead's token and generation.
	all, _ := store.ListByLabel(sessionBeadLabel, 0)
	token1 := all[0].Metadata["instance_token"]
	gen1 := all[0].Metadata["generation"]

	// Run again — should be idempotent.
	clk.Advance(5 * time.Second)
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ = store.ListByLabel(sessionBeadLabel, 0)
	if len(all) != 1 {
		t.Fatalf("expected 1 bead after re-sync, got %d", len(all))
	}

	// Token and generation should NOT change when config is unchanged.
	if all[0].Metadata["instance_token"] != token1 {
		t.Error("instance_token changed on idempotent re-sync")
	}
	if all[0].Metadata["generation"] != gen1 {
		t.Error("generation changed on idempotent re-sync")
	}
}

func TestSyncSessionBeads_ConfigDrift(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	_ = sp.Start(context.Background(), "mayor", runtime.Config{Command: "claude"})
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "mayor",
			FakeSessionName:   "mayor",
			Running:           true,
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ := store.ListByLabel(sessionBeadLabel, 0)
	token1 := all[0].Metadata["instance_token"]

	// Change config — different command.
	agents[0].(*agent.Fake).FakeSessionConfig = runtime.Config{Command: "gemini"}
	clk.Advance(5 * time.Second)
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ = store.ListByLabel(sessionBeadLabel, 0)
	if all[0].Metadata["generation"] != "2" {
		t.Errorf("generation = %q, want %q after config drift", all[0].Metadata["generation"], "2")
	}
	if all[0].Metadata["instance_token"] == token1 {
		t.Error("instance_token should change on config drift")
	}
}

func TestSyncSessionBeads_OrphanDetection(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	// Create a bead for "old-agent".
	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "old-agent",
			FakeSessionName:   "old-agent",
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer
	syncSessionBeads(store, agents, sp, clk, &stderr)

	// Now sync with a different agent list (old-agent removed).
	agents = []agent.Agent{
		&agent.Fake{
			FakeName:          "new-agent",
			FakeSessionName:   "new-agent",
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}
	clk.Advance(5 * time.Second)
	syncSessionBeads(store, agents, sp, clk, &stderr)

	// old-agent's bead should be marked orphaned.
	all, _ := store.ListByLabel(sessionBeadLabel, 0)
	var oldBead beads.Bead
	for _, b := range all {
		if b.Metadata["session_name"] == "old-agent" {
			oldBead = b
			break
		}
	}
	if oldBead.Metadata["state"] != "orphaned" {
		t.Errorf("old-agent state = %q, want %q", oldBead.Metadata["state"], "orphaned")
	}
}

func TestSyncSessionBeads_NilStore(t *testing.T) {
	// Verify nil store does not panic.
	var stderr bytes.Buffer
	syncSessionBeads(nil, nil, nil, &clock.Fake{}, &stderr)
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestSyncSessionBeads_StoppedAgent(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	// NOT running.
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "mayor",
			FakeSessionName:   "mayor",
			Running:           false,
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ := store.ListByLabel(sessionBeadLabel, 0)
	if len(all) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(all))
	}
	if all[0].Metadata["state"] != "stopped" {
		t.Errorf("state = %q, want %q", all[0].Metadata["state"], "stopped")
	}
}

func TestSyncSessionBeads_ClosedBeadCreatesNew(t *testing.T) {
	store := beads.NewMemStore()
	sp := runtime.NewFake()
	_ = sp.Start(context.Background(), "mayor", runtime.Config{Command: "claude"})
	clk := &clock.Fake{Time: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}

	agents := []agent.Agent{
		&agent.Fake{
			FakeName:          "mayor",
			FakeSessionName:   "mayor",
			Running:           true,
			FakeSessionConfig: runtime.Config{Command: "claude"},
		},
	}

	var stderr bytes.Buffer

	// First sync creates the bead.
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ := store.ListByLabel(sessionBeadLabel, 0)
	if len(all) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(all))
	}

	// Close the bead to simulate a completed lifecycle.
	_ = store.Close(all[0].ID)

	// Re-sync should create a NEW bead, not reuse the closed one.
	clk.Advance(5 * time.Second)
	syncSessionBeads(store, agents, sp, clk, &stderr)

	all, _ = store.ListByLabel(sessionBeadLabel, 0)
	if len(all) != 2 {
		t.Fatalf("expected 2 beads (1 closed + 1 new), got %d", len(all))
	}

	// Find the open bead.
	var openBead beads.Bead
	for _, b := range all {
		if b.Status == "open" {
			openBead = b
			break
		}
	}
	if openBead.ID == "" {
		t.Fatal("no open bead found after re-sync")
	}
	if openBead.Metadata["state"] != "active" {
		t.Errorf("state = %q, want %q", openBead.Metadata["state"], "active")
	}
	if openBead.Metadata["generation"] != "1" {
		t.Errorf("generation = %q, want %q (fresh bead)", openBead.Metadata["generation"], "1")
	}
}
