package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
)

func TestWispGC_NilSafe(t *testing.T) {
	// A nil wispGC must be safe to nil-guard (same pattern as crashTracker).
	var wg wispGC
	if wg != nil {
		t.Error("nil wispGC should be nil")
	}
}

func TestWispGC_DisabledReturnsNil(t *testing.T) {
	// interval=0 or ttl=0 → disabled → nil.
	wg := newWispGC(0, time.Hour, nil)
	if wg != nil {
		t.Error("zero interval should return nil")
	}
	wg = newWispGC(time.Hour, 0, nil)
	if wg != nil {
		t.Error("zero TTL should return nil")
	}
}

func TestWispGC_ShouldRunRespectsInterval(t *testing.T) {
	wg := newWispGC(5*time.Minute, time.Hour, nil)
	now := time.Now()

	// First call: should run (never run before).
	if !wg.shouldRun(now) {
		t.Error("should run on first call")
	}

	// Mark as run.
	wg.(*memoryWispGC).lastRun = now

	// Too soon.
	if wg.shouldRun(now.Add(time.Minute)) {
		t.Error("should not run before interval elapsed")
	}

	// After interval.
	if !wg.shouldRun(now.Add(6 * time.Minute)) {
		t.Error("should run after interval elapsed")
	}
}

func TestWispGC_PurgesExpiredMolecules(t *testing.T) {
	now := time.Now()
	ttl := time.Hour
	runner := &fakeGCRunner{
		listOutput: makeMoleculeList([]fakeMol{
			{ID: "mol-1", CreatedAt: now.Add(-2 * time.Hour), Status: "closed", Type: "molecule"},
			{ID: "mol-2", CreatedAt: now.Add(-30 * time.Minute), Status: "closed", Type: "molecule"},
			{ID: "mol-3", CreatedAt: now.Add(-3 * time.Hour), Status: "closed", Type: "molecule"},
		}),
	}

	wg := newWispGC(5*time.Minute, ttl, runner.run)
	purged, err := wg.runGC("/city", now)
	if err != nil {
		t.Fatalf("runGC: %v", err)
	}

	// mol-1 and mol-3 expired (>1h old), mol-2 not yet.
	if purged != 2 {
		t.Errorf("purged = %d, want 2", purged)
	}
	if len(runner.deletedIDs) != 2 {
		t.Fatalf("deleted = %v, want 2 entries", runner.deletedIDs)
	}
	// Verify correct IDs deleted.
	deleted := map[string]bool{}
	for _, id := range runner.deletedIDs {
		deleted[id] = true
	}
	if !deleted["mol-1"] || !deleted["mol-3"] {
		t.Errorf("deleted = %v, want mol-1 and mol-3", runner.deletedIDs)
	}
}

func TestWispGC_NothingExpired(t *testing.T) {
	now := time.Now()
	runner := &fakeGCRunner{
		listOutput: makeMoleculeList([]fakeMol{
			{ID: "mol-1", CreatedAt: now.Add(-10 * time.Minute), Status: "closed", Type: "molecule"},
		}),
	}

	wg := newWispGC(5*time.Minute, time.Hour, runner.run)
	purged, err := wg.runGC("/city", now)
	if err != nil {
		t.Fatalf("runGC: %v", err)
	}
	if purged != 0 {
		t.Errorf("purged = %d, want 0", purged)
	}
	if len(runner.deletedIDs) != 0 {
		t.Errorf("should not have deleted anything, got %v", runner.deletedIDs)
	}
}

func TestWispGC_EmptyList(t *testing.T) {
	runner := &fakeGCRunner{
		listOutput: []byte("[]\n"),
	}

	wg := newWispGC(5*time.Minute, time.Hour, runner.run)
	purged, err := wg.runGC("/city", time.Now())
	if err != nil {
		t.Fatalf("runGC: %v", err)
	}
	if purged != 0 {
		t.Errorf("purged = %d, want 0", purged)
	}
}

func TestWispGC_DeleteErrorContinues(t *testing.T) {
	now := time.Now()
	runner := &fakeGCRunner{
		listOutput: makeMoleculeList([]fakeMol{
			{ID: "mol-1", CreatedAt: now.Add(-2 * time.Hour), Status: "closed", Type: "molecule"},
			{ID: "mol-2", CreatedAt: now.Add(-2 * time.Hour), Status: "closed", Type: "molecule"},
		}),
		deleteErrors: map[string]error{
			"mol-1": fmt.Errorf("delete failed"),
		},
	}

	wg := newWispGC(5*time.Minute, time.Hour, runner.run)
	purged, err := wg.runGC("/city", now)
	// Should return nil (best-effort) even though one delete failed.
	if err != nil {
		t.Fatalf("runGC: %v", err)
	}
	// Only mol-2 was successfully purged.
	if purged != 1 {
		t.Errorf("purged = %d, want 1 (one delete failed)", purged)
	}
}

// --- test helpers ---

type fakeMol struct {
	ID        string
	CreatedAt time.Time
	Status    string
	Type      string
}

func makeMoleculeList(mols []fakeMol) []byte {
	type entry struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		Status    string `json:"status"`
		Type      string `json:"type"`
	}
	var entries []entry
	for _, m := range mols {
		entries = append(entries, entry{
			ID:        m.ID,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
			Status:    m.Status,
			Type:      m.Type,
		})
	}
	data, _ := json.Marshal(entries)
	return data
}

type fakeGCRunner struct {
	listOutput   []byte
	deleteErrors map[string]error
	deletedIDs   []string
}

func (f *fakeGCRunner) run(_, name string, args ...string) ([]byte, error) {
	// Detect whether this is a "list" or "delete" call.
	cmdLine := strings.Join(append([]string{name}, args...), " ")
	if strings.Contains(cmdLine, "list") {
		return f.listOutput, nil
	}
	if strings.Contains(cmdLine, "delete") {
		// Extract ID: last arg before --force, or second-to-last.
		id := ""
		for _, a := range args {
			if a != "--force" && a != "delete" {
				id = a
			}
		}
		if f.deleteErrors != nil {
			if err, ok := f.deleteErrors[id]; ok {
				return nil, err
			}
		}
		f.deletedIDs = append(f.deletedIDs, id)
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command: %s", cmdLine)
}

// Verify fakeGCRunner satisfies beads.CommandRunner type.
var _ beads.CommandRunner = (&fakeGCRunner{}).run
