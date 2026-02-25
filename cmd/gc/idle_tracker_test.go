package main

import (
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/session"
)

// fakeIdleTracker is a test double for idleTracker.
type fakeIdleTracker struct {
	idle map[string]bool
}

func newFakeIdleTracker() *fakeIdleTracker {
	return &fakeIdleTracker{idle: make(map[string]bool)}
}

func (f *fakeIdleTracker) checkIdle(sessionName string, _ time.Time) bool {
	return f.idle[sessionName]
}

func (f *fakeIdleTracker) setTimeout(_ string, _ time.Duration) {}

// --- memoryIdleTracker unit tests ---

func TestIdleTrackerNoTimeout(t *testing.T) {
	sp := session.NewFake()
	it := newIdleTracker(sp)
	// No timeout configured → never idle.
	if it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should not be idle when no timeout is set")
	}
}

func TestIdleTrackerNotIdle(t *testing.T) {
	sp := session.NewFake()
	sp.SetActivity("gc-test-mayor", time.Now().Add(-5*time.Minute))

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-mayor", 15*time.Minute)

	if it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should not be idle: 5m activity < 15m timeout")
	}
}

func TestIdleTrackerIdle(t *testing.T) {
	sp := session.NewFake()
	sp.SetActivity("gc-test-mayor", time.Now().Add(-30*time.Minute))

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-mayor", 15*time.Minute)

	if !it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should be idle: 30m inactivity > 15m timeout")
	}
}

func TestIdleTrackerActivityError(t *testing.T) {
	sp := session.NewFailFake() // all ops return error

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-mayor", 15*time.Minute)

	// Error from provider → not idle (no false positive).
	if it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should not be idle when provider returns error")
	}
}

func TestIdleTrackerZeroActivity(t *testing.T) {
	sp := session.NewFake()
	// Activity not set → zero time returned.

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-mayor", 15*time.Minute)

	// Zero time from provider → not idle (no false positive).
	if it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should not be idle when provider returns zero time")
	}
}

func TestIdleTrackerSetTimeoutZeroDisables(t *testing.T) {
	sp := session.NewFake()
	sp.SetActivity("gc-test-mayor", time.Now().Add(-30*time.Minute))

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-mayor", 15*time.Minute)
	// Now disable.
	it.setTimeout("gc-test-mayor", 0)

	if it.checkIdle("gc-test-mayor", time.Now()) {
		t.Error("should not be idle after timeout disabled")
	}
}

func TestIdleTrackerNilProvider(t *testing.T) {
	it := newIdleTracker(nil)
	if it != nil {
		t.Error("newIdleTracker(nil) should return nil")
	}
}

func TestIdleTrackerDifferentSessions(t *testing.T) {
	sp := session.NewFake()
	sp.SetActivity("gc-test-a", time.Now().Add(-30*time.Minute))
	sp.SetActivity("gc-test-b", time.Now().Add(-2*time.Minute))

	it := newIdleTracker(sp)
	it.setTimeout("gc-test-a", 15*time.Minute)
	it.setTimeout("gc-test-b", 15*time.Minute)

	if !it.checkIdle("gc-test-a", time.Now()) {
		t.Error("agent A should be idle")
	}
	if it.checkIdle("gc-test-b", time.Now()) {
		t.Error("agent B should NOT be idle")
	}
}
