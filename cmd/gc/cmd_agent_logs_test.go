package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/sessionlog"
)

func writeTestSession(t *testing.T, searchBase, workDir string, lines ...string) {
	t.Helper()
	slug := strings.ReplaceAll(workDir, "/", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	dir := filepath.Join(searchBase, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "test-session.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDoAgentLogsBasic(t *testing.T) {
	searchBase := t.TempDir()
	workDir := t.TempDir()

	writeTestSession(t, searchBase, workDir,
		`{"uuid":"1","parentUuid":"","type":"user","message":{"role":"user","content":"hello"},"timestamp":"2025-01-01T00:00:00Z"}`,
		`{"uuid":"2","parentUuid":"1","type":"assistant","message":{"role":"assistant","content":"world"},"timestamp":"2025-01-01T00:00:01Z"}`,
	)

	path := sessionlog.FindSessionFile([]string{searchBase}, workDir)
	if path == "" {
		t.Fatal("session file not found")
	}

	var stdout, stderr bytes.Buffer
	code := doAgentLogs(path, false, 0, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "[USER]") {
		t.Errorf("output should contain [USER], got: %s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output should contain 'hello', got: %s", out)
	}
	if !strings.Contains(out, "[ASSISTANT]") {
		t.Errorf("output should contain [ASSISTANT], got: %s", out)
	}
	if !strings.Contains(out, "world") {
		t.Errorf("output should contain 'world', got: %s", out)
	}
}

func TestDoAgentLogsCompactBoundary(t *testing.T) {
	searchBase := t.TempDir()
	workDir := t.TempDir()

	// Two compact boundaries so tail=1 returns only the last segment.
	writeTestSession(t, searchBase, workDir,
		`{"uuid":"1","parentUuid":"","type":"user","message":{"role":"user","content":"first"},"timestamp":"2025-01-01T00:00:00Z"}`,
		`{"uuid":"2","parentUuid":"1","type":"assistant","message":{"role":"assistant","content":"reply"},"timestamp":"2025-01-01T00:00:01Z"}`,
		`{"uuid":"3","parentUuid":"2","type":"system","subtype":"compact_boundary","message":{"role":"system","content":"compacted 1"},"timestamp":"2025-01-01T00:00:02Z"}`,
		`{"uuid":"4","parentUuid":"3","type":"user","message":{"role":"user","content":"middle"},"timestamp":"2025-01-01T00:00:03Z"}`,
		`{"uuid":"5","parentUuid":"4","type":"assistant","message":{"role":"assistant","content":"middle reply"},"timestamp":"2025-01-01T00:00:04Z"}`,
		`{"uuid":"6","parentUuid":"5","type":"system","subtype":"compact_boundary","message":{"role":"system","content":"compacted 2"},"timestamp":"2025-01-01T00:00:05Z"}`,
		`{"uuid":"7","parentUuid":"6","type":"user","message":{"role":"user","content":"second"},"timestamp":"2025-01-01T00:00:06Z"}`,
		`{"uuid":"8","parentUuid":"7","type":"assistant","message":{"role":"assistant","content":"reply2"},"timestamp":"2025-01-01T00:00:07Z"}`,
	)

	path := sessionlog.FindSessionFile([]string{searchBase}, workDir)
	if path == "" {
		t.Fatal("session file not found")
	}

	// tail=1 should show only from the last compact boundary.
	var stdout, stderr bytes.Buffer
	code := doAgentLogs(path, false, 1, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if strings.Contains(out, "first") {
		t.Errorf("tail=1 should not include messages before compact boundary, got: %s", out)
	}
	if !strings.Contains(out, "context compacted") {
		t.Errorf("output should contain compact divider, got: %s", out)
	}
	if !strings.Contains(out, "second") {
		t.Errorf("output should contain 'second', got: %s", out)
	}
}

func TestDoAgentLogsToolUse(t *testing.T) {
	searchBase := t.TempDir()
	workDir := t.TempDir()

	writeTestSession(t, searchBase, workDir,
		`{"uuid":"1","parentUuid":"","type":"user","message":{"role":"user","content":"read file"},"timestamp":"2025-01-01T00:00:00Z"}`,
		`{"uuid":"2","parentUuid":"1","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will read the file."},{"type":"tool_use","name":"Read","id":"t1","input":{}}]},"timestamp":"2025-01-01T00:00:01Z"}`,
	)

	path := sessionlog.FindSessionFile([]string{searchBase}, workDir)
	if path == "" {
		t.Fatal("session file not found")
	}

	var stdout, stderr bytes.Buffer
	code := doAgentLogs(path, false, 0, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "I will read the file.") {
		t.Errorf("output should contain text content, got: %s", out)
	}
	if !strings.Contains(out, "tool_use: Read") {
		t.Errorf("output should contain tool_use, got: %s", out)
	}
}

func TestPrintLogEntryTimestamp(t *testing.T) {
	searchBase := t.TempDir()
	workDir := t.TempDir()

	writeTestSession(t, searchBase, workDir,
		`{"uuid":"1","parentUuid":"","type":"user","message":{"role":"user","content":"hi"},"timestamp":"2025-06-15T14:30:45Z"}`,
	)

	path := sessionlog.FindSessionFile([]string{searchBase}, workDir)
	if path == "" {
		t.Fatal("session file not found")
	}

	var stdout, stderr bytes.Buffer
	code := doAgentLogs(path, false, 0, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "14:30:45") {
		t.Errorf("output should contain timestamp, got: %s", out)
	}
}
