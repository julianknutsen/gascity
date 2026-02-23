package tmux

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/session"
)

// startCall records a single invocation on fakeStartOps with full arguments.
type startCall struct {
	method       string
	name         string
	workDir      string
	command      string
	env          map[string]string
	processNames []string
	rc           *RuntimeConfig
	timeout      time.Duration
}

// fakeStartOps records calls with full arguments and simulates outcomes
// for doStartSession tests.
type fakeStartOps struct {
	calls []startCall

	// createSession returns errors from this slice sequentially.
	// First call returns createErrs[0], second call returns createErrs[1], etc.
	// If the slice is exhausted, returns nil.
	createErrs []error
	createIdx  int

	isRuntimeRunningResult bool
	killErr                error
	waitCommandErr         error
	acceptBypassErr        error
	waitReadyErr           error
	hasSessionResult       bool
	hasSessionErr          error
}

func (f *fakeStartOps) createSession(name, workDir, command string, env map[string]string) error {
	f.calls = append(f.calls, startCall{
		method:  "createSession",
		name:    name,
		workDir: workDir,
		command: command,
		env:     env,
	})
	if f.createIdx < len(f.createErrs) {
		err := f.createErrs[f.createIdx]
		f.createIdx++
		return err
	}
	return nil
}

func (f *fakeStartOps) isRuntimeRunning(name string, processNames []string) bool {
	f.calls = append(f.calls, startCall{
		method:       "isRuntimeRunning",
		name:         name,
		processNames: processNames,
	})
	return f.isRuntimeRunningResult
}

func (f *fakeStartOps) killSession(name string) error {
	f.calls = append(f.calls, startCall{method: "killSession", name: name})
	return f.killErr
}

func (f *fakeStartOps) waitForCommand(name string, timeout time.Duration) error {
	f.calls = append(f.calls, startCall{
		method:  "waitForCommand",
		name:    name,
		timeout: timeout,
	})
	return f.waitCommandErr
}

func (f *fakeStartOps) acceptBypassWarning(name string) error {
	f.calls = append(f.calls, startCall{method: "acceptBypassWarning", name: name})
	return f.acceptBypassErr
}

func (f *fakeStartOps) waitForReady(name string, rc *RuntimeConfig, timeout time.Duration) error {
	f.calls = append(f.calls, startCall{
		method:  "waitForReady",
		name:    name,
		rc:      rc,
		timeout: timeout,
	})
	return f.waitReadyErr
}

func (f *fakeStartOps) hasSession(name string) (bool, error) {
	f.calls = append(f.calls, startCall{method: "hasSession", name: name})
	return f.hasSessionResult, f.hasSessionErr
}

// callMethods returns just the method names for sequence assertions.
func (f *fakeStartOps) callMethods() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.method
	}
	return out
}

