package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

// fakeAdoptionProvider implements runtime.Provider for adoption barrier tests.
type fakeAdoptionProvider struct {
	runtime.Provider
	running []string
}

func (f *fakeAdoptionProvider) ListRunning(_ string) ([]string, error) {
	return f.running, nil
}

func TestAdoptionBarrier_NoRunning(t *testing.T) {
	store := beads.NewMemStore()
	sp := &fakeAdoptionProvider{running: nil}
	cfg := &config.City{}
	var stderr bytes.Buffer

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if !passed {
		t.Error("barrier should pass with no running sessions")
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
}

func TestAdoptionBarrier_AdoptsRunning(t *testing.T) {
	store := beads.NewMemStore()
	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor", "test-city-worker"}}
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
		},
	}
	var stderr bytes.Buffer
	clk := &clock.Fake{Time: time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)}

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clk, &stderr, false)
	if !passed {
		t.Errorf("barrier should pass, stderr: %s", stderr.String())
	}
	if result.Adopted != 2 {
		t.Errorf("Adopted = %d, want 2", result.Adopted)
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}

	// Verify beads were created.
	beadList, _ := store.ListByLabel(sessionBeadLabel, 0)
	if len(beadList) != 2 {
		t.Errorf("beads count = %d, want 2", len(beadList))
	}
}

func TestAdoptionBarrier_SkipsExistingBead(t *testing.T) {
	store := beads.NewMemStore()
	// Pre-create a bead for mayor.
	_, err := store.Create(beads.Bead{
		Title:  "mayor",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"session_name": "test-city-mayor",
			"state":        "active",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor", "test-city-worker"}}
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
		},
	}
	var stderr bytes.Buffer

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if !passed {
		t.Error("barrier should pass")
	}
	if result.Adopted != 1 {
		t.Errorf("Adopted = %d, want 1", result.Adopted)
	}
	if result.AlreadyHadBead != 1 {
		t.Errorf("AlreadyHadBead = %d, want 1", result.AlreadyHadBead)
	}
}

func TestAdoptionBarrier_ClosedBeadDoesNotBlock(t *testing.T) {
	store := beads.NewMemStore()
	// Pre-create and close a bead for mayor.
	b, err := store.Create(beads.Bead{
		Title:  "mayor",
		Type:   sessionBeadType,
		Labels: []string{sessionBeadLabel},
		Metadata: map[string]string{
			"session_name": "test-city-mayor",
			"state":        "active",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(b.ID); err != nil {
		t.Fatal(err)
	}

	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor"}}
	cfg := &config.City{Agents: []config.Agent{{Name: "mayor"}}}
	var stderr bytes.Buffer

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if !passed {
		t.Error("barrier should pass")
	}
	if result.Adopted != 1 {
		t.Errorf("Adopted = %d, want 1 (closed bead should not prevent adoption)", result.Adopted)
	}
}

func TestAdoptionBarrier_Rerunnable(t *testing.T) {
	store := beads.NewMemStore()
	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor"}}
	cfg := &config.City{Agents: []config.Agent{{Name: "mayor"}}}
	var stderr bytes.Buffer

	// First run: adopts.
	r1, _ := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if r1.Adopted != 1 {
		t.Fatalf("first run Adopted = %d, want 1", r1.Adopted)
	}

	// Second run: dedup prevents duplicates.
	r2, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if !passed {
		t.Error("second run: barrier should pass")
	}
	if r2.Adopted != 0 {
		t.Errorf("second run Adopted = %d, want 0", r2.Adopted)
	}
	if r2.AlreadyHadBead != 1 {
		t.Errorf("second run AlreadyHadBead = %d, want 1", r2.AlreadyHadBead)
	}
}

func TestAdoptionBarrier_DryRun(t *testing.T) {
	store := beads.NewMemStore()
	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor", "test-city-worker"}}
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "mayor"},
			{Name: "worker"},
		},
	}
	var stderr bytes.Buffer

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, true)
	if !passed {
		t.Error("dry run barrier should pass")
	}
	if result.Adopted != 2 {
		t.Errorf("Adopted = %d, want 2", result.Adopted)
	}

	// Verify no beads were actually created.
	beadList, _ := store.ListByLabel(sessionBeadLabel, 0)
	if len(beadList) != 0 {
		t.Errorf("dry run created %d beads, want 0", len(beadList))
	}
}

func TestAdoptionBarrier_NilStore(t *testing.T) {
	sp := &fakeAdoptionProvider{running: []string{"test-city-mayor"}}
	cfg := &config.City{}
	var stderr bytes.Buffer

	_, passed := runAdoptionBarrier(nil, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if passed {
		t.Error("nil store: barrier should not pass")
	}
}

func TestAdoptionBarrier_PoolSlotDetection(t *testing.T) {
	store := beads.NewMemStore()
	sp := &fakeAdoptionProvider{running: []string{"test-city-worker-3"}}
	cfg := &config.City{
		Agents: []config.Agent{
			{Name: "worker", Pool: &config.PoolConfig{Min: 1, Max: 5}},
		},
	}
	var stderr bytes.Buffer

	result, _ := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, true)
	// The session name test-city-worker-3 won't match config agent "worker"
	// (whose session name is test-city-worker), so it's adopted as unknown.
	// But pool slot -3 should still be parsed from the suffix.
	found := false
	for _, d := range result.Details {
		if d.SessionName == "test-city-worker-3" && d.PoolSlot == 3 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail with PoolSlot=3 for test-city-worker-3, got %+v", result.Details)
	}
}

func TestParsePoolSlot(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"s-worker-3", 3},
		{"s-worker-10", 10},
		{"s-mayor", 0},
		{"worker", 0},
	}
	for _, tt := range tests {
		got := parsePoolSlot(tt.name)
		if got != tt.want {
			t.Errorf("parsePoolSlot(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestAdoptionBarrier_UnknownSession(t *testing.T) {
	store := beads.NewMemStore()
	// Running session that doesn't match any config agent.
	sp := &fakeAdoptionProvider{running: []string{"unknown-session"}}
	cfg := &config.City{} // no agents configured
	var stderr bytes.Buffer

	result, passed := runAdoptionBarrier(store, sp, cfg, "test-city", clock.Real{}, &stderr, false)
	if !passed {
		t.Error("barrier should pass (adopt permissively)")
	}
	if result.Adopted != 1 {
		t.Errorf("Adopted = %d, want 1", result.Adopted)
	}
}
