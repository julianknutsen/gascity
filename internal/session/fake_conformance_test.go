package session_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/session/sessiontest"
)

func TestFakeConformance(t *testing.T) {
	fp := session.NewFake()
	var counter int64

	sessiontest.RunProviderTests(t, func(_ *testing.T) (session.Provider, session.Config, string) {
		id := atomic.AddInt64(&counter, 1)
		name := fmt.Sprintf("fake-conform-%d", id)
		return fp, session.Config{}, name
	})
}
