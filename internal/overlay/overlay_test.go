package overlay

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDir_RecursiveCopy(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create source tree:
	//   file.txt
	//   sub/nested.txt
	//   sub/deep/leaf.txt
	writeFile(t, filepath.Join(src, "file.txt"), "top-level")
	mkdirAll(t, filepath.Join(src, "sub"))
	writeFile(t, filepath.Join(src, "sub", "nested.txt"), "nested content")
	mkdirAll(t, filepath.Join(src, "sub", "deep"))
	writeFile(t, filepath.Join(src, "sub", "deep", "leaf.txt"), "deep content")

	var stderr bytes.Buffer
	if err := CopyDir(src, dst, &stderr); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}

	assertFileContent(t, filepath.Join(dst, "file.txt"), "top-level")
	assertFileContent(t, filepath.Join(dst, "sub", "nested.txt"), "nested content")
	assertFileContent(t, filepath.Join(dst, "sub", "deep", "leaf.txt"), "deep content")
}

func TestCopyDir_PreservesPermissions(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create an executable file.
	path := filepath.Join(src, "run.sh")
	writeFile(t, path, "#!/bin/sh\necho hello")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	var stderr bytes.Buffer
	if err := CopyDir(src, dst, &stderr); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	info, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("permissions = %o, want 755", info.Mode().Perm())
	}
}

func TestCopyDir_MissingSrcDir(t *testing.T) {
	dst := t.TempDir()
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")

	var stderr bytes.Buffer
	err := CopyDir(nonExistent, dst, &stderr)
	if err != nil {
		t.Errorf("CopyDir should return nil for missing src, got: %v", err)
	}
}

func TestCopyDir_EmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	var stderr bytes.Buffer
	if err := CopyDir(src, dst, &stderr); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
}

func TestCopyDir_OverwriteExisting(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "config.toml"), "new content")
	writeFile(t, filepath.Join(dst, "config.toml"), "old content")

	var stderr bytes.Buffer
	if err := CopyDir(src, dst, &stderr); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	assertFileContent(t, filepath.Join(dst, "config.toml"), "new content")
}

func TestCopyDir_NestedSubdirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create deeply nested structure.
	mkdirAll(t, filepath.Join(src, "a", "b", "c"))
	writeFile(t, filepath.Join(src, "a", "b", "c", "deep.txt"), "deep")

	var stderr bytes.Buffer
	if err := CopyDir(src, dst, &stderr); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	assertFileContent(t, filepath.Join(dst, "a", "b", "c", "deep.txt"), "deep")
}

func TestCopyDir_SrcNotADirectory(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "file.txt")
	writeFile(t, src, "not a dir")
	dst := t.TempDir()

	var stderr bytes.Buffer
	err := CopyDir(src, dst, &stderr)
	if err == nil {
		t.Fatal("expected error when src is a file, got nil")
	}
}

// helpers

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %q: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%q content = %q, want %q", path, string(data), want)
	}
}
