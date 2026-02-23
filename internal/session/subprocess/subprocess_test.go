package subprocess

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/session"
)

func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	return NewProviderWithDir(filepath.Join(t.TempDir(), "pids"))
}

func TestStartCreatesProcess(t *testing.T) {
	p := newTestProvider(t)
	err := p.Start("test", session.Config{Command: "sleep 3600"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop("test") //nolint:errcheck

	if !p.IsRunning("test") {
		t.Error("expected IsRunning=true after Start")
	}
}

func TestStartDuplicateNameFails(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("dup", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer p.Stop("dup") //nolint:errcheck

	err := p.Start("dup", session.Config{Command: "sleep 3600"})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestStartReusesDeadName(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("reuse", session.Config{Command: "true"}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if p.IsRunning("reuse") {
		t.Fatal("expected process to have exited")
	}

	if err := p.Start("reuse", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	defer p.Stop("reuse") //nolint:errcheck

	if !p.IsRunning("reuse") {
		t.Error("expected IsRunning=true after reuse")
	}
}

func TestStopKillsProcess(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("kill", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop("kill"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if p.IsRunning("kill") {
		t.Error("expected IsRunning=false after Stop")
	}
}

func TestStopIdempotent(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Stop("nonexistent"); err != nil {
		t.Errorf("Stop(nonexistent) = %v, want nil", err)
	}
}

func TestStopDeadProcess(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("dead", session.Config{Command: "true"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := p.Stop("dead"); err != nil {
		t.Errorf("Stop(dead) = %v, want nil", err)
	}
}

func TestIsRunningFalseAfterExit(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("short", session.Config{Command: "true"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if p.IsRunning("short") {
		t.Error("expected IsRunning=false after process exits")
	}
}

func TestIsRunningFalseForUnknown(t *testing.T) {
	p := newTestProvider(t)
	if p.IsRunning("unknown") {
		t.Error("expected IsRunning=false for unknown session")
	}
}

func TestAttachReturnsError(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Attach("anything"); err == nil {
		t.Error("expected Attach to return error")
	}
}

func TestEnvPassedToProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "env.txt")

	p := newTestProvider(t)
	err := p.Start("env-test", session.Config{
		Command: "echo $GC_TEST_VAR > " + marker,
		Env:     map[string]string{"GC_TEST_VAR": "hello-from-subprocess"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop("env-test") //nolint:errcheck

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(marker)
		if err == nil && len(data) > 0 {
			got := string(data)
			if got != "hello-from-subprocess\n" {
				t.Errorf("env var = %q, want %q", got, "hello-from-subprocess\n")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for env marker file")
}

func TestWorkDirSet(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "pwd.txt")

	p := newTestProvider(t)
	err := p.Start("workdir-test", session.Config{
		Command: "pwd > " + marker,
		WorkDir: dir,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop("workdir-test") //nolint:errcheck

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(marker)
		if err == nil && len(data) > 0 {
			got := string(data)
			want := dir + "\n"
			if got != want {
				t.Errorf("workdir = %q, want %q", got, want)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for workdir marker file")
}

func TestPIDFileWritten(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("pid-check", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop("pid-check") //nolint:errcheck

	data, err := os.ReadFile(p.pidPath("pid-check"))
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		t.Errorf("PID file contains %q, want a positive integer", string(data))
	}
}

func TestPIDFileRemovedAfterStop(t *testing.T) {
	p := newTestProvider(t)
	if err := p.Start("cleanup", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop("cleanup"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if _, err := os.Stat(p.pidPath("cleanup")); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop")
	}
}

func TestCrossProcessStopByPID(t *testing.T) {
	// Simulate the gc start → gc stop cross-process pattern:
	// Provider 1 starts a process, Provider 2 (same dir) stops it.
	dir := filepath.Join(t.TempDir(), "pids")

	p1 := NewProviderWithDir(dir)
	if err := p1.Start("cross", session.Config{Command: "sleep 3600"}); err != nil {
		t.Fatalf("p1.Start: %v", err)
	}

	// Read the PID to verify the process is alive.
	pid, err := p1.readPID("cross")
	if err != nil {
		t.Fatalf("reading PID: %v", err)
	}
	if syscall.Kill(pid, 0) != nil {
		t.Fatal("process should be alive")
	}

	// New provider (simulates gc stop in a separate process).
	p2 := NewProviderWithDir(dir)
	if !p2.IsRunning("cross") {
		t.Fatal("p2.IsRunning should be true via PID file")
	}
	if err := p2.Stop("cross"); err != nil {
		t.Fatalf("p2.Stop: %v", err)
	}

	// Process should be dead.
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		t.Error("process should be dead after cross-process Stop")
	}
}
