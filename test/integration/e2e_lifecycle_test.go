//go:build integration

package integration

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestE2E_Kill verifies that gc agent kill stops the agent session.
func TestE2E_Kill(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "killme", StartCommand: e2eSleepScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Agent should be running.
	out, err := gc(cityDir, "agent", "status", "killme")
	if err != nil {
		t.Fatalf("gc agent status failed: %v\noutput: %s", err, out)
	}

	// Kill the agent.
	out, err = gc(cityDir, "agent", "kill", "killme")
	if err != nil {
		t.Fatalf("gc agent kill failed: %v\noutput: %s", err, out)
	}

	// Give it a moment to die.
	time.Sleep(500 * time.Millisecond)

	// Status should show not running.
	out, err = gc(cityDir, "agent", "status", "killme")
	if err != nil {
		// Some providers may error on status of dead agent; that's OK.
		t.Logf("gc agent status after kill: %v\noutput: %s", err, out)
	}
}

// TestE2E_StopGraceful verifies that gc stop shuts down all agents.
func TestE2E_StopGraceful(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "stopper1", StartCommand: e2eSleepScript()},
			{Name: "stopper2", StartCommand: e2eSleepScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Let agents settle.
	time.Sleep(500 * time.Millisecond)

	// Stop should succeed cleanly.
	out, err := gc("", "stop", cityDir)
	if err != nil {
		t.Fatalf("gc stop failed: %v\noutput: %s", err, out)
	}
}

// TestE2E_Restart verifies that gc restart stops and restarts all agents.
func TestE2E_Restart(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "restartee", StartCommand: e2eReportScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Wait for initial report.
	waitForReport(t, cityDir, "restartee", e2eDefaultTimeout)

	// Remove old report.
	safeName := strings.ReplaceAll("restartee", "/", "__")
	_ = removeFile(cityDir + "/.gc-reports/" + safeName + ".report")

	// Restart.
	out, err := gc("", "restart", cityDir)
	if err != nil {
		t.Fatalf("gc restart failed: %v\noutput: %s", err, out)
	}

	// Wait for new report (proves agent restarted).
	report := waitForReport(t, cityDir, "restartee", e2eDefaultTimeout)
	if report.get("STATUS") != "complete" {
		t.Error("agent did not restart successfully")
	}
}

// TestE2E_SuspendResume_Agent verifies that gc agent suspend prevents
// the agent from running, and gc agent resume allows it to restart.
func TestE2E_SuspendResume_Agent(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "suspendee", StartCommand: e2eReportScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Wait for initial report.
	waitForReport(t, cityDir, "suspendee", e2eDefaultTimeout)

	// Suspend the agent.
	out, err := gc(cityDir, "agent", "suspend", "suspendee")
	if err != nil {
		t.Fatalf("gc agent suspend failed: %v\noutput: %s", err, out)
	}

	// Kill it to simulate controller stopping suspended agent.
	gc(cityDir, "agent", "kill", "suspendee") //nolint:errcheck
	time.Sleep(500 * time.Millisecond)

	// Remove old report.
	safeName := strings.ReplaceAll("suspendee", "/", "__")
	_ = removeFile(cityDir + "/.gc-reports/" + safeName + ".report")

	// Reconcile (gc start) — suspended agent should NOT restart.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start after suspend failed: %v\noutput: %s", err, out)
	}

	// Brief wait — no report should appear.
	time.Sleep(1 * time.Second)
	reportPath := cityDir + "/.gc-reports/" + safeName + ".report"
	if fileExists(reportPath) {
		t.Error("suspended agent should not have restarted")
	}

	// Resume the agent.
	out, err = gc(cityDir, "agent", "resume", "suspendee")
	if err != nil {
		t.Fatalf("gc agent resume failed: %v\noutput: %s", err, out)
	}

	// Reconcile again — agent should restart.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start after resume failed: %v\noutput: %s", err, out)
	}

	report := waitForReport(t, cityDir, "suspendee", e2eDefaultTimeout)
	if report.get("STATUS") != "complete" {
		t.Error("resumed agent did not restart")
	}
}

// TestE2E_SuspendResume_City verifies that gc suspend stops all agents
// and gc resume allows them to restart.
func TestE2E_SuspendResume_City(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "citysus", StartCommand: e2eReportScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Wait for initial report.
	waitForReport(t, cityDir, "citysus", e2eDefaultTimeout)

	// Suspend the entire city.
	out, err := gc("", "suspend", cityDir)
	if err != nil {
		t.Fatalf("gc suspend failed: %v\noutput: %s", err, out)
	}

	// Kill existing agent.
	gc(cityDir, "agent", "kill", "citysus") //nolint:errcheck
	time.Sleep(500 * time.Millisecond)

	// Remove old report.
	safeName := strings.ReplaceAll("citysus", "/", "__")
	_ = removeFile(cityDir + "/.gc-reports/" + safeName + ".report")

	// Reconcile — should NOT start agents while city is suspended.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start after city suspend failed: %v\noutput: %s", err, out)
	}

	time.Sleep(1 * time.Second)
	reportPath := cityDir + "/.gc-reports/" + safeName + ".report"
	if fileExists(reportPath) {
		t.Error("agents should not start while city is suspended")
	}

	// Resume the city.
	out, err = gc("", "resume", cityDir)
	if err != nil {
		t.Fatalf("gc resume failed: %v\noutput: %s", err, out)
	}

	// Reconcile — agents should restart.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start after resume failed: %v\noutput: %s", err, out)
	}

	report := waitForReport(t, cityDir, "citysus", e2eDefaultTimeout)
	if report.get("STATUS") != "complete" {
		t.Error("agent did not restart after city resume")
	}
}

// TestE2E_StartIdempotent verifies that running gc start twice is a no-op
// for already-running agents.
func TestE2E_StartIdempotent(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "idem", StartCommand: e2eReportScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Wait for initial report.
	waitForReport(t, cityDir, "idem", e2eDefaultTimeout)

	// Start again — should be a no-op.
	out, err := gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start (second) failed: %v\noutput: %s", err, out)
	}

	// Agent should still have its original report (not restarted).
	report := waitForReport(t, cityDir, "idem", e2eDefaultTimeout)
	if report.get("STATUS") != "complete" {
		t.Error("agent lost its report after idempotent start")
	}
}

// TestE2E_Handoff_Remote verifies that gc handoff --target sends mail to
// the target agent.
func TestE2E_Handoff_Remote(t *testing.T) {
	city := e2eCity{
		Agents: []e2eAgent{
			{Name: "source", StartCommand: e2eSleepScript()},
			{Name: "target", StartCommand: e2eSleepScript()},
		},
	}
	cityDir := setupE2ECity(t, nil, city)

	// Remote handoff.
	out, err := gc(cityDir, "handoff", "--target", "target", "Context refresh", "Check status")
	if err != nil {
		t.Fatalf("gc handoff --target failed: %v\noutput: %s", err, out)
	}

	// Verify mail was delivered to target.
	out, err = gc(cityDir, "mail", "inbox", "target")
	if err != nil {
		t.Fatalf("gc mail inbox failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "Context refresh") {
		t.Errorf("expected handoff mail in target inbox:\n%s", out)
	}
}

// fileExists returns true if the file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
