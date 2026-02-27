package exec //nolint:revive // internal package, always imported with alias

import (
	"encoding/json"

	"github.com/steveyegge/gascity/internal/mail"
)

// sendInput is the JSON wire format sent to the script's stdin on Send.
type sendInput struct {
	From string `json:"from"`
	Body string `json:"body"`
}

// marshalSendInput encodes the send payload as JSON.
func marshalSendInput(from, body string) ([]byte, error) {
	return json.Marshal(sendInput{From: from, Body: body})
}

// unmarshalMessage decodes a single Message from JSON.
func unmarshalMessage(data string) (mail.Message, error) {
	var m mail.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return mail.Message{}, err
	}
	return m, nil
}

// unmarshalMessages decodes a JSON array of Messages.
func unmarshalMessages(data string) ([]mail.Message, error) {
	var msgs []mail.Message
	if err := json.Unmarshal([]byte(data), &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}
