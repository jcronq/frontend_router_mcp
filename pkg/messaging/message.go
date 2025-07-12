package messaging

import (
	"encoding/json"
	"time"
	
	"github.com/google/uuid"
)

// ResponseSender defines an interface for sending responses back to clients
type ResponseSender interface {
	SendResponse(msg *Message) error
}

// Message represents a message passed between the WebSocket server and MCP server
type Message struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	ClientID       string          `json:"client_id,omitempty"`
	ResponseSender ResponseSender  `json:"-"` // Not serialized
}

// NewMessage creates a new message with the given type and payload
func NewMessage(messageType string, payload interface{}) (*Message, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:        GenerateID(),
		Type:      messageType,
		Timestamp: time.Now().UTC(),
		Payload:   payloadBytes,
	}, nil
}

// ParsePayload parses the message payload into the provided interface
func (m *Message) ParsePayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// GenerateID creates a unique ID for a message
func GenerateID() string {
	return uuid.New().String()
}

// RandString generates a random string of length n
func RandString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
