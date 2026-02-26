package telemetry

import (
	"os"
	"strings"
	"testing"
)

func TestBuildGCResourceAttrs_Empty(t *testing.T) {
	t.Setenv("GC_AGENT", "")
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_CITY", "")

	result := buildGCResourceAttrs()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildGCResourceAttrs_AllVars(t *testing.T) {
	t.Setenv("GC_AGENT", "mayor")
	t.Setenv("GC_RIG", "tower")
	t.Setenv("GC_CITY", "/tmp/bright-lights")

	result := buildGCResourceAttrs()
	for _, want := range []string{"gc.agent=mayor", "gc.rig=tower", "gc.city=/tmp/bright-lights"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in result, got %q", want, result)
		}
	}
}

func TestBuildGCResourceAttrs_Comma(t *testing.T) {
	t.Setenv("GC_AGENT", "a")
	t.Setenv("GC_RIG", "b")
	t.Setenv("GC_CITY", "")

	result := buildGCResourceAttrs()
	if !strings.Contains(result, ",") {
		t.Errorf("expected comma-separated result, got %q", result)
	}
}

func TestOTELEnvMap_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	m := OTELEnvMap()
	if m != nil {
		t.Errorf("expected nil when telemetry disabled, got %v", m)
	}
}

func TestOTELEnvMap_Enabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "http://localhost:9428/insert/opentelemetry/v1/logs")
	t.Setenv("GC_AGENT", "")
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_CITY", "")

	m := OTELEnvMap()
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if m["BD_OTEL_METRICS_URL"] != "http://localhost:8428/opentelemetry/api/v1/push" {
		t.Errorf("BD_OTEL_METRICS_URL = %q", m["BD_OTEL_METRICS_URL"])
	}
	if m["BD_OTEL_LOGS_URL"] != "http://localhost:9428/insert/opentelemetry/v1/logs" {
		t.Errorf("BD_OTEL_LOGS_URL = %q", m["BD_OTEL_LOGS_URL"])
	}
	if m["CLAUDE_CODE_ENABLE_TELEMETRY"] != "1" {
		t.Errorf("CLAUDE_CODE_ENABLE_TELEMETRY = %q", m["CLAUDE_CODE_ENABLE_TELEMETRY"])
	}
}

func TestOTELEnvMap_NoLogsURL(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GC_AGENT", "")
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_CITY", "")

	m := OTELEnvMap()
	if _, ok := m["BD_OTEL_LOGS_URL"]; ok {
		t.Error("BD_OTEL_LOGS_URL should not be present when GC_OTEL_LOGS_URL is empty")
	}
}

func TestOTELEnvMap_WithResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GC_AGENT", "mayor")
	t.Setenv("GC_RIG", "tower")
	t.Setenv("GC_CITY", "")

	m := OTELEnvMap()
	attrs := m["OTEL_RESOURCE_ATTRIBUTES"]
	if !strings.Contains(attrs, "gc.agent=mayor") {
		t.Errorf("expected gc.agent in OTEL_RESOURCE_ATTRIBUTES, got %q", attrs)
	}
}

func TestOTELEnvForSubprocess_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	env := OTELEnvForSubprocess()
	if env != nil {
		t.Errorf("expected nil when telemetry disabled, got %v", env)
	}
}

func TestOTELEnvForSubprocess_BothURLs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "http://localhost:9428/insert/opentelemetry/v1/logs")
	t.Setenv("GC_AGENT", "")
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_CITY", "")

	env := OTELEnvForSubprocess()
	if len(env) == 0 {
		t.Fatal("expected non-empty env")
	}

	hasMetrics, hasLogs := false, false
	for _, e := range env {
		if strings.HasPrefix(e, "BD_OTEL_METRICS_URL=") {
			hasMetrics = true
		}
		if strings.HasPrefix(e, "BD_OTEL_LOGS_URL=") {
			hasLogs = true
		}
	}
	if !hasMetrics {
		t.Error("expected BD_OTEL_METRICS_URL in subprocess env")
	}
	if !hasLogs {
		t.Error("expected BD_OTEL_LOGS_URL in subprocess env")
	}
}

func TestSetProcessOTELAttrs_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	t.Setenv("BD_OTEL_METRICS_URL", "")
	t.Setenv("BD_OTEL_LOGS_URL", "")

	SetProcessOTELAttrs()

	if v := os.Getenv("BD_OTEL_METRICS_URL"); v != "" {
		t.Errorf("BD_OTEL_METRICS_URL should not be set when telemetry disabled, got %q", v)
	}
}

func TestSetProcessOTELAttrs_Enabled(t *testing.T) {
	metricsURL := "http://localhost:8428/opentelemetry/api/v1/push"
	logsURL := "http://localhost:9428/insert/opentelemetry/v1/logs"
	t.Setenv(EnvMetricsURL, metricsURL)
	t.Setenv(EnvLogsURL, logsURL)
	t.Setenv("GC_AGENT", "")
	t.Setenv("GC_RIG", "")
	t.Setenv("GC_CITY", "")

	SetProcessOTELAttrs()

	if got := os.Getenv("BD_OTEL_METRICS_URL"); got != metricsURL {
		t.Errorf("BD_OTEL_METRICS_URL = %q, want %q", got, metricsURL)
	}
	if got := os.Getenv("BD_OTEL_LOGS_URL"); got != logsURL {
		t.Errorf("BD_OTEL_LOGS_URL = %q, want %q", got, logsURL)
	}
	if got := os.Getenv("CLAUDE_CODE_ENABLE_TELEMETRY"); got != "1" {
		t.Errorf("CLAUDE_CODE_ENABLE_TELEMETRY = %q, want %q", got, "1")
	}
}

func TestSetProcessOTELAttrs_SetsResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GC_AGENT", "mayor")
	t.Setenv("GC_RIG", "tower")
	t.Setenv("GC_CITY", "")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")

	SetProcessOTELAttrs()

	got := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if got == "" {
		t.Error("expected OTEL_RESOURCE_ATTRIBUTES to be set")
	}
	if !strings.Contains(got, "gc.agent=mayor") {
		t.Errorf("expected gc.agent in OTEL_RESOURCE_ATTRIBUTES, got %q", got)
	}
}
