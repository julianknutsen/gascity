package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"gc": func() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) },
	})
}

func TestTutorial01(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
	})
}

func TestRunNoArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run(nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run(nil) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no command specified") {
		t.Errorf("stderr = %q, want 'no command specified'", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"blorp"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([blorp]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `unknown command "blorp"`) {
		t.Errorf("stderr = %q, want 'unknown command'", stderr.String())
	}
}

func TestStartNoPath(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"start"}, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Errorf("run([start]) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "missing city path") {
		t.Errorf("stderr = %q, want 'missing city path'", stderr.String())
	}
}

func TestStartSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"start", t.TempDir()}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run([start dir]) = %d, want 0", code)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Welcome to Gas City!") {
		t.Errorf("stdout missing welcome message: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "city.toml") {
		t.Errorf("stdout missing city.toml reference: %q", stdout.String())
	}
}
