package mail //nolint:revive // internal package, always imported qualified

import (
	"fmt"
	"sync"
	"time"
)

// fakeMsg tracks a message with its read/archived status.
type fakeMsg struct {
	msg      Message
	read     bool
	archived bool
}

// Fake is an in-memory mail provider for testing. It records messages and
// supports all Provider operations. Safe for concurrent use.
//
// When broken is true (via [NewFailFake]), all operations return errors.
type Fake struct {
	mu       sync.Mutex
	messages []fakeMsg
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
	f.messages = append(f.messages, fakeMsg{msg: m})
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
	for _, fm := range f.messages {
		if fm.msg.To == recipient && !fm.read && !fm.archived {
			result = append(result, fm.msg)
		}
	}
	return result, nil
}

// Read returns a message by ID and marks it as read.
func (f *Fake) Read(id string) (Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return Message{}, fmt.Errorf("mail provider unavailable")
	}
	for i := range f.messages {
		if f.messages[i].msg.ID == id {
			f.messages[i].read = true
			return f.messages[i].msg, nil
		}
	}
	return Message{}, fmt.Errorf("message %q not found", id)
}

// Archive closes a message without reading it.
func (f *Fake) Archive(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.broken {
		return fmt.Errorf("mail provider unavailable")
	}
	for i := range f.messages {
		if f.messages[i].msg.ID == id {
			if f.messages[i].archived {
				return ErrAlreadyArchived
			}
			f.messages[i].archived = true
			return nil
		}
	}
	return fmt.Errorf("message %q not found", id)
}

// Check returns unread messages for the recipient without marking them read.
func (f *Fake) Check(recipient string) ([]Message, error) {
	return f.Inbox(recipient)
}

// Messages returns a copy of all messages currently stored, regardless of status.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Message, len(f.messages))
	for i, fm := range f.messages {
		result[i] = fm.msg
	}
	return result
}
