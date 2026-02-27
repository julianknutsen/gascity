package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Fake is an in-memory [Provider] for testing. It records all calls
// (spy) and simulates session state (fake). Safe for concurrent use.
//
// When broken is true (via [NewFailFake]), all mutating operations return
// an error and IsRunning always returns false. Calls are still recorded.
type Fake struct {
	mu         sync.Mutex
	sessions   map[string]Config            // live sessions
	meta       map[string]map[string]string // session → key → value
	Calls      []Call                       // recorded calls in order
	broken     bool                         // when true, all ops fail
	Zombies    map[string]bool              // sessions with dead agent processes
	PeekOutput map[string]string            // session → canned peek output
	Activity   map[string]time.Time         // session → last activity time
}

// Call records a single method invocation on [Fake].
type Call struct {
	Method  string // method name (e.g. "Start", "Stop", "SetMeta")
	Name    string // session name argument
	Config  Config // only set for Start calls
	Message string // only set for Nudge calls
	Key     string // only set for meta calls
	Value   string // only set for SetMeta calls
}

// NewFake returns a ready-to-use [Fake].
func NewFake() *Fake {
	return &Fake{
		sessions: make(map[string]Config),
		meta:     make(map[string]map[string]string),
		Zombies:  make(map[string]bool),
	}
}

// NewFailFake returns a [Fake] where Start, Stop, and Attach always fail
// and IsRunning always returns false. Useful for testing error paths in
// session-dependent commands.
func NewFailFake() *Fake {
	return &Fake{
		sessions: make(map[string]Config),
		meta:     make(map[string]map[string]string),
		Zombies:  make(map[string]bool),
		broken:   true,
	}
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

// Interrupt records the call. Best-effort: returns nil normally,
// or an error if the fake is broken.
func (f *Fake) Interrupt(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Interrupt", Name: name})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
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

// SetMeta stores a key-value pair for the named session.
func (f *Fake) SetMeta(name, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "SetMeta", Name: name, Key: key, Value: value})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	if f.meta[name] == nil {
		f.meta[name] = make(map[string]string)
	}
	f.meta[name][key] = value
	return nil
}

// GetMeta retrieves a metadata value. Returns ("", nil) if not set.
func (f *Fake) GetMeta(name, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "GetMeta", Name: name, Key: key})
	if f.broken {
		return "", fmt.Errorf("session unavailable")
	}
	return f.meta[name][key], nil
}

// RemoveMeta removes a metadata key from the named session.
func (f *Fake) RemoveMeta(name, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "RemoveMeta", Name: name, Key: key})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	delete(f.meta[name], key)
	return nil
}

// SetPeekOutput sets the canned output returned by [Fake.Peek] for the
// named session. Used in test setup.
func (f *Fake) SetPeekOutput(name, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PeekOutput == nil {
		f.PeekOutput = make(map[string]string)
	}
	f.PeekOutput[name] = content
}

// Peek returns canned output for the named session. Records the call.
// Returns ("", error) if broken.
func (f *Fake) Peek(name string, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Peek", Name: name})
	if f.broken {
		return "", fmt.Errorf("session unavailable")
	}
	return f.PeekOutput[name], nil
}

// ListRunning returns session names matching the given prefix.
func (f *Fake) ListRunning(prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "ListRunning"})
	if f.broken {
		return nil, fmt.Errorf("session unavailable")
	}
	var names []string
	for name := range f.sessions {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, nil
}

// SetActivity sets the canned last activity time for the named session.
// Used in test setup.
func (f *Fake) SetActivity(name string, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Activity == nil {
		f.Activity = make(map[string]time.Time)
	}
	f.Activity[name] = t
}

// GetLastActivity returns the configured activity time for the named session.
// Returns zero time if not set.
func (f *Fake) GetLastActivity(name string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "GetLastActivity", Name: name})
	if f.broken {
		return time.Time{}, fmt.Errorf("session unavailable")
	}
	return f.Activity[name], nil
}

// ClearScrollback records the call and returns nil (or error if broken).
func (f *Fake) ClearScrollback(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "ClearScrollback", Name: name})
	if f.broken {
		return fmt.Errorf("session unavailable")
	}
	return nil
}