// assertCallSequence is a helper that verifies the method call sequence.
func assertCallSequence(t *testing.T, ops *fakeStartOps, want []string) {
	t.Helper()
	got := ops.callMethods()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i, c := range got {
		if c != want[i] {
			t.Errorf("call %d = %q, want %q", i, c, want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// doStartSession tests
// ---------------------------------------------------------------------------

func TestDoStartSession_FireAndForget(t *testing.T) {
	ops := &fakeStartOps{}

	err := doStartSession(ops, "test-sess", session.Config{
		WorkDir: "/w",
		Command: "sleep 300",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No hints → only createSession called.
	assertCallSequence(t, ops, []string{"createSession"})

	// Verify arguments were passed through.
	c := ops.calls[0]
	if c.name != "test-sess" {
		t.Errorf("createSession name = %q, want %q", c.name, "test-sess")
	}
	if c.workDir != "/w" {
		t.Errorf("createSession workDir = %q, want %q", c.workDir, "/w")
	}
	if c.command != "sleep 300" {
		t.Errorf("createSession command = %q, want %q", c.command, "sleep 300")
	}
}

func TestDoStartSession_FullSequence(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		WorkDir:                "/proj",
		Command:                "claude",
		Env:                    map[string]string{"GC_AGENT": "mayor"},
		ReadyPromptPrefix:      "> ",
		ReadyDelayMs:           5000,
		ProcessNames:           []string{"claude", "node"},
		EmitsPermissionWarning: true,
	}

	err := doStartSession(ops, "gc-city-mayor", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallSequence(t, ops, []string{
		"createSession",
		"waitForCommand",
		"acceptBypassWarning",
		"waitForReady",
		"hasSession",
	})

	// Verify createSession got full config.
	create := ops.calls[0]
	if create.workDir != "/proj" {
		t.Errorf("createSession workDir = %q, want %q", create.workDir, "/proj")
	}
	if create.command != "claude" {
		t.Errorf("createSession command = %q, want %q", create.command, "claude")
	}
	if create.env["GC_AGENT"] != "mayor" {
		t.Errorf("createSession env = %v, want GC_AGENT=mayor", create.env)
	}

	// Verify session name flows to all ops.
	for i, c := range ops.calls {
		if c.name != "gc-city-mayor" {
			t.Errorf("call %d (%s): name = %q, want %q", i, c.method, c.name, "gc-city-mayor")
		}
	}

	// Verify waitForCommand got the right timeout.
	wfc := ops.calls[1]
	if wfc.timeout != 30*time.Second {
		t.Errorf("waitForCommand timeout = %v, want %v", wfc.timeout, 30*time.Second)
	}

	// Verify waitForReady got correct RuntimeConfig and timeout.
	wfr := ops.calls[3]
	if wfr.timeout != 60*time.Second {
		t.Errorf("waitForReady timeout = %v, want %v", wfr.timeout, 60*time.Second)
	}
	if wfr.rc == nil || wfr.rc.Tmux == nil {
		t.Fatal("waitForReady rc is nil")
	}
	if wfr.rc.Tmux.ReadyPromptPrefix != "> " {
		t.Errorf("rc.ReadyPromptPrefix = %q, want %q", wfr.rc.Tmux.ReadyPromptPrefix, "> ")
	}
	if wfr.rc.Tmux.ReadyDelayMs != 5000 {
		t.Errorf("rc.ReadyDelayMs = %d, want %d", wfr.rc.Tmux.ReadyDelayMs, 5000)
	}
	if len(wfr.rc.Tmux.ProcessNames) != 2 || wfr.rc.Tmux.ProcessNames[0] != "claude" {
		t.Errorf("rc.ProcessNames = %v, want [claude node]", wfr.rc.Tmux.ProcessNames)
	}
}

func TestDoStartSession_CreateFails(t *testing.T) {
	ops := &fakeStartOps{
		createErrs: []error{errors.New("tmux not found")},
	}

	err := doStartSession(ops, "test", session.Config{Command: "sleep 300"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating session") {
		t.Errorf("error = %q, want 'creating session'", err)
	}

	assertCallSequence(t, ops, []string{"createSession"})
}

func TestDoStartSession_SessionDiesDuringStartup(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: false, // session died
	}

	cfg := session.Config{
		Command:      "claude",
		ProcessNames: []string{"claude"},
	}

	err := doStartSession(ops, "test", cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "died during startup") {
		t.Errorf("error = %q, want 'died during startup'", err)
	}
}

func TestDoStartSession_HasSessionError(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionErr: errors.New("tmux crashed"),
	}

	cfg := session.Config{
		Command:      "claude",
		ProcessNames: []string{"claude"},
	}

	err := doStartSession(ops, "test", cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "verifying session") {
		t.Errorf("error = %q, want 'verifying session'", err)
	}
}

// ---------------------------------------------------------------------------
// Individual hint tests — each hint field activates specific steps
// ---------------------------------------------------------------------------

func TestDoStartSession_ProcessNamesOnly(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		Command:      "codex",
		ProcessNames: []string{"codex"},
	}

	err := doStartSession(ops, "test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ProcessNames → waitForCommand + hasSession.
	// No acceptBypassWarning, no waitForReady.
	assertCallSequence(t, ops, []string{
		"createSession",
		"waitForCommand",
		"hasSession",
	})

	// Verify isRuntimeRunning sees the process names in zombie detection path.
	// (Here create succeeded, so isRuntimeRunning isn't called.)
}

func TestDoStartSession_ReadyPromptPrefixOnly(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		Command:           "gemini",
		ReadyPromptPrefix: "❯ ",
	}

	err := doStartSession(ops, "test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ReadyPromptPrefix → waitForReady + hasSession.
	// No waitForCommand (no ProcessNames), no acceptBypassWarning.
	assertCallSequence(t, ops, []string{
		"createSession",
		"waitForReady",
		"hasSession",
	})

	// Verify RuntimeConfig carries the prefix.
	wfr := ops.calls[1]
	if wfr.rc.Tmux.ReadyPromptPrefix != "❯ " {
		t.Errorf("rc.ReadyPromptPrefix = %q, want %q", wfr.rc.Tmux.ReadyPromptPrefix, "❯ ")
	}
}

func TestDoStartSession_ReadyDelayOnly(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		Command:      "gemini",
		ReadyDelayMs: 3000,
	}

	err := doStartSession(ops, "test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallSequence(t, ops, []string{
		"createSession",
		"waitForReady",
		"hasSession",
	})

	// Verify RuntimeConfig carries the delay.
	wfr := ops.calls[1]
	if wfr.rc.Tmux.ReadyDelayMs != 3000 {
		t.Errorf("rc.ReadyDelayMs = %d, want %d", wfr.rc.Tmux.ReadyDelayMs, 3000)
	}
}

func TestDoStartSession_EmitsPermissionWarningOnly(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		Command:                "claude",
		EmitsPermissionWarning: true,
	}

	err := doStartSession(ops, "test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// EmitsPermissionWarning → acceptBypassWarning + hasSession.
	// No waitForCommand (no ProcessNames), no waitForReady (no prefix/delay).
	assertCallSequence(t, ops, []string{
		"createSession",
		"acceptBypassWarning",
		"hasSession",
	})
}

func TestDoStartSession_ProcessNamesAndReadyPrefix(t *testing.T) {
	ops := &fakeStartOps{
		hasSessionResult: true,
	}

	cfg := session.Config{
		Command:           "claude",
		ProcessNames:      []string{"claude"},
		ReadyPromptPrefix: "> ",
	}

	err := doStartSession(ops, "test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both ProcessNames and ReadyPromptPrefix but no EmitsPermissionWarning.
	assertCallSequence(t, ops, []string{
		"createSession",
		"waitForCommand",
		"waitForReady",
		"hasSession",
	})
}

// ---------------------------------------------------------------------------
// ensureFreshSession tests
// ---------------------------------------------------------------------------

func TestEnsureFreshSession_Success(t *testing.T) {
	ops := &fakeStartOps{}

	cfg := session.Config{
		WorkDir: "/proj",
		Command: "claude",
		Env:     map[string]string{"GC_AGENT": "mayor"},
	}
	err := ensureFreshSession(ops, "gc-test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallSequence(t, ops, []string{"createSession"})

	// Verify config passed through.
	c := ops.calls[0]
	if c.name != "gc-test" {
		t.Errorf("name = %q, want %q", c.name, "gc-test")
	}
	if c.workDir != "/proj" {
		t.Errorf("workDir = %q, want %q", c.workDir, "/proj")
	}
	if c.command != "claude" {
		t.Errorf("command = %q, want %q", c.command, "claude")
	}
	if c.env["GC_AGENT"] != "mayor" {
		t.Errorf("env = %v, want GC_AGENT=mayor", c.env)
	}
}

func TestEnsureFreshSession_ZombieDetection(t *testing.T) {
	ops := &fakeStartOps{
		createErrs:             []error{ErrSessionExists},
		isRuntimeRunningResult: false, // zombie
	}

	cfg := session.Config{
		WorkDir:      "/proj",
		Command:      "claude",
		ProcessNames: []string{"claude", "node"},
	}
	err := ensureFreshSession(ops, "gc-test", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallSequence(t, ops, []string{
		"createSession",
		"isRuntimeRunning",
		"killSession",
		"createSession",
	})

	// Verify isRuntimeRunning received the ProcessNames from config.
	irt := ops.calls[1]
	if len(irt.processNames) != 2 || irt.processNames[0] != "claude" || irt.processNames[1] != "node" {
		t.Errorf("isRuntimeRunning processNames = %v, want [claude node]", irt.processNames)
	}

	// Verify recreate (second createSession) passes same config as initial.
	first := ops.calls[0]
	second := ops.calls[3]
	if first.workDir != second.workDir {
		t.Errorf("recreate workDir = %q, initial = %q", second.workDir, first.workDir)
	}
	if first.command != second.command {
		t.Errorf("recreate command = %q, initial = %q", second.command, first.command)
	}
}

func TestEnsureFreshSession_HealthyExisting(t *testing.T) {
	ops := &fakeStartOps{
		createErrs:             []error{ErrSessionExists},
		isRuntimeRunningResult: true, // alive
	}

	err := ensureFreshSession(ops, "test", session.Config{
		Command:      "claude",
		ProcessNames: []string{"claude"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not kill or recreate.
	assertCallSequence(t, ops, []string{"createSession", "isRuntimeRunning"})
}

func TestEnsureFreshSession_ZombieKillFails(t *testing.T) {
	ops := &fakeStartOps{
		createErrs:             []error{ErrSessionExists},
		isRuntimeRunningResult: false, // zombie
		killErr:                errors.New("permission denied"),
	}

	err := ensureFreshSession(ops, "test", session.Config{Command: "claude"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "killing zombie session") {
		t.Errorf("error = %q, want 'killing zombie session'", err)
	}
}

func TestEnsureFreshSession_RecreateRace(t *testing.T) {
	// After zombie kill, recreate gets ErrSessionExists from a concurrent process.
	ops := &fakeStartOps{
		createErrs:             []error{ErrSessionExists, ErrSessionExists},
		isRuntimeRunningResult: false, // zombie
	}

	err := ensureFreshSession(ops, "test", session.Config{Command: "claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v (race should be tolerated)", err)
	}
}

func TestEnsureFreshSession_RecreateFails(t *testing.T) {
	ops := &fakeStartOps{
		createErrs:             []error{ErrSessionExists, errors.New("out of memory")},
		isRuntimeRunningResult: false, // zombie
	}

	err := ensureFreshSession(ops, "test", session.Config{Command: "claude"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating session after zombie cleanup") {
		t.Errorf("error = %q, want 'creating session after zombie cleanup'", err)
	}
}
