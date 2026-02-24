package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gascity/internal/fsys"
)

// fileData is the on-disk JSON format for the bead store.
type fileData struct {
	Seq   int    `json:"seq"`
	Beads []Bead `json:"beads"`
}

// FileStore is a file-backed Store implementation. It embeds a MemStore for
// all bead logic and adds JSON persistence — load on open, flush on every
// write. Fine for Tutorial 01 volumes.
type FileStore struct {
	*MemStore
	fs   fsys.FS
	path string
}

// OpenFileStore opens or creates a file-backed bead store at path. All file
// I/O goes through fs for testability. If the file exists, its contents are
// loaded into memory. If it doesn't exist, the store starts empty. Parent
// directories are created as needed.
func OpenFileStore(fs fsys.FS, path string) (*FileStore, error) {
	if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("opening file store: %w", err)
	}

	data, err := fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileStore{MemStore: NewMemStore(), fs: fs, path: path}, nil
		}
		return nil, fmt.Errorf("opening file store: %w", err)
	}

	var fd fileData
	if err := json.Unmarshal(data, &fd); err != nil {
		return nil, fmt.Errorf("opening file store: %w", err)
	}
	return &FileStore{MemStore: NewMemStoreFrom(fd.Seq, fd.Beads), fs: fs, path: path}, nil
}

// Create delegates to MemStore.Create and flushes to disk.
func (fs *FileStore) Create(b Bead) (Bead, error) {
	result, err := fs.MemStore.Create(b)
	if err != nil {
		return Bead{}, err
	}
	if err := fs.save(); err != nil {
		return Bead{}, err
	}
	return result, nil
}

// Update delegates to MemStore.Update and flushes to disk.
func (fs *FileStore) Update(id string, opts UpdateOpts) error {
	if err := fs.MemStore.Update(id, opts); err != nil {
		return err
	}
	return fs.save()
}

// Claim delegates to MemStore.Claim and flushes to disk.
func (fs *FileStore) Claim(id, assignee string) error {
	if err := fs.MemStore.Claim(id, assignee); err != nil {
		return err
	}
	return fs.save()
}

// Unclaim delegates to MemStore.Unclaim and flushes to disk.
func (fs *FileStore) Unclaim(id, assignee string) error {
	if err := fs.MemStore.Unclaim(id, assignee); err != nil {
		return err
	}
	return fs.save()
}

// Close delegates to MemStore.Close and flushes to disk.
func (fs *FileStore) Close(id string) error {
	if err := fs.MemStore.Close(id); err != nil {
		return err
	}
	return fs.save()
}

// save writes the full store state to disk atomically (temp file + rename).
func (fs *FileStore) save() error {
	fs.mu.Lock()
	seq, beads := fs.snapshot()
	fs.mu.Unlock()

	fd := fileData{Seq: seq, Beads: beads}
	data, err := json.MarshalIndent(fd, "", "  ")
	if err != nil {
		return fmt.Errorf("saving file store: %w", err)
	}

	tmp := fs.path + ".tmp"
	if err := fs.fs.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("saving file store: %w", err)
	}
	if err := fs.fs.Rename(tmp, fs.path); err != nil {
		return fmt.Errorf("saving file store: %w", err)
	}
	return nil
}
