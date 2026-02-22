package main

import (
	"os"

	"github.com/steveyegge/gascity/internal/session"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

// newSessionProvider returns a session.Provider based on the GC_SESSION
// environment variable. This allows txtar tests to exercise session-dependent
// commands without real tmux.
//
//   - "fake" → in-memory fake (all ops succeed)
//   - "fail" → broken fake (all ops return errors)
//   - default → real tmux provider
func newSessionProvider() session.Provider {
	switch os.Getenv("GC_SESSION") {
	case "fake":
		return session.NewFake()
	case "fail":
		return session.NewFailFake()
	default:
		return sessiontmux.NewProvider()
	}
}
