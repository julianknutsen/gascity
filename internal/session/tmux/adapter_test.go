//go:build integration

package tmux

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/session/sessiontest"
)

// Compile-time check.
var _ session.Provider = (*Provider)(nil)

func TestTmuxConformance(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	p := NewProvider()
	var counter int64

	sessiontest.RunProviderTests(t, func(t *testing.T) (session.Provider, session.Config, string) {
		id := atomic.AddInt64(&counter, 1)
		name := fmt.Sprintf("gc-test-conform-%d", id)
		// Safety cleanup for orphan prevention.
		t.Cleanup(func() { _ = p.Stop(name) })
		return p, session.Config{
			Command: "sleep 300",
			WorkDir: t.TempDir(),
		}, name
	})
}

func TestProvider_StartStopIsRunning(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	p := NewProvider()
	name := "gc-test-adapter"

	// Clean slate.
	_ = p.Stop(name)

	if p.IsRunning(name) {
		t.Fatal("session should not exist before Start")
	}

	if err := p.Start(name, session.Config{Command: "sleep 300"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = p.Stop(name) }()

	if !p.IsRunning(name) {
		t.Fatal("session should be running after Start")
	}

	// Duplicate start is idempotent (zombie detection sees healthy session).
	if err := p.Start(name, session.Config{}); err != nil {
		t.Fatalf("duplicate Start should be idempotent: %v", err)
	}

	if err := p.Stop(name); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if p.IsRunning(name) {
		t.Fatal("session should not be running after Stop")
	}

	// Idempotent stop.
	if err := p.Stop(name); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestProvider_StartWithEnv(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}

	p := NewProvider()
	name := "gc-test-adapter-env"
	_ = p.Stop(name)

	err := p.Start(name, session.Config{
		Command: "sleep 300",
		Env:     map[string]string{"GC_TEST": "hello"},
	})
	if err != nil {
		t.Fatalf("Start with env: %v", err)
	}
	defer func() { _ = p.Stop(name) }()

	// Verify the env var was set.
	val, err := p.Tmux().GetEnvironment(name, "GC_TEST")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if val != "hello" {
		t.Fatalf("GC_TEST: got %q, want %q", val, "hello")
	}
}
