package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileRecorder appends events to a JSONL file. It uses O_APPEND for
// cross-process safety and a mutex for in-process serialization.
// Recording errors are written to stderr and never returned.
type FileRecorder struct {
	mu     sync.Mutex
	file   *os.File
	seq    uint64
	stderr io.Writer
}

// NewFileRecorder opens (or creates) the event log at path. It scans any
// existing file to find the maximum sequence number so new events continue
// monotonically. Parent directories are created as needed.
func NewFileRecorder(path string, stderr io.Writer) (*FileRecorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating event log directory: %w", err)
	}

	// Scan existing file for max seq before opening for append.
	var maxSeq uint64
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var e Event
			if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Seq > maxSeq {
				maxSeq = e.Seq
			}
		}
		f.Close() //nolint:errcheck // read-only scan
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening event log: %w", err)
	}

	return &FileRecorder{
		file:   file,
		seq:    maxSeq,
		stderr: stderr,
	}, nil
}

// Record appends an event to the log. It auto-fills Seq and Ts (if zero).
// Errors are written to stderr — never returned.
func (r *FileRecorder) Record(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	e.Seq = r.seq
	if e.Ts.IsZero() {
		e.Ts = time.Now()
	}

	data, err := json.Marshal(e)
	if err != nil {
		fmt.Fprintf(r.stderr, "events: marshal: %v\n", err) //nolint:errcheck // best-effort stderr
		return
	}
	data = append(data, '\n')
	if _, err := r.file.Write(data); err != nil {
		fmt.Fprintf(r.stderr, "events: write: %v\n", err) //nolint:errcheck // best-effort stderr
	}
}

// Close closes the underlying file.
func (r *FileRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Close()
}
