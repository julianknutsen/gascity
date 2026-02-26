package telemetry

import (
	"os"
	"strings"
)

// buildGCResourceAttrs builds the OTEL_RESOURCE_ATTRIBUTES value from GC context
// vars present in the current process environment.
// Returns "" when no GC vars are found.
func buildGCResourceAttrs() string {
	var attrs []string
	if v := os.Getenv("GC_AGENT"); v != "" {
		attrs = append(attrs, "gc.agent="+v)
	}
	if v := os.Getenv("GC_RIG"); v != "" {
		attrs = append(attrs, "gc.rig="+v)
	}
	if v := os.Getenv("GC_CITY"); v != "" {
		attrs = append(attrs, "gc.city="+v)
	}
	return strings.Join(attrs, ",")
}

// SetProcessOTELAttrs sets OTEL-related variables in the current process
// environment so that all bd subprocesses spawned via exec.Command inherit
// them automatically — no per-call injection needed.
//
// Sets:
//   - OTEL_RESOURCE_ATTRIBUTES — GC context labels (gc.agent, gc.rig, gc.city)
//   - BD_OTEL_METRICS_URL      — bd's own metrics var (mirrors GC_OTEL_METRICS_URL)
//   - BD_OTEL_LOGS_URL         — bd's own logs var   (mirrors GC_OTEL_LOGS_URL)
//   - CLAUDE_CODE_ENABLE_TELEMETRY=1 — enables Claude Code's built-in telemetry
//
// Called once at gc startup when telemetry is active.
// No-op when GC_OTEL_METRICS_URL is not set.
func SetProcessOTELAttrs() {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return
	}
	if attrs := buildGCResourceAttrs(); attrs != "" {
		_ = os.Setenv("OTEL_RESOURCE_ATTRIBUTES", attrs)
	}
	// Mirror GC vars into bd's own var names so bd subprocesses
	// emit their metrics to the same VictoriaMetrics instance.
	_ = os.Setenv("BD_OTEL_METRICS_URL", metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		_ = os.Setenv("BD_OTEL_LOGS_URL", logsURL)
	}
	// Enable Claude Code's built-in telemetry for agent sessions.
	_ = os.Setenv("CLAUDE_CODE_ENABLE_TELEMETRY", "1")
}

// OTELEnvForSubprocess returns OTEL environment variables to inject into bd
// subprocesses when cmd.Env is built explicitly (overriding os.Environ).
//
// Complements SetProcessOTELAttrs for callers that construct cmd.Env manually
// so the vars aren't lost when the explicit env slice is built from scratch.
//
// Returns nil when GC telemetry is not active (GC_OTEL_METRICS_URL not set).
func OTELEnvForSubprocess() []string {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return nil
	}
	var env []string
	if attrs := buildGCResourceAttrs(); attrs != "" {
		env = append(env, "OTEL_RESOURCE_ATTRIBUTES="+attrs)
	}
	env = append(env, "BD_OTEL_METRICS_URL="+metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		env = append(env, "BD_OTEL_LOGS_URL="+logsURL)
	}
	env = append(env, "CLAUDE_CODE_ENABLE_TELEMETRY=1")
	return env
}

// OTELEnvMap returns OTEL environment variables as a map for Gas City's
// mergeEnv() pattern. Returns nil when telemetry is not active.
func OTELEnvMap() map[string]string {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return nil
	}
	m := map[string]string{
		"BD_OTEL_METRICS_URL":          metricsURL,
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
	}
	if attrs := buildGCResourceAttrs(); attrs != "" {
		m["OTEL_RESOURCE_ATTRIBUTES"] = attrs
	}
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		m["BD_OTEL_LOGS_URL"] = logsURL
	}
	return m
}
