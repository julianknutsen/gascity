package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory [Provider] for testing. It captures all recorded
// events in the Events slice. Safe for concurrent use.
//
// When broken is true (via [NewFailFake]), all operations return errors.
type Fake struct {
	mu     sync.Mutex
	Events []Event
	seq    uint64
	broken bool
	notify chan struct{} // signaled on Record for watchers
}

// NewFake returns a ready-to-use in-memory event provider.
func NewFake() *Fake {
	return &Fake{notify: make(chan struct{}, 1)}
}

// NewFailFake returns an event provider where all operations return errors.
// Useful for testing error paths.
func NewFailFake() *Fake {
	return &Fake{broken: true, notify: make(chan struct{}, 1)}
}

// Record appends the event to the Events slice. Auto-fills Seq and Ts.
func (f *Fake) Record(e Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	e.Seq = f.seq
	if e.Ts.IsZero() {
		e.Ts = time.Now()
	}
	f.Events = append(f.Events, e)
	// Non-blocking notify for watchers.
	select {
	case f.notify <- struct{}{}:
	default:
	}
}

// List returns events matching the filter from the in-memory store.
func (f *Fake) List(filter Filter) ([]Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return nil, fmt.Errorf("events provider unavailable")
	}
	var result []Event
	for _, e := range f.Events {
		if filter.AfterSeq > 0 && e.Seq <= filter.AfterSeq {
			continue
		}
		if filter.Type != "" && e.Type != filter.Type {
			continue
		}
		if filter.Actor != "" && e.Actor != filter.Actor {
			continue
		}
		if !filter.Since.IsZero() && e.Ts.Before(filter.Since) {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

// LatestSeq returns the highest sequence number, or 0 if empty.
func (f *Fake) LatestSeq() (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return 0, fmt.Errorf("events provider unavailable")
	}
	return f.seq, nil
}

// Watch returns a Watcher that yields events from the in-memory store.
func (f *Fake) Watch(ctx context.Context, afterSeq uint64) (Watcher, error) {
	if f.broken {
		return nil, fmt.Errorf("events provider unavailable")
	}
	return &fakeWatcher{fake: f, afterSeq: afterSeq, ctx: ctx}, nil
}

// Close is a no-op for the fake provider.
func (f *Fake) Close() error {
	return nil
}

// fakeWatcher watches the Fake's Events slice for new events.
type fakeWatcher struct {
	fake     *Fake
	afterSeq uint64
	ctx      context.Context
}

// Next blocks until the next event with Seq > afterSeq is available.
func (w *fakeWatcher) Next() (Event, error) {
	for {
		// Check in-memory events.
		w.fake.mu.Lock()
		for _, e := range w.fake.Events {
			if e.Seq > w.afterSeq {
				w.afterSeq = e.Seq
				w.fake.mu.Unlock()
				return e, nil
			}
		}
		w.fake.mu.Unlock()

		// Wait for notification or context cancel.
		select {
		case <-w.ctx.Done():
			return Event{}, w.ctx.Err()
		case <-w.fake.notify:
			// New event recorded — check again.
		}
	}
}

// Close is a no-op for fake watchers.
func (w *fakeWatcher) Close() error {
	return nil
}
