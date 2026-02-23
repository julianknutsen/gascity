package events

import "sync"

// Fake is an in-memory [Recorder] for testing. It captures all recorded
// events in the Events slice. Safe for concurrent use.
type Fake struct {
	mu     sync.Mutex
	Events []Event
}

// NewFake returns a ready-to-use [Fake] recorder.
func NewFake() *Fake {
	return &Fake{}
}

// Record appends the event to the Events slice.
func (f *Fake) Record(e Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Events = append(f.Events, e)
}
