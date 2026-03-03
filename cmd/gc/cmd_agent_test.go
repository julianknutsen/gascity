package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/fsys"
)

// ---------------------------------------------------------------------------
// doAgentAttach tests (no existing coverage)
// ---------------------------------------------------------------------------

func TestDoAgentAttachStartsThenAttaches(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !f.Running {
		t.Error("agent should be started before attach")
	}
	// Verify attach was called.
	attachCalled := false
	for _, c := range f.Calls {
		if c.Method == "Attach" {
			attachCalled = true
		}
	}
	if !attachCalled {
		t.Error("Attach was not called")
	}
	if !strings.Contains(stdout.String(), "Attaching to agent 'mayor'") {
		t.Errorf("stdout = %q, want attach message", stdout.String())
	}
}

func TestDoAgentAttachSkipsStartWhenRunning(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.Running = true

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	// Should NOT call Start since already running.
	for _, c := range f.Calls {
		if c.Method == "Start" {
			t.Error("Start should not be called when already running")
		}
	}
}

func TestDoAgentAttachStartFailure(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.StartErr = errors.New("tmux crashed")

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "tmux crashed") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
}

func TestDoAgentAttachFailure(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.AttachErr = errors.New("terminal gone")

	var stdout, stderr bytes.Buffer
	code := doAgentAttach(f, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "terminal gone") {
		t.Errorf("stderr = %q, want error message", stderr.String())
	}
}

// ---------------------------------------------------------------------------
// doAgentList — pool annotation (no existing coverage)
// ---------------------------------------------------------------------------

func TestDoAgentListPoolAnnotation(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`[workspace]
name = "test-city"

[[agents]]
name = "worker"
[agents.pool]
min = 2
max = 5
`)

	var stdout, stderr bytes.Buffer
	code := doAgentList(fs, "/city", "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pool:") {
		t.Errorf("stdout should annotate pool config: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "min=2") {
		t.Errorf("stdout should show min: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "max=5") {
		t.Errorf("stdout should show max: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// doAgentSuspend/Resume — bad config error path (no existing coverage)
// ---------------------------------------------------------------------------

func TestDoAgentSuspendBadConfig(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`invalid ][`)

	var stdout, stderr bytes.Buffer
	code := doAgentSuspend(fs, "/city", "mayor", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("stderr should contain error message")
	}
}

func TestDoAgentResumeBadConfig(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`invalid ][`)

	var stdout, stderr bytes.Buffer
	code := doAgentResume(fs, "/city", "mayor", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("stderr should contain error message")
	}
}

// ---------------------------------------------------------------------------
// doAgentAdd — qualified name with dir component (no existing coverage)
// ---------------------------------------------------------------------------

func TestDoAgentAddQualifiedName(t *testing.T) {
	fs := fsys.NewFake()
	fs.Files["/city/city.toml"] = []byte(`[workspace]
name = "test-city"
`)

	var stdout, stderr bytes.Buffer
	code := doAgentAdd(fs, "/city", "myrig/worker", "", "", false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}
	data := string(fs.Files["/city/city.toml"])
	if !strings.Contains(data, "worker") {
		t.Errorf("city.toml should contain agent name: %s", data)
	}
	if !strings.Contains(data, "myrig") {
		t.Errorf("city.toml should contain dir from qualified name: %s", data)
	}
}

// ---------------------------------------------------------------------------
// doAgentPeek — lines parameter (no existing coverage)
// ---------------------------------------------------------------------------

func TestDoAgentPeekPassesLineCount(t *testing.T) {
	f := agent.NewFake("mayor", "gc-city-mayor")
	f.FakePeekOutput = "output"

	var stdout, stderr bytes.Buffer
	code := doAgentPeek(f, 100, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	// Verify Peek was called with the right line count.
	for _, c := range f.Calls {
		if c.Method == "Peek" && c.Lines != 100 {
			t.Errorf("Peek called with lines=%d, want 100", c.Lines)
		}
	}
}
