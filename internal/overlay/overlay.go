// Package overlay copies directory trees into agent working directories.
package overlay

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyDir recursively copies all files from srcDir into dstDir.
// Directory structure is preserved. File permissions are preserved.
// If srcDir does not exist, returns nil (no-op).
// Individual file copy failures are logged to stderr but don't abort.
func CopyDir(srcDir, dstDir string, stderr io.Writer) error {
	info, err := os.Stat(srcDir)
	if os.IsNotExist(err) {
		return nil // Missing source dir is a no-op (like Gas Town).
	}
	if err != nil {
		return fmt.Errorf("overlay: stat %q: %w", srcDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("overlay: %q is not a directory", srcDir)
	}
	return copyDirRecursive(srcDir, dstDir, "", stderr)
}

// copyDirRecursive walks srcBase/rel and copies files into dstBase/rel.
func copyDirRecursive(srcBase, dstBase, rel string, stderr io.Writer) error {
	srcPath := srcBase
	if rel != "" {
		srcPath = filepath.Join(srcBase, rel)
	}

	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("overlay: reading %q: %w", srcPath, err)
	}

	for _, entry := range entries {
		entryRel := entry.Name()
		if rel != "" {
			entryRel = filepath.Join(rel, entry.Name())
		}

		if entry.IsDir() {
			// Create destination subdirectory and recurse.
			dstSubDir := filepath.Join(dstBase, entryRel)
			if err := os.MkdirAll(dstSubDir, 0o755); err != nil {
				fmt.Fprintf(stderr, "overlay: mkdir %q: %v\n", dstSubDir, err) //nolint:errcheck
				continue
			}
			if err := copyDirRecursive(srcBase, dstBase, entryRel, stderr); err != nil {
				fmt.Fprintf(stderr, "overlay: %v\n", err) //nolint:errcheck
			}
			continue
		}

		// Copy file.
		if err := copyFile(filepath.Join(srcBase, entryRel), filepath.Join(dstBase, entryRel)); err != nil {
			fmt.Fprintf(stderr, "overlay: %v\n", err) //nolint:errcheck
		}
	}
	return nil
}

// copyFile copies a single file preserving permissions.
func copyFile(src, dst string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("creating parent for %q: %w", dst, err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %q: %w", src, err)
	}
	defer srcFile.Close() //nolint:errcheck // read-only file

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("creating %q: %w", dst, err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		closeErr := dstFile.Close()
		_ = closeErr
		return fmt.Errorf("copying %q → %q: %w", src, dst, err)
	}
	return dstFile.Close()
}
