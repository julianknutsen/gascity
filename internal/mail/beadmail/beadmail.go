// Package beadmail implements [mail.Provider] backed by [beads.Store].
// This is the built-in default mail backend — messages are stored as beads
// with Type="message". No subprocess needed.
package beadmail

import (
	"fmt"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/mail"
)

// Provider implements [mail.Provider] using [beads.Store] as the backend.
type Provider struct {
	store beads.Store
}

// New returns a beadmail provider backed by the given store.
func New(store beads.Store) *Provider {
	return &Provider{store: store}
}

// Send creates a message bead.
func (p *Provider) Send(from, to, body string) (mail.Message, error) {
	b, err := p.store.Create(beads.Bead{
		Title:    body,
		Type:     "message",
		Assignee: to,
		From:     from,
	})
	if err != nil {
		return mail.Message{}, fmt.Errorf("beadmail send: %w", err)
	}
	return beadToMessage(b), nil
}

// Inbox returns all unread messages for the recipient.
func (p *Provider) Inbox(recipient string) ([]mail.Message, error) {
	return p.filterMessages(recipient)
}

// Read retrieves a message by ID and marks it as read (closes the bead).
func (p *Provider) Read(id string) (mail.Message, error) {
	b, err := p.store.Get(id)
	if err != nil {
		return mail.Message{}, fmt.Errorf("beadmail read: %w", err)
	}
	if b.Status != "closed" {
		if err := p.store.Close(id); err != nil {
			return mail.Message{}, fmt.Errorf("beadmail read: marking as read: %w", err)
		}
	}
	return beadToMessage(b), nil
}

// Archive closes a message bead without reading it.
func (p *Provider) Archive(id string) error {
	b, err := p.store.Get(id)
	if err != nil {
		return fmt.Errorf("beadmail archive: %w", err)
	}
	if b.Type != "message" {
		return fmt.Errorf("beadmail archive: bead %s is not a message", id)
	}
	if b.Status == "closed" {
		return mail.ErrAlreadyArchived
	}
	if err := p.store.Close(id); err != nil {
		return fmt.Errorf("beadmail archive: %w", err)
	}
	return nil
}

// Check returns unread messages for the recipient without marking them read.
func (p *Provider) Check(recipient string) ([]mail.Message, error) {
	return p.filterMessages(recipient)
}

// filterMessages returns open message beads assigned to the recipient.
func (p *Provider) filterMessages(recipient string) ([]mail.Message, error) {
	all, err := p.store.List()
	if err != nil {
		return nil, fmt.Errorf("beadmail: listing beads: %w", err)
	}
	var msgs []mail.Message
	for _, b := range all {
		if b.Type == "message" && b.Status == "open" && b.Assignee == recipient {
			msgs = append(msgs, beadToMessage(b))
		}
	}
	return msgs, nil
}

// beadToMessage converts a bead to a mail.Message.
func beadToMessage(b beads.Bead) mail.Message {
	return mail.Message{
		ID:        b.ID,
		From:      b.From,
		To:        b.Assignee,
		Body:      b.Title,
		CreatedAt: b.CreatedAt,
	}
}
