// Package events provides tier-0 observability for Gas City.
//
// Events are simple, synchronous, append-only records of what happened.
// The recorder writes JSON lines to .gc/events.jsonl; the reader scans
// them back. Recording is best-effort: errors are logged to stderr but
// never returned to callers.
package events

import "time"

// Event type constants. Only types we actually emit today.
const (
	AgentStarted      = "agent.started"
	AgentStopped      = "agent.stopped"
	BeadCreated       = "bead.created"
	BeadClaimed       = "bead.claimed"
	BeadUnclaimed     = "bead.unclaimed"
	BeadClosed        = "bead.closed"
	BeadUpdated       = "bead.updated"
	MailSent          = "mail.sent"
	MailRead          = "mail.read"
	AgentDraining     = "agent.draining"
	AgentUndrained    = "agent.undrained"
	MoleculeCreated   = "molecule.created"
	StepCompleted     = "step.completed"
	AgentQuarantined  = "agent.quarantined"
	AgentSuspended    = "agent.suspended"
	ControllerStarted = "controller.started"
	ControllerStopped = "controller.stopped"
)

// Event is a single recorded occurrence in the system.
type Event struct {
	Seq     uint64    `json:"seq"`
	Type    string    `json:"type"`
	Ts      time.Time `json:"ts"`
	Actor   string    `json:"actor"`
	Subject string    `json:"subject,omitempty"`
	Message string    `json:"message,omitempty"`
}

// Recorder records events. Safe for concurrent use. Best-effort.
type Recorder interface {
	Record(e Event)
}

// Discard silently drops all events.
var Discard Recorder = discardRecorder{}

type discardRecorder struct{}

func (discardRecorder) Record(Event) {}
