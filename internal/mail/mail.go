// Package mail defines the pluggable mail provider interface for Gas City.
// The primary extension point is the exec script protocol (see
// internal/mail/exec); the Go interface exists for code organization and
// testability.
package mail //nolint:revive // internal package, always imported qualified

import (
	"errors"
	"time"
)

// ErrAlreadyArchived is returned by [Provider.Archive] when the message
// has already been archived. CLI code uses this to print a distinct message.
var ErrAlreadyArchived = errors.New("already archived")

// Message represents a mail message between agents or humans.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Provider is the internal interface for mail backends. Implementations
// include beadmail (built-in default backed by beads.Store) and exec
// (user-supplied script via fork/exec).
type Provider interface {
	// Send creates a message from sender to recipient. Returns the
	// created message with its assigned ID and timestamp.
	Send(from, to, body string) (Message, error)

	// Inbox returns all unread messages for the recipient.
	Inbox(recipient string) ([]Message, error)

	// Read retrieves a message by ID and marks it as read.
	Read(id string) (Message, error)

	// Archive closes a message without reading it.
	Archive(id string) error

	// Check returns unread messages for the recipient (used for hook
	// injection). Unlike Inbox, Check does not mark messages as read.
	Check(recipient string) ([]Message, error)
}
