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
	for _, sub := range []string{"logs", "sql", "list", "recover", "sync", "rollback"} {
		if !strings.Contains(out, sub) {
			t.Errorf("gc dolt --help missing subcommand %q in:\n%s", sub, out)
		}
	}
}

// --- gc dolt rollback ---

func TestDoltRollbackListEmpty(t *testing.T) {
	// City with no migration-backup-* dirs → "No backups found."
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	cityFlag = dir
	defer func() { cityFlag = "" }()

	var stdout, stderr bytes.Buffer
	code := doDoltRollbackList(dir, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doDoltRollbackList = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No backups found") {
		t.Errorf("stdout = %q, want 'No backups found'", stdout.String())
	}
}

func TestDoltRollbackListShowsBackups(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create two backup directories.
	if err := os.MkdirAll(filepath.Join(dir, "migration-backup-20250101-120000"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "migration-backup-20250102-120000"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	var stdout, stderr bytes.Buffer
	code := doDoltRollbackList(dir, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doDoltRollbackList = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "20250102-120000") {
		t.Errorf("stdout missing newer backup: %q", out)
	}
	if !strings.Contains(out, "20250101-120000") {
		t.Errorf("stdout missing older backup: %q", out)
	}
}

func TestDoltRollbackRequiresForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "migration-backup-20250101-120000", "town-beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	backupPath := filepath.Join(dir, "migration-backup-20250101-120000")
	var stdout, stderr bytes.Buffer
	code := doDoltRollback(dir, backupPath, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doDoltRollback without --force = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "--force") {
		t.Errorf("stderr = %q, want --force hint", stderr.String())
	}
}

func TestDoltRollbackRestore(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a backup with town-beads containing a test file.
	backupDir := filepath.Join(dir, "migration-backup-20250101-120000")
	townBeads := filepath.Join(backupDir, "town-beads")
	if err := os.MkdirAll(townBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeads, "beads.jsonl"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	var stdout, stderr bytes.Buffer
	code := doDoltRollback(dir, backupDir, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doDoltRollback = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Restored") {
		t.Errorf("stdout missing restore message: %q", out)
	}

	// Verify the town beads were actually restored.
	restored := filepath.Join(dir, ".beads", "beads.jsonl")
	if _, err := os.Stat(restored); err != nil {
		t.Errorf("restored file not found: %v", err)
	}
}

func TestDoltRollbackBadPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	var stdout, stderr bytes.Buffer
	code := doDoltRollback(dir, "/nonexistent/backup", true, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("doDoltRollback bad path = %d, want 1", code)
	}
}

func TestDoltRollbackByTimestamp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a backup identified by timestamp.
	backupDir := filepath.Join(dir, "migration-backup-20250101-120000")
	townBeads := filepath.Join(backupDir, "town-beads")
	if err := os.MkdirAll(townBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeads, "beads.jsonl"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	cityFlag = dir
	defer func() { cityFlag = "" }()

	// Pass just the timestamp — the command should resolve it.
	var stdout, stderr bytes.Buffer
	code := doDoltRollback(dir, "20250101-120000", true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doDoltRollback by timestamp = %d, want 0; stderr: %s", code, stderr.String())
	}
}
