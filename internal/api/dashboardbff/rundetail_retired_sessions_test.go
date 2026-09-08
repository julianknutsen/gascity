package dashboardbff

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/runproj"
)

// retiredSessionID mints this city's session-bead id shape (gcg-session-<32hex>)
// from a small integer, so fixtures can reference many distinct closed seats.
func retiredSessionID(n int) string {
	return fmt.Sprintf("gcg-session-%032x", n)
}

// sessionsFake is a stateful stand-in for the supervisor's session reads: the
// OPEN-only listing at /v0/city/{name}/sessions and the by-id read at
// /v0/city/{name}/session/{id}, which resolves closed beads too. It counts the
// per-id reads so a test can prove the enrichment is bounded by the run's own
// links, and records every listing query so a test can prove the periodic poll
// never asks for closed beads.
type sessionsFake struct {
	mu          sync.Mutex
	open        []map[string]any
	closed      map[string]map[string]any
	listQueries []string
	byIDHits    map[string]int
	listStatus  int
}

func newSessionsFake() *sessionsFake {
	return &sessionsFake{closed: map[string]map[string]any{}, byIDHits: map[string]int{}, listStatus: http.StatusOK}
}

func (f *sessionsFake) addOpen(id, alias string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open = append(f.open, map[string]any{"id": id, "alias": alias, "state": "active", "running": true})
}

func (f *sessionsFake) addClosed(id, alias string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed[id] = map[string]any{"id": id, "alias": alias, "state": "", "running": false}
}

func (f *sessionsFake) hits(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byIDHits[id]
}

func (f *sessionsFake) totalByIDHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.byIDHits {
		n += c
	}
	return n
}

func (f *sessionsFake) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.listQueries...)
}

