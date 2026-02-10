package storage

import (
	"encoding/json"
	"time"
)

// Session represents a stored session (storage layer copy to avoid import cycle)
type Session struct {
	ID        string
	AgentID   string
	ParentID  *string
	Status    string
	Messages  []Message
	Metadata  map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message represents a stored message
type Message struct {
	ID          string
	Role        string
	Content     string
	ToolCalls   json.RawMessage
	ToolResults json.RawMessage
	Timestamp   time.Time
}

// Store defines the interface for session storage
type Store interface {
	// Session operations
	SaveSession(sess *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	DeleteSession(id string) error

	// Close closes the store
	Close() error
}
