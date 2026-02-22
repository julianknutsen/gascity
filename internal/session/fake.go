package session

import (
	"fmt"
	"sync"
)

// Fake is an in-memory [Provider] for testing. It records all calls
// (spy) and simulates session state (fake). Safe for concurrent use.
type Fake struct {
	mu       sync.Mutex
	sessions map[string]Config // live sessions
	Calls    []Call            // recorded calls in order
}

// Call records a single method invocation on [Fake].
type Call struct {
	Method string // "Start", "Stop", "IsRunning", or "Attach"
	Name   string // session name argument
	Config Config // only set for Start calls
}

// NewFake returns a ready-to-use [Fake].
func NewFake() *Fake {
	return &Fake{sessions: make(map[string]Config)}
}

// Start creates a fake session. Returns an error if the name is taken.
func (f *Fake) Start(name string, cfg Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Start", Name: name, Config: cfg})
	if _, exists := f.sessions[name]; exists {
		return fmt.Errorf("session %q already exists", name)
	}
	f.sessions[name] = cfg
	return nil
}

// Stop removes a fake session. Returns nil if it doesn't exist.
func (f *Fake) Stop(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Stop", Name: name})
	delete(f.sessions, name)
	return nil
}

// IsRunning reports whether the fake session exists.
func (f *Fake) IsRunning(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "IsRunning", Name: name})
	_, exists := f.sessions[name]
	return exists
}

// Attach records the call but returns immediately (no terminal to attach).
func (f *Fake) Attach(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Attach", Name: name})
	if _, exists := f.sessions[name]; !exists {
		return fmt.Errorf("session %q not found", name)
	}
	return nil
}

// Sessions returns a snapshot of the currently live session names.
func (f *Fake) Sessions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.sessions))
	for name := range f.sessions {
		names = append(names, name)
	}
	return names
}
