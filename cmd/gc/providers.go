package main

import (
	"os"
	"path/filepath"

	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
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

// beadsProvider returns the bead store provider name.
// Priority: GC_BEADS env var → city.toml [beads].provider → "bd" default.
//
// Related env vars:
//   - GC_DOLT=skip — bypass dolt server lifecycle in init/start/stop.
//     Used by testscript and integration tests to avoid needing a real
//     dolt installation. Checked inline in cmd_init.go, cmd_start.go,
//     and cmd_stop.go.
func beadsProvider(cityPath string) string {
	if v := os.Getenv("GC_BEADS"); v != "" {
		return v
	}
	// Try to read provider from city.toml.
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err == nil && cfg.Beads.Provider != "" {
		return cfg.Beads.Provider
	}
	return "bd"
}
