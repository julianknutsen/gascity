package config

import (
	"fmt"
	"time"
)

// ValidateDurations checks all duration string fields in the config and returns
// warnings for any values that cannot be parsed by time.ParseDuration. This
// catches typos like "5mins" (should be "5m") at config load time rather than
// silently defaulting to zero at runtime.
func ValidateDurations(cfg *City, source string) []string {
	var warnings []string
	check := func(context, field, value string) {
		if value == "" {
			return
		}
		if _, err := time.ParseDuration(value); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"%s: %s %s = %q is not a valid duration: %v",
				source, context, field, value, err))
		}
	}

	// Session config durations.
	check("[session]", "setup_timeout", cfg.Session.SetupTimeout)
	check("[session]", "nudge_ready_timeout", cfg.Session.NudgeReadyTimeout)
	check("[session]", "nudge_retry_interval", cfg.Session.NudgeRetryInterval)
	check("[session]", "nudge_lock_timeout", cfg.Session.NudgeLockTimeout)
	check("[session]", "startup_timeout", cfg.Session.StartupTimeout)

	// Daemon config durations.
	check("[daemon]", "patrol_interval", cfg.Daemon.PatrolInterval)
	check("[daemon]", "restart_window", cfg.Daemon.RestartWindow)
	check("[daemon]", "shutdown_timeout", cfg.Daemon.ShutdownTimeout)
	check("[daemon]", "wisp_gc_interval", cfg.Daemon.WispGCInterval)
	check("[daemon]", "wisp_ttl", cfg.Daemon.WispTTL)
	check("[daemon]", "drift_drain_timeout", cfg.Daemon.DriftDrainTimeout)

	// Automations config durations.
	check("[automations]", "max_timeout", cfg.Automations.MaxTimeout)

	// Per-agent durations.
	for _, a := range cfg.Agents {
		ctx := fmt.Sprintf("agent %q", a.QualifiedName())
		check(ctx, "idle_timeout", a.IdleTimeout)
		if a.Pool != nil {
			check(ctx+" [pool]", "drain_timeout", a.Pool.DrainTimeout)
		}
	}

	return warnings
}
