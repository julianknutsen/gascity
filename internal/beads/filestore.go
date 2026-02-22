package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	path string
}

// OpenFileStore opens or creates a file-backed bead store at path. If the
// file exists, its contents are loaded into memory. If it doesn't exist,
// the store starts empty. Parent directories are created as needed.
func OpenFileStore(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("opening file store: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileStore{MemStore: NewMemStore(), path: path}, nil
		}
		return nil, fmt.Errorf("opening file store: %w", err)
	}

	var fd fileData
	if err := json.Unmarshal(data, &fd); err != nil {
		return nil, fmt.Errorf("opening file store: %w", err)
	}
	return &FileStore{MemStore: NewMemStoreFrom(fd.Seq, fd.Beads), path: path}, nil
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
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("saving file store: %w", err)
	}
	if err := os.Rename(tmp, fs.path); err != nil {
		return fmt.Errorf("saving file store: %w", err)
	}
	return nil
}
