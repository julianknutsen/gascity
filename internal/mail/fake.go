package mail //nolint:revive // internal package, always imported qualified

import (
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory mail provider for testing. It records messages and
// supports all Provider operations. Safe for concurrent use.
//
// When broken is true (via [NewFailFake]), all operations return errors.
type Fake struct {
	mu       sync.Mutex
	messages []Message
	seq      int
	broken   bool
}

// NewFake returns a ready-to-use in-memory mail provider.
func NewFake() *Fake {
	return &Fake{}
}

// NewFailFake returns a mail provider where all operations return errors.
// Useful for testing error paths.
func NewFailFake() *Fake {
	return &Fake{broken: true}
}

// Send creates a message in memory.
func (f *Fake) Send(from, to, body string) (Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return Message{}, fmt.Errorf("mail provider unavailable")
	}
	f.seq++
	m := Message{
		ID:        fmt.Sprintf("fake-%d", f.seq),
		From:      from,
		To:        to,
		Body:      body,
		CreatedAt: time.Now(),
	}
	f.messages = append(f.messages, m)
	return m, nil
}

// Inbox returns unread messages for the recipient.
func (f *Fake) Inbox(recipient string) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return nil, fmt.Errorf("mail provider unavailable")
	}
	var result []Message
	for _, m := range f.messages {
		if m.To == recipient {
			result = append(result, m)
		}
	}
	return result, nil
}

// Read returns a message by ID and removes it from the inbox.
func (f *Fake) Read(id string) (Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return Message{}, fmt.Errorf("mail provider unavailable")
	}
	for i, m := range f.messages {
		if m.ID == id {
			f.messages = append(f.messages[:i], f.messages[i+1:]...)
			return m, nil
		}
	}
	return Message{}, fmt.Errorf("message %q not found", id)
}

// Archive removes a message without reading it.
func (f *Fake) Archive(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return fmt.Errorf("mail provider unavailable")
	}
	for i, m := range f.messages {
		if m.ID == id {
			f.messages = append(f.messages[:i], f.messages[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("message %q not found", id)
}

// Check returns unread messages for the recipient without marking them read.
func (f *Fake) Check(recipient string) ([]Message, error) {
	return f.Inbox(recipient) // same behavior for fake
}

// Messages returns a copy of all messages currently stored.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Message, len(f.messages))
	copy(result, f.messages)
	return result
}
