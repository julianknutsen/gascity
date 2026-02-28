//go:build integration

package subprocess

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/session/sessiontest"
)

func TestSubprocessConformance(t *testing.T) {
	p := NewProviderWithDir(filepath.Join(t.TempDir(), "pids"))
	var counter int64

	sessiontest.RunProviderTests(t, func(t *testing.T) (session.Provider, session.Config, string) {
		id := atomic.AddInt64(&counter, 1)
		name := fmt.Sprintf("gc-subproc-conform-%d", id)
		// Safety cleanup: stop any lingering process.
		t.Cleanup(func() { _ = p.Stop(name) })
		return p, session.Config{
			Command: "sleep 300",
			WorkDir: t.TempDir(),
		}, name
	})
}
