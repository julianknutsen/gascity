package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestHookNoWork(t *testing.T) {
	runner := func(string) (string, error) { return "", nil }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", false, runner, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doHook(no work) = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestHookHasWork(t *testing.T) {
	runner := func(string) (string, error) { return "hw-1  open  Fix the bug\n", nil }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", false, runner, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doHook(has work) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "hw-1") {
		t.Errorf("stdout = %q, want to contain %q", stdout.String(), "hw-1")
	}
}

func TestHookCommandError(t *testing.T) {
	runner := func(string) (string, error) { return "", fmt.Errorf("command failed") }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", false, runner, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doHook(error) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "command failed") {
		t.Errorf("stderr = %q, want to contain %q", stderr.String(), "command failed")
	}
}

func TestHookInjectNoWork(t *testing.T) {
	runner := func(string) (string, error) { return "", nil }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", true, runner, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doHook(inject, no work) = %d, want 0", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestHookInjectFormatsOutput(t *testing.T) {
	runner := func(string) (string, error) { return "hw-1  open  Fix the bug\n", nil }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", true, runner, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doHook(inject, work) = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "<system-reminder>") {
		t.Errorf("stdout missing <system-reminder>: %q", out)
	}
	if !strings.Contains(out, "</system-reminder>") {
		t.Errorf("stdout missing </system-reminder>: %q", out)
	}
	if !strings.Contains(out, "<work-items>") {
		t.Errorf("stdout missing <work-items>: %q", out)
	}
	if !strings.Contains(out, "hw-1") {
		t.Errorf("stdout missing work item: %q", out)
	}
	if !strings.Contains(out, "gc hook") {
		t.Errorf("stdout missing 'gc hook' hint: %q", out)
	}
}

func TestHookInjectAlwaysExitsZero(t *testing.T) {
	// Even on command failure, inject mode exits 0.
	runner := func(string) (string, error) { return "", fmt.Errorf("command failed") }
	var stdout, stderr bytes.Buffer
	code := doHook("bd ready", true, runner, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doHook(inject, error) = %d, want 0", code)
	}
}

func TestHookPassesWorkQuery(t *testing.T) {
	// Verify the runner receives the correct work query string.
	var received string
	runner := func(cmd string) (string, error) {
		received = cmd
		return "item-1\n", nil
	}
	var stdout, stderr bytes.Buffer
	doHook("bd ready --assignee=mayor", false, runner, &stdout, &stderr)
	if received != "bd ready --assignee=mayor" {
		t.Errorf("runner received %q, want %q", received, "bd ready --assignee=mayor")
	}
}
