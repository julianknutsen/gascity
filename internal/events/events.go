// Package events provides tier-0 observability for Gas City.
//
// Events are simple, synchronous, append-only records of what happened.
// The recorder writes JSON lines to .gc/events.jsonl; the reader scans
// them back. Recording is best-effort: errors are logged to stderr but
// never returned to callers.
package events

import (
	"context"
	"encoding/json"
	"time"
)

// Event type constants. Only types we actually emit today.
const (
	AgentStarted        = "agent.started"
	AgentStopped        = "agent.stopped"
	AgentCrashed        = "agent.crashed"
	BeadCreated         = "bead.created"
	BeadClosed          = "bead.closed"
	BeadUpdated         = "bead.updated"
	MailSent            = "mail.sent"
	MailRead            = "mail.read"
	AgentDraining       = "agent.draining"
	AgentUndrained      = "agent.undrained"
	AgentQuarantined    = "agent.quarantined"
	AgentIdleKilled     = "agent.idle_killed"
	AgentSuspended      = "agent.suspended"
	ConvoyCreated       = "convoy.created"
	ConvoyClosed        = "convoy.closed"
	ControllerStarted   = "controller.started"
	ControllerStopped   = "controller.stopped"
	CitySuspended       = "city.suspended"
	CityResumed         = "city.resumed"
	AutomationFired     = "automation.fired"
	AutomationCompleted = "automation.completed"
	AutomationFailed    = "automation.failed"
)

// Event is a single recorded occurrence in the system.
type Event struct {
	Seq     uint64          `json:"seq"`
	Type    string          `json:"type"`
	Ts      time.Time       `json:"ts"`
	Actor   string          `json:"actor"`
	Subject string          `json:"subject,omitempty"`
	Message string          `json:"message,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Recorder records events. Safe for concurrent use. Best-effort.
// This sub-interface is used by callers that only need to write events.
type Recorder interface {
	Record(e Event)
}

// Provider is the full interface for event backends. It embeds Recorder
// for writing and adds reading, querying, and watching. Implementations
// include FileRecorder (built-in JSONL file) and exec (user-supplied
// script via fork/exec).
type Provider interface {
	Recorder

	// List returns events matching the filter.
	List(filter Filter) ([]Event, error)

	// LatestSeq returns the highest sequence number, or 0 if empty.
	LatestSeq() (uint64, error)

	// Watch returns a Watcher that yields events with Seq > afterSeq.
	// The watcher blocks on Next() until an event arrives or ctx is
	// canceled. Callers must call Close() when done.
	Watch(ctx context.Context, afterSeq uint64) (Watcher, error)

	// Close releases any resources held by the provider.
	Close() error
}

// Watcher yields events one at a time. Created by [Provider.Watch].
// Callers must call Close() when done watching.
type Watcher interface {
	// Next blocks until the next event is available, the context is
	// canceled, or the watcher is closed. Returns the event or an error.
	Next() (Event, error)

	// Close stops the watcher and releases resources.
	Close() error
}

// Discard silently drops all events.
var Discard Recorder = discardRecorder{}

type discardRecorder struct{}

func (discardRecorder) Record(Event) {}
