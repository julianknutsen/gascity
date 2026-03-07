package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnforceGCPermissions_TightensLooseDir(t *testing.T) {
	cityPath := t.TempDir()
	gcDir := filepath.Join(cityPath, ".gc")
	secretsPath := filepath.Join(gcDir, secretsDir)

	// Create with loose permissions (as gc init currently does).
	if err := os.MkdirAll(secretsPath, 0o755); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	enforceGCPermissions(cityPath, &stderr)

	// Verify .gc/ is tightened.
	info, _ := os.Stat(gcDir)
	if perm := info.Mode().Perm(); perm != gcDirPerm {
		t.Errorf(".gc/ perm = %o, want %o", perm, gcDirPerm)
	}

	// Verify .gc/secrets/ is tightened.
	info, _ = os.Stat(secretsPath)
	if perm := info.Mode().Perm(); perm != secretsDirPerm {
		t.Errorf(".gc/secrets/ perm = %o, want %o", perm, secretsDirPerm)
	}
}

func TestEnforceGCPermissions_NoErrorWhenMissing(t *testing.T) {
	cityPath := t.TempDir()
	var stderr bytes.Buffer
	// Should not error when .gc/ doesn't exist.
	enforceGCPermissions(cityPath, &stderr)
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestEnforceGCPermissions_AlreadyCorrect(t *testing.T) {
	cityPath := t.TempDir()
	gcDir := filepath.Join(cityPath, ".gc")
	if err := os.MkdirAll(gcDir, gcDirPerm); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	enforceGCPermissions(cityPath, &stderr)

	info, _ := os.Stat(gcDir)
	if perm := info.Mode().Perm(); perm != gcDirPerm {
		t.Errorf(".gc/ perm = %o, want %o", perm, gcDirPerm)
	}
}
