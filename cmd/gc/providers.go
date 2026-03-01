package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/events"
	eventsexec "github.com/steveyegge/gascity/internal/events/exec"
	"github.com/steveyegge/gascity/internal/mail"
	"github.com/steveyegge/gascity/internal/mail/beadmail"
	mailexec "github.com/steveyegge/gascity/internal/mail/exec"
	"github.com/steveyegge/gascity/internal/session"
	sessionexec "github.com/steveyegge/gascity/internal/session/exec"
	sessionk8s "github.com/steveyegge/gascity/internal/session/k8s"
	sessionsubprocess "github.com/steveyegge/gascity/internal/session/subprocess"
	sessiontmux "github.com/steveyegge/gascity/internal/session/tmux"
)

// sessionProviderName returns the session provider name.
// Priority: GC_SESSION env var → city.toml [session].provider → "" (default: tmux).
func sessionProviderName() string {
	if v := os.Getenv("GC_SESSION"); v != "" {
		return v
	}
	if cp, err := resolveCity(); err == nil {
		if cfg, err := loadCityConfig(cp); err == nil && cfg.Session.Provider != "" {
			return cfg.Session.Provider
		}
	}
	return ""
}

// newSessionProvider returns a session.Provider based on the session provider
// name (env var → city.toml → default). This allows txtar tests to exercise
// session-dependent commands without real tmux.
//
//   - "fake" → in-memory fake (all ops succeed)
//   - "fail" → broken fake (all ops return errors)
//   - "subprocess" → headless child processes
//   - "exec:<script>" → user-supplied script (absolute path or PATH lookup)
//   - "k8s" → native Kubernetes provider (client-go)
//   - default → real tmux provider
func newSessionProvider() session.Provider {
	v := sessionProviderName()
	if strings.HasPrefix(v, "exec:") {
		return sessionexec.NewProvider(strings.TrimPrefix(v, "exec:"))
	}
	switch v {
	case "fake":
		return session.NewFake()
	case "fail":
		return session.NewFailFake()
	case "subprocess":
		return sessionsubprocess.NewProvider()
	case "k8s":
		p, err := sessionk8s.NewProvider()
		if err != nil {
			fmt.Fprintf(os.Stderr, "k8s session provider: %v\n", err) //nolint:errcheck // best-effort stderr
			os.Exit(1)
		}
		return p
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
	cfg, err := loadCityConfig(cityPath)
	if err == nil && cfg.Beads.Provider != "" {
		return cfg.Beads.Provider
	}
	return "bd"
}

// mailProviderName returns the mail provider name.
// Priority: GC_MAIL env var → city.toml [mail].provider → "" (default: beadmail).
func mailProviderName() string {
	if v := os.Getenv("GC_MAIL"); v != "" {
		return v
	}
	if cp, err := resolveCity(); err == nil {
		if cfg, err := loadCityConfig(cp); err == nil && cfg.Mail.Provider != "" {
			return cfg.Mail.Provider
		}
	}
	return ""
}

// newMailProvider returns a mail.Provider based on the mail provider name
// (env var → city.toml → default) and the given bead store (used as the
// default backend).
//
//   - "fake" → in-memory fake (all ops succeed)
//   - "fail" → broken fake (all ops return errors)
//   - "exec:<script>" → user-supplied script (absolute path or PATH lookup)
//   - default → beadmail (backed by beads.Store, no subprocess)
func newMailProvider(store beads.Store) mail.Provider {
	v := mailProviderName()
	if strings.HasPrefix(v, "exec:") {
		return mailexec.NewProvider(strings.TrimPrefix(v, "exec:"))
	}
	switch v {
	case "fake":
		return mail.NewFake()
	case "fail":
		return mail.NewFailFake()
	default:
		return beadmail.New(store)
	}
}

// openCityMailProvider opens the city's bead store and wraps it in a
// mail.Provider. Returns (nil, exitCode) on failure.
func openCityMailProvider(stderr io.Writer, cmdName string) (mail.Provider, int) {
	// For exec: and test doubles, no store needed.
	v := mailProviderName()
	if strings.HasPrefix(v, "exec:") || v == "fake" || v == "fail" {
		return newMailProvider(nil), 0
	}

	store, code := openCityStore(stderr, cmdName)
	if store == nil {
		return nil, code
	}
	return newMailProvider(store), 0
}

// eventsProviderName returns the events provider name.
// Priority: GC_EVENTS env var → city.toml [events].provider → "" (default: file JSONL).
func eventsProviderName() string {
	if v := os.Getenv("GC_EVENTS"); v != "" {
		return v
	}
	if cp, err := resolveCity(); err == nil {
		if cfg, err := loadCityConfig(cp); err == nil && cfg.Events.Provider != "" {
			return cfg.Events.Provider
		}
	}
	return ""
}

// newEventsProvider returns an events.Provider based on the events provider
// name (env var → city.toml → default) and the given events file path (used
// as the default backend).
//
//   - "fake" → in-memory fake (all ops succeed)
//   - "fail" → broken fake (all ops return errors)
//   - "exec:<script>" → user-supplied script (absolute path or PATH lookup)
//   - default → file-backed JSONL provider
func newEventsProvider(eventsPath string, stderr io.Writer) (events.Provider, error) {
	v := eventsProviderName()
	if strings.HasPrefix(v, "exec:") {
		return eventsexec.NewProvider(strings.TrimPrefix(v, "exec:"), stderr), nil
	}
	switch v {
	case "fake":
		return events.NewFake(), nil
	case "fail":
		return events.NewFailFake(), nil
	default:
		return events.NewFileRecorder(eventsPath, stderr)
	}
}

// openCityEventsProvider resolves the city and returns an events.Provider.
// Returns (nil, exitCode) on failure.
func openCityEventsProvider(stderr io.Writer, cmdName string) (events.Provider, int) {
	// For exec: and test doubles, no city needed.
	v := eventsProviderName()
	if strings.HasPrefix(v, "exec:") || v == "fake" || v == "fail" {
		p, err := newEventsProvider("", stderr)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
			return nil, 1
		}
		return p, 0
	}

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	eventsPath := filepath.Join(cityPath, ".gc", "events.jsonl")
	p, err := newEventsProvider(eventsPath, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	return p, 0
}
