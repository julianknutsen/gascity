//go:build integration

package integration

import (
	"testing"
)

// TestE2E_MultiAgent_Independent verifies that multiple agents start
// independently with their own GC_AGENT and custom env.
func TestE2E_MultiAgent_Independent(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "alpha",
				StartCommand: e2eReportScript(),
				Env:          map[string]string{"CUSTOM_ROLE": "alpha"},
			},
			{
				Name:         "beta",
				StartCommand: e2eReportScript(),
				Env:          map[string]string{"CUSTOM_ROLE": "beta"},
			},
			{
				Name:         "gamma",
				StartCommand: e2eReportScript(),
				Env:          map[string]string{"CUSTOM_ROLE": "gamma"},
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		report := waitForReport(t, cityDir, name, e2eDefaultTimeout())

		if !report.has("GC_AGENT", name) {
			t.Errorf("%s: GC_AGENT got %v, want [%s]", name, report.getAll("GC_AGENT"), name)
		}
		if !report.has("CUSTOM_ROLE", name) {
			t.Errorf("%s: CUSTOM_ROLE got %v, want [%s]", name, report.getAll("CUSTOM_ROLE"), name)
		}
	}
}

// TestE2E_MultiAgent_PoolAndFixed verifies that a pool agent and a fixed
// agent coexist in the same city.
func TestE2E_MultiAgent_PoolAndFixed(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "fixed",
				StartCommand: e2eReportScript(),
				Env:          map[string]string{"CUSTOM_TYPE": "fixed"},
			},
			{
				Name:         "pooled",
				StartCommand: e2eReportScript(),
				Env:          map[string]string{"CUSTOM_TYPE": "pooled"},
				Pool:         &e2ePool{Min: 2, Max: 2, Check: "echo 2"},
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Fixed agent uses bare name.
	fixedReport := waitForReport(t, cityDir, "fixed", e2eDefaultTimeout())
	if !fixedReport.has("GC_AGENT", "fixed") {
		t.Errorf("fixed agent GC_AGENT: got %v", fixedReport.getAll("GC_AGENT"))
	}

	// Pool agents use numbered names.
	for _, name := range []string{"pooled-1", "pooled-2"} {
		report := waitForReport(t, cityDir, name, e2eDefaultTimeout())
		if !report.has("GC_AGENT", name) {
			t.Errorf("%s GC_AGENT: got %v", name, report.getAll("GC_AGENT"))
		}
		if !report.has("CUSTOM_TYPE", "pooled") {
			t.Errorf("%s missing CUSTOM_TYPE=pooled", name)
		}
	}
}

// TestE2E_MultiAgent_CityAndRig verifies that city-scoped and rig-scoped
// agents can coexist, with rig agents receiving GC_RIG and the correct dir.
func TestE2E_MultiAgent_CityAndRig(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{
				Name:         "cityscoped",
				StartCommand: e2eReportScript(),
			},
			{
				Name:         "rigscoped",
				StartCommand: e2eReportScript(),
				Dir:          "rigs/myrig",
			},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	cityReport := waitForReport(t, cityDir, "cityscoped", e2eDefaultTimeout())
	// QualifiedName = dir/name = "rigs/myrig/rigscoped"
	rigReport := waitForReport(t, cityDir, "rigs/myrig/rigscoped", e2eDefaultTimeout())

	// City-scoped agent should not have GC_RIG.
	if cityReport.hasKey("GC_RIG") {
		t.Errorf("city-scoped agent has unexpected GC_RIG: %v", cityReport.getAll("GC_RIG"))
	}

	// Rig-scoped agent should have its dir set correctly.
	if cwd := rigReport.get("CWD"); cwd == "" {
		t.Error("rig-scoped agent CWD is empty")
	}
}
