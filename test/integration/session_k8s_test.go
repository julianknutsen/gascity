//go:build integration

package integration

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gascity/internal/session"
	sessionexec "github.com/steveyegge/gascity/internal/session/exec"
	"github.com/steveyegge/gascity/internal/session/sessiontest"
)

// TestK8sSessionConformance runs the session conformance suite against a
// real Kubernetes cluster via the exec provider. Requires:
//
//	GC_SESSION_K8S_SCRIPT — path to the gc-session-k8s script
//	GC_K8S_IMAGE         — container image (e.g. ubuntu:22.04)
//
// Example:
//
//	GC_SESSION_K8S_SCRIPT=./contrib/session-scripts/gc-session-k8s \
//	GC_K8S_IMAGE=ubuntu:22.04 \
//	go test -tags integration ./test/integration/ -run TestK8sSession
func TestK8sSessionConformance(t *testing.T) {
	script := os.Getenv("GC_SESSION_K8S_SCRIPT")
	if script == "" {
		t.Skip("GC_SESSION_K8S_SCRIPT not set")
	}

	p := sessionexec.NewProvider(script)
	var counter int64

	// Lifecycle tests: each creates its own pod (unavoidable).
	sessiontest.RunLifecycleTests(t, func(t *testing.T) (session.Provider, session.Config, string) {
		id := atomic.AddInt64(&counter, 1)
		name := fmt.Sprintf("gc-k8s-conform-%d", id)
		t.Cleanup(func() { _ = p.Stop(name) })
		return p, session.Config{
			Command: "sleep 300",
			WorkDir: "/tmp",
		}, name
	})

	// Shared-session tests: one pod for all metadata/observation/signaling.
	t.Run("SharedSession", func(t *testing.T) {
		name := "gc-k8s-shared"
		cfg := session.Config{Command: "sleep 300", WorkDir: "/tmp"}
		if err := p.Start(name, cfg); err != nil {
			t.Fatalf("Start shared session: %v", err)
		}
		t.Cleanup(func() { _ = p.Stop(name) })
		sessiontest.RunSessionTests(t, p, cfg, name)
	})
}
