// Package fsys defines a minimal filesystem interface for testability.
//
// Production code uses [OSFS] which delegates to the os package.
// Tests use [Fake] which provides an in-memory filesystem with spy
// capabilities and error injection — following the same pattern as
// [session.Provider] / [session.Fake].
package fsys

import (
	"os"
)

// FS abstracts the filesystem operations used by CLI commands.
// It covers exactly the operations needed by cmdStart, cmdRigAdd,
// and cmdRigList — no more.
type FS interface {
	// MkdirAll creates a directory path and all parents that do not exist.
	MkdirAll(path string, perm os.FileMode) error

	// WriteFile writes data to the named file, creating it if necessary.
	WriteFile(name string, data []byte, perm os.FileMode) error

	// Stat returns file info for the named file.
	Stat(name string) (os.FileInfo, error)

	// ReadDir reads the named directory and returns its entries.
	ReadDir(name string) ([]os.DirEntry, error)
}

// OSFS implements [FS] by delegating to the os package.
type OSFS struct{}

// MkdirAll delegates to [os.MkdirAll].
func (OSFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// WriteFile delegates to [os.WriteFile].
func (OSFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// Stat delegates to [os.Stat].
func (OSFS) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// ReadDir delegates to [os.ReadDir].
func (OSFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}
