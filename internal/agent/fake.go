package agent

// Call records a method invocation on [Fake].
type Call struct {
	Method string // "Name", "SessionName", "IsRunning", "Start", "Stop", or "Attach"
	Name   string // agent name at time of call
}

// Fake is a test double for [Agent] with spy and configurable errors.
// Set the exported error fields to inject failures per-test.
type Fake struct {
	FakeName        string
	FakeSessionName string
	Running         bool
	Calls           []Call

	// Set these to inject errors per-test.
	StartErr  error
	StopErr   error
	AttachErr error
}

// NewFake returns a ready-to-use [Fake] with the given identity.
func NewFake(name, sessionName string) *Fake {
	return &Fake{FakeName: name, FakeSessionName: sessionName}
}

// Name records the call and returns FakeName.
func (f *Fake) Name() string {
	f.Calls = append(f.Calls, Call{Method: "Name", Name: f.FakeName})
	return f.FakeName
}

// SessionName records the call and returns FakeSessionName.
func (f *Fake) SessionName() string {
	f.Calls = append(f.Calls, Call{Method: "SessionName", Name: f.FakeName})
	return f.FakeSessionName
}

// IsRunning records the call and returns the Running field.
func (f *Fake) IsRunning() bool {
	f.Calls = append(f.Calls, Call{Method: "IsRunning", Name: f.FakeName})
	return f.Running
}

// Start records the call. Returns StartErr if set; otherwise sets Running=true.
func (f *Fake) Start() error {
	f.Calls = append(f.Calls, Call{Method: "Start", Name: f.FakeName})
	if f.StartErr != nil {
		return f.StartErr
	}
	f.Running = true
	return nil
}

// Stop records the call. Returns StopErr if set; otherwise sets Running=false.
func (f *Fake) Stop() error {
	f.Calls = append(f.Calls, Call{Method: "Stop", Name: f.FakeName})
	if f.StopErr != nil {
		return f.StopErr
	}
	f.Running = false
	return nil
}

// Attach records the call and returns AttachErr (nil if not set).
func (f *Fake) Attach() error {
	f.Calls = append(f.Calls, Call{Method: "Attach", Name: f.FakeName})
	return f.AttachErr
}
