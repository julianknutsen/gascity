package agent

import (
	"sync"

	"github.com/steveyegge/gascity/internal/session"
)

// Call records a method invocation on [Fake].
type Call struct {
	Method  string // "Name", "SessionName", "IsRunning", "Start", "Stop", "Attach", "Nudge", "Peek", or "SessionConfig"
	Name    string // agent name at time of call
	Message string // only set for Nudge calls
	Lines   int    // only set for Peek calls
}

// Fake is a test double for [Agent] with spy and configurable errors.
// Set the exported error fields to inject failures per-test.
// Safe for concurrent use.
type Fake struct {
	mu              sync.Mutex
	FakeName        string
	FakeSessionName string
	Running         bool
	Calls           []Call

	// FakeSessionConfig is returned by SessionConfig(). Set it per-test
	// to control the config fingerprint for reconciliation tests.
	FakeSessionConfig session.Config

	// FakePeekOutput is returned by Peek(). Set it per-test.
	FakePeekOutput string

	// Set these to inject errors per-test.
	StartErr  error
	StopErr   error
	AttachErr error
	NudgeErr  error
	PeekErr   error
}

// NewFake returns a ready-to-use [Fake] with the given identity.
func NewFake(name, sessionName string) *Fake {
	return &Fake{FakeName: name, FakeSessionName: sessionName}
}

// Name records the call and returns FakeName.
func (f *Fake) Name() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Name", Name: f.FakeName})
	return f.FakeName
}

// SessionName records the call and returns FakeSessionName.
func (f *Fake) SessionName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "SessionName", Name: f.FakeName})
	return f.FakeSessionName
}

// IsRunning records the call and returns the Running field.
func (f *Fake) IsRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "IsRunning", Name: f.FakeName})
	return f.Running
}

// Start records the call. Returns StartErr if set; otherwise sets Running=true.
func (f *Fake) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Start", Name: f.FakeName})
	if f.StartErr != nil {
		return f.StartErr
	}
	f.Running = true
	return nil
}

// Stop records the call. Returns StopErr if set; otherwise sets Running=false.
func (f *Fake) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Stop", Name: f.FakeName})
	if f.StopErr != nil {
		return f.StopErr
	}
	f.Running = false
	return nil
}

// Attach records the call and returns AttachErr (nil if not set).
func (f *Fake) Attach() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Attach", Name: f.FakeName})
	return f.AttachErr
}

// Nudge records the call and returns NudgeErr (nil if not set).
func (f *Fake) Nudge(message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Nudge", Name: f.FakeName, Message: message})
	return f.NudgeErr
}

// Peek records the call and returns FakePeekOutput or PeekErr.
func (f *Fake) Peek(lines int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "Peek", Name: f.FakeName, Lines: lines})
	if f.PeekErr != nil {
		return "", f.PeekErr
	}
	return f.FakePeekOutput, nil
}

// SessionConfig records the call and returns FakeSessionConfig.
func (f *Fake) SessionConfig() session.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, Call{Method: "SessionConfig", Name: f.FakeName})
	return f.FakeSessionConfig
}
