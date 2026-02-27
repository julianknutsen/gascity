package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gascity/internal/dolt"
)

func TestDoltLogsNoLogFile(t *testing.T) {
	// Create a minimal city directory with no dolt log file.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	var stdout, stderr bytes.Buffer
	code := doDoltLogs(50, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doDoltLogs = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "log file not found") {
		t.Errorf("stderr = %q, want 'log file not found'", stderr.String())
	}
}

func TestDoltListEmptyDataDir(t *testing.T) {
	// Create a city with an empty dolt-data directory.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc", "dolt-data"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	// GC_DOLT=skip so ListDatabases reads from disk instead of querying a server.
	t.Setenv("GC_DOLT", "skip")

	var stdout, stderr bytes.Buffer
	code := doDoltList(&stdout, &stderr)
	if code != 0 {
		t.Fatalf("doDoltList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No databases found") {
		t.Errorf("stdout = %q, want 'No databases found'", stdout.String())
	}
}

func TestDoltRecoverRejectsRemote(t *testing.T) {
	// Create a city that resolves to a remote config.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	// Set a remote host to trigger the "not supported for remote" error.
	t.Setenv("GC_DOLT_HOST", "10.0.0.5")

	var stdout, stderr bytes.Buffer
	code := doDoltRecover(&stdout, &stderr)
	if code != 1 {
		t.Fatalf("doDoltRecover = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not supported for remote") {
		t.Errorf("stderr = %q, want 'not supported for remote'", stderr.String())
	}
}

func TestDoltSummary(t *testing.T) {
	tests := []struct {
		name    string
		results []dolt.SyncResult
		want    string
	}{
		{"empty", nil, "no databases"},
		{"one pushed", []dolt.SyncResult{
			{Database: "db1", Pushed: true},
		}, "1 pushed"},
		{"mixed", []dolt.SyncResult{
			{Database: "db1", Pushed: true},
			{Database: "db2", Skipped: true},
			{Database: "db3", Error: fmt.Errorf("fail")},
		}, "1 pushed, 1 skipped, 1 errors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := doltSummary(tt.results)
			if got != tt.want {
				t.Errorf("doltSummary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDoltCmdHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"dolt", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc dolt --help exited %d; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, sub := range []string{"logs", "sql", "list", "recover", "sync"} {
		if !strings.Contains(out, sub) {
			t.Errorf("gc dolt --help missing subcommand %q in:\n%s", sub, out)
		}
	}
}
