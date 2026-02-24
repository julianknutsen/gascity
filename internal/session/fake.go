package session

import (
	"fmt"
	"sync"
)

// Fake is an in-memory [Provider] for testing. It records all calls
// (spy) and simulates session state (fake). Safe for concurrent use.
//
// When broken is true (via [NewFailFake]), all mutating operations return
// an error and IsRunning always returns false. Calls are still recorded.
type Fake struct {
	mu       sync.Mutex
	sessions map[string]Config // live sessions
	Calls    []Call            // recorded calls in order
	broken   bool              // when true, all ops fail
	Zombies  map[string]bool   // sessions with dead agent processes
}

// Call records a single method invocation on [Fake].
type Call struct {
	Method  string // "Start", "Stop", "IsRunning", "Attach", "ProcessAlive", or "Nudge"
	Name    string // session name argument
	Config  Config // only set for Start calls
	Message string // only set for Nudge calls
}

// NewFake returns a ready-to-use [Fake].
func NewFake() *Fake {
	return &Fake{sessions: make(map[string]Config), Zombies: make(map[string]bool)}
}

// NewFailFake returns a [Fake] where Start, Stop, and Attach always fail
// and IsRunning always returns false. Useful for testing error paths in
// session-dependent commands.
func NewFailFake() *Fake {
	return &Fake{sessions: make(map[string]Config), Zombies: make(map[string]bool), broken: true}
}

// Start creates a fake session. Returns an error if the name is taken.
// When broken, always returns an error.
func (f *Fake) Start(name string, cfg Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Start", Name: name, Config: cfg})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	if _, exists := f.sessions[name]; exists {
		return fmt.Errorf("session %q already exists", name)
	}
	f.sessions[name] = cfg
	return nil
}

// Stop removes a fake session. Returns nil if it doesn't exist.
// When broken, always returns an error.
func (f *Fake) Stop(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Stop", Name: name})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	delete(f.sessions, name)
	return nil
}

// IsRunning reports whether the fake session exists.
// When broken, always returns false.
func (f *Fake) IsRunning(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "IsRunning", Name: name})
	if f.broken {
		return false
	}
	_, exists := f.sessions[name]
	return exists
}

// Attach records the call but returns immediately (no terminal to attach).
// When broken, always returns an error.
func (f *Fake) Attach(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Attach", Name: name})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	if _, exists := f.sessions[name]; !exists {
		return fmt.Errorf("session %q not found", name)
	}
	return nil
}

// ProcessAlive reports whether the named session has a live agent process.
// Returns true if processNames is empty (no check possible).
// Returns false if the session is in the Zombies set or the fake is broken.
func (f *Fake) ProcessAlive(name string, processNames []string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "ProcessAlive", Name: name})
	if f.broken {
		return false
	}
	if len(processNames) == 0 {
		return true
	}
	return !f.Zombies[name]
}

// Nudge records the call and returns nil (or an error if broken).
func (f *Fake) Nudge(name, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Nudge", Name: name, Message: message})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	return nil
}
