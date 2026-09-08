package worker

import (
	"fmt"
	"os"
	"sync"
)

// derivedActivityMemoCap bounds the memo. Entries are one per mirror path ever
// polled; past the cap the memo is cleared rather than evicted piecemeal, which
// costs one re-parse per live mirror and keeps the structure trivial.
const derivedActivityMemoCap = 1024

// derivedActivityMemo remembers the tail activity derived from a whole-file
// JSON mirror, keyed by path and fenced by the mirror's generation — mtime and
// size, the same identity LoadHistory stamps on a snapshot.
//
// Families that derive activity from history (zcode) have no tail chunk to
// read: the verdict needs the whole mirror parsed and normalized. State is
// polled per API request through a fresh handle each time, so without the memo
// every poll re-read the entire transcript. The adapter rewrites the mirror
// atomically per turn, so an unchanged generation is an unchanged verdict.
//
// A nil memo is valid and derives on every call.
type derivedActivityMemo struct {
	mu      sync.Mutex
	entries map[string]derivedActivityEntry
	derived int
}

type derivedActivityEntry struct {
	generation string
	activity   TailActivity
}

func newDerivedActivityMemo() *derivedActivityMemo {
	return &derivedActivityMemo{entries: make(map[string]derivedActivityEntry)}
}

// resolve returns the memoized activity for path's current generation, or
// runs derive and memoizes its verdict. Failures are returned, never cached.
func (m *derivedActivityMemo) resolve(path string, derive func() (TailActivity, error)) (TailActivity, error) {
	if m == nil {
		return derive()
	}
	info, err := os.Stat(path)
	if err != nil {
		return TailActivityUnknown, fmt.Errorf("stat transcript: %w", err)
	}
	generation := transcriptGenerationID(info)

	m.mu.Lock()
	entry, ok := m.entries[path]
	m.mu.Unlock()
	if ok && entry.generation == generation {
		return entry.activity, nil
	}

	activity, err := derive()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.derived++
	if err != nil {
		return activity, err
	}
	if len(m.entries) >= derivedActivityMemoCap {
		m.entries = make(map[string]derivedActivityEntry)
	}
	m.entries[path] = derivedActivityEntry{generation: generation, activity: activity}
	return activity, nil
}

// derivations reports how many times resolve ran its derive function.
func (m *derivedActivityMemo) derivations() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.derived
}

// transcriptGenerationID identifies one on-disk generation of a transcript by
// its mtime and size — the fence both the history snapshot and the activity
// memo use.
func transcriptGenerationID(info os.FileInfo) string {
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}