func (f *sessionsFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/sessions"):
		f.listQueries = append(f.listQueries, r.URL.RawQuery)
		if f.listStatus != http.StatusOK {
			w.WriteHeader(f.listStatus)
			return
		}
		items := f.open
		if items == nil {
			items = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})
	case strings.Contains(r.URL.Path, "/session/"):
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		f.byIDHits[id]++
		if s, ok := f.closed[id]; ok {
			_ = json.NewEncoder(w).Encode(s)
			return
		}
		for _, s := range f.open {
			if s["id"] == id {
				_ = json.NewEncoder(w).Encode(s)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// retiredStepEvent builds a step bead parented to run1 whose session reference is
// either a durable gc.session_id stamp or a bare assignee.
func retiredStepEvent(seq uint64, beadID, stepID, status, assignee, stampedID string) events.Event {
	meta := map[string]string{
		"gc.kind":         "step",
		"gc.root_bead_id": "run1",
		"gc.step_id":      stepID,
		"gc.scope_ref":    "demo",
	}
	if stampedID != "" {
		meta[beadmeta.SessionIDMetadataKey] = stampedID
	}
	return beadCreatedEvent(seq, beads.Bead{
		ID:        beadID,
		Title:     stepID,
		Status:    status,
		Type:      "task",
		ParentID:  "run1",
		Ref:       "mol-adopt-pr-v2." + stepID,
		Assignee:  assignee,
		CreatedAt: time.Date(2026, 6, 1, 10, 1, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 1, 10, 5, 0, 0, time.UTC),
		Metadata:  meta,
	})
}

// handlerTransport dispatches the plane's self-reads against an in-process
// handler through the production SelfReadTransport seam — the same shape the
// supervisor's LoopbackTransport has — so no listener is opened.
type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// warmRunTailerWithSupervisor is warmRunTailer against a fake supervisor served
// in-process over the plane's self-read transport.
func warmRunTailerWithSupervisor(t *testing.T, supervisor http.Handler, evts ...events.Event) *cityRunTailer {
	t.Helper()
	dir := t.TempDir()
	writeEventLog(t, filepath.Join(dir, ".gc", "events.jsonl"), evts...)
	p := New(Deps{
		Resolver:          fakeResolver{paths: map[string]string{"alpha": dir}},
		SupervisorBaseURL: "http://supervisor.loopback",
		SelfReadTransport: handlerTransport{h: supervisor},
	})
	p.Start(t.Context())
	t.Cleanup(p.Stop)
	tl, ok := p.cityRunTailer("alpha")
	if !ok {
		t.Fatal("no tailer for alpha")
	}
	select {
	case <-tl.readyCh:
	case <-time.After(2 * time.Second):
		t.Fatal("cold replay did not complete")
	}
	return tl
}

// sessionLinksByBead flattens a detail's execution instances to bead id → session
// attachment.
func sessionLinksByBead(d runproj.FormulaRunDetail) map[string]runproj.RunSessionAttachment {
	out := map[string]runproj.RunSessionAttachment{}
	for _, node := range d.Nodes {
		for _, inst := range node.ExecutionInstances {
			out[inst.BeadID] = inst.Session
		}
	}
	return out
}

func expectLink(t *testing.T, links map[string]runproj.RunSessionAttachment, beadID, wantID, wantName string) {
	t.Helper()
	got := links[beadID]
	if got.Kind != "attached" {
		t.Fatalf("%s: session = %+v, want attached link", beadID, got)
	}
	if got.Link.SessionID != wantID || got.Link.SessionName != wantName {
		t.Fatalf("%s: link = %+v, want id %q name %q", beadID, got.Link, wantID, wantName)
	}
}

// TestRunDetailResolvesRetiredSessionLinksByID is the ga-3cs9p fix: a finished
// run's retired seats — steps whose session has CLOSED and left the open-only
// listing — resolve their session by durable id through the by-id read and render
// the session's alias, on both the stamped and the assignee-only path. The live
// listing stays the default open-only read (never state=closed), a live seat is
// never looked up by id, and an id the supervisor does not know keeps its correct
// bare link. Repeat reads at the same generation, and across a sessions-cache
// refresh, are served from the per-id cache with zero further upstream reads.
func TestRunDetailResolvesRetiredSessionLinksByID(t *testing.T) {
	closedA, closedB := retiredSessionID(1), retiredSessionID(2)
	const liveID, unknownID = "gcg-session-live", "gcg-session-unknown"
	fake := newSessionsFake()
	fake.addOpen(liveID, "live-worker")
	fake.addClosed(closedA, "adopt-a")
	fake.addClosed(closedB, "adopt-b")

	tl := warmRunTailerWithSupervisor(t, fake,
		runDetailRootEvent(),
		retiredStepEvent(2, "run1.1", "preflight", "closed", "", closedA),
		retiredStepEvent(3, "run1.2", "review", "closed", closedB, ""),
		retiredStepEvent(4, "run1.3", "merge", "in_progress", "", liveID),
		retiredStepEvent(5, "run1.4", "cleanup", "closed", "", unknownID),
	)
	ctx := context.Background()

	value, ready, err := tl.detail(ctx, "run1")
	if err != nil || !ready {
		t.Fatalf("detail: err=%v ready=%v", err, ready)
	}
	links := sessionLinksByBead(value.detail)
	expectLink(t, links, "run1.1", closedA, "adopt-a")
	expectLink(t, links, "run1.2", closedB, "adopt-b")
	expectLink(t, links, "run1.3", liveID, "live-worker")
	expectLink(t, links, "run1.4", unknownID, unknownID)

	for _, q := range fake.queries() {
		if strings.Contains(q, "closed") || strings.Contains(q, "state=") {
			t.Fatalf("sessions listing query %q must stay the default open-only read", q)
		}
	}
	if got := fake.hits(liveID); got != 0 {
		t.Fatalf("live session looked up by id %d times, want 0 (it is in the open listing)", got)
	}
	for _, id := range []string{closedA, closedB, unknownID} {
		if got := fake.hits(id); got != 1 {
			t.Fatalf("by-id reads for %s = %d, want exactly 1", id, got)
		}
	}
	hitsAfterFirst := fake.totalByIDHits()

	// Same generation: served from the detail memo, no upstream work at all.
	before := detailBuildCount.Load()
	if _, _, err := tl.detail(ctx, "run1"); err != nil {
		t.Fatalf("second detail: %v", err)
	}
	if builds := detailBuildCount.Load() - before; builds != 0 {
		t.Fatalf("same-generation repeat rebuilt %d times, want 0 (memo hit)", builds)
	}

	// Across a sessions-cache refresh (the session-event path expires the
	// listing) the detail rebuilds — the listing version bumped — but the
	// retired seats are served from the per-id cache.
	tl.refreshSessionEnrichment()
	again, _, err := tl.detail(ctx, "run1")
	if err != nil {
		t.Fatalf("third detail: %v", err)
	}
	if builds := detailBuildCount.Load() - before; builds != 1 {
		t.Fatalf("post-refresh detail rebuilt %d times, want 1 (listing version bumped)", builds)
	}
	expectLink(t, sessionLinksByBead(again.detail), "run1.1", closedA, "adopt-a")
	if got := fake.totalByIDHits(); got != hitsAfterFirst {
		t.Fatalf("by-id reads after a sessions refresh = %d, want %d (per-id cache must serve within its TTL)", got, hitsAfterFirst)
	}
}

// TestRunDetailRetiredLookupsAreBoundedByLinks is the performance contract: with
// thousands of closed sessions behind the supervisor, the run-detail path issues
// exactly one by-id read per distinct retired link the run carries — never a
// listing of closed beads — and the lookup count is capped at
// maxRetiredSessionLookups for a pathological run. Wall time per K is logged as
// the O(links) evidence for the PR.
func TestRunDetailRetiredLookupsAreBoundedByLinks(t *testing.T) {
	const closedSessions = 5000
	fake := newSessionsFake()
	for i := 0; i < closedSessions; i++ {
		fake.addClosed(retiredSessionID(i), "worker-"+strconv.Itoa(i))
	}

	for _, links := range []int{0, 1, 8, 32, maxRetiredSessionLookups + 5} {
		t.Run(fmt.Sprintf("links=%d", links), func(t *testing.T) {
			evts := []events.Event{runDetailRootEvent()}
			for i := 0; i < links; i++ {
				evts = append(evts, retiredStepEvent(uint64(i+2), "run1."+strconv.Itoa(i+1), "step-"+strconv.Itoa(i), "closed", "", retiredSessionID(i)))
			}
			tl := warmRunTailerWithSupervisor(t, fake, evts...)
			hitsBefore := fake.totalByIDHits()

			start := time.Now()
			value, ready, err := tl.detail(context.Background(), "run1")
			elapsed := time.Since(start)
			if err != nil || !ready {
				t.Fatalf("detail: err=%v ready=%v", err, ready)
			}

			wantLookups := links
			if wantLookups > maxRetiredSessionLookups {
				wantLookups = maxRetiredSessionLookups
			}
			if got := fake.totalByIDHits() - hitsBefore; got != wantLookups {
				t.Fatalf("by-id reads = %d, want %d (one per retired link, capped at %d)", got, wantLookups, maxRetiredSessionLookups)
			}
			for _, q := range fake.queries() {
				if strings.Contains(q, "closed") || strings.Contains(q, "state=") {
					t.Fatalf("sessions listing query %q must never ask for closed beads", q)
				}
			}
			named := 0
			for _, att := range sessionLinksByBead(value.detail) {
				if att.Kind == "attached" && strings.HasPrefix(att.Link.SessionName, "worker-") {
					named++
				}
			}
			if named != wantLookups {
				t.Fatalf("named retired links = %d, want %d", named, wantLookups)
			}
			t.Logf("closed sessions behind supervisor=%d links=%d by-id reads=%d detail wall=%s", closedSessions, links, wantLookups, elapsed)
		})
	}
}

// TestRunDetailRetiredMissIsRetriedAfterMissTTL: an id the supervisor does not
// know yet (a session bead still landing) is cached as a miss for
// retiredSessionMissTTL only — shorter than a hit's TTL — so the link is enriched
// on the first read after the session appears instead of being pinned bare until
// eviction. The clock is injected, so crossing the TTL costs no wall time.
func TestRunDetailRetiredMissIsRetriedAfterMissTTL(t *testing.T) {
	late := retiredSessionID(7)
	fake := newSessionsFake()
	tl := warmRunTailerWithSupervisor(t, fake,
		runDetailRootEvent(),
		retiredStepEvent(2, "run1.1", "preflight", "closed", "", late),
	)
	ctx := context.Background()
	var skew atomic.Int64
	base := time.Now()
	tl.mgr.retiredClock = func() time.Time { return base.Add(time.Duration(skew.Load())) }

	first, _, err := tl.detail(ctx, "run1")
	if err != nil {
		t.Fatalf("first detail: %v", err)
	}
	expectLink(t, sessionLinksByBead(first.detail), "run1.1", late, late)
	if got := fake.hits(late); got != 1 {
		t.Fatalf("by-id reads = %d, want 1", got)
	}

	// Within the miss TTL a repeat does not re-read upstream.
	if _, _, err := tl.detail(ctx, "run1"); err != nil {
		t.Fatalf("second detail: %v", err)
	}
	if got := fake.hits(late); got != 1 {
		t.Fatalf("by-id reads within miss TTL = %d, want 1", got)
	}

	// The session lands; the listing is expired the way a session event would
	// (so the memo rebuilds), but the miss is still inside its TTL: no re-read.
	fake.addClosed(late, "late-worker")
	tl.refreshSessionEnrichment()
	if _, _, err := tl.detail(ctx, "run1"); err != nil {
		t.Fatalf("detail inside miss TTL: %v", err)
	}
	if got := fake.hits(late); got != 1 {
		t.Fatalf("by-id reads inside miss TTL after a listing refresh = %d, want 1", got)
	}

	// Past the miss TTL (well inside a hit's TTL) the miss is forgotten and
	// re-read, and the link picks up the session.
	skew.Store(int64(retiredSessionMissTTL + time.Second))
	tl.refreshSessionEnrichment()
	third, _, err := tl.detail(ctx, "run1")
	if err != nil {
		t.Fatalf("third detail: %v", err)
	}
	expectLink(t, sessionLinksByBead(third.detail), "run1.1", late, "late-worker")
	if got := fake.hits(late); got != 2 {
		t.Fatalf("by-id reads after miss TTL = %d, want 2", got)
	}
}

// TestRunDetailSkipsRetiredLookupsWhenSessionsUnavailable: without a usable open
// listing there is no way to tell which links are retired, so the by-id reads are
// skipped entirely rather than issued blindly against a failing supervisor.
func TestRunDetailSkipsRetiredLookupsWhenSessionsUnavailable(t *testing.T) {
	fake := newSessionsFake()
	fake.listStatus = http.StatusInternalServerError
	fake.addClosed(retiredSessionID(1), "adopt-a")
	tl := warmRunTailerWithSupervisor(t, fake,
		runDetailRootEvent(),
		retiredStepEvent(2, "run1.1", "preflight", "closed", "", retiredSessionID(1)),
	)
	value, _, err := tl.detail(context.Background(), "run1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	expectLink(t, sessionLinksByBead(value.detail), "run1.1", retiredSessionID(1), retiredSessionID(1))
	if got := fake.totalByIDHits(); got != 0 {
		t.Fatalf("by-id reads with the listing unavailable = %d, want 0", got)
	}
}

// TestRunDetailRetiredByIDRequiresExactID: the by-id read resolves names as well
// as ids, so a 200 whose id differs from the requested durable id (a name that
// happened to resolve elsewhere) is treated as unknown rather than attributed.
func TestRunDetailRetiredByIDRequiresExactID(t *testing.T) {
	const asked = "gcg-session-asked"
	supervisor := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/sessions"):
			_, _ = w.Write([]byte(`{"items":[],"total":0}`))
		case strings.HasSuffix(r.URL.Path, "/session/"+asked):
			_, _ = w.Write([]byte(`{"id":"gcg-session-other","alias":"wrong-worker","state":"active","running":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	tl := warmRunTailerWithSupervisor(t, supervisor,
		runDetailRootEvent(),
		retiredStepEvent(2, "run1.1", "preflight", "closed", "", asked),
	)
	value, _, err := tl.detail(context.Background(), "run1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	expectLink(t, sessionLinksByBead(value.detail), "run1.1", asked, asked)
}

// TestRunDetailRetiredSessionsDroppedOnCityRebind: a city re-registered at another
// root gets a fresh per-id cache like every other per-city enrichment.
func TestRunDetailRetiredSessionsDroppedOnCityRebind(t *testing.T) {
	m := newRunTailerManager(Deps{})
	key := retiredSessionKey{name: "alpha", id: retiredSessionID(1)}
	other := retiredSessionKey{name: "beta", id: retiredSessionID(1)}
	for _, k := range []retiredSessionKey{key, other} {
		if _, err := m.retiredSessions.getOrBuild(k, func() (cachedRetiredSession, error) {
			return cachedRetiredSession{found: true, expires: time.Now().Add(time.Hour)}, nil
		}); err != nil {
			t.Fatalf("seed %v: %v", k, err)
		}
	}
	m.retiredSessions.forgetMatching(func(k retiredSessionKey) bool { return k.name == "alpha" })
	rebuilt := false
	if _, err := m.retiredSessions.getOrBuild(key, func() (cachedRetiredSession, error) {
		rebuilt = true
		return cachedRetiredSession{}, nil
	}); err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !rebuilt {
		t.Fatal("alpha's retired-session entry survived forgetMatching")
	}
	if _, err := m.retiredSessions.getOrBuild(other, func() (cachedRetiredSession, error) {
		t.Fatal("beta's entry must be untouched")
		return cachedRetiredSession{}, nil
	}); err != nil {
		t.Fatalf("beta re-read: %v", err)
	}
}
