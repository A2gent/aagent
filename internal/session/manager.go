package session

import (
	"encoding/json"
	"fmt"

	"github.com/gratheon/aagent/internal/storage"
)

// Manager manages sessions
type Manager struct {
	store storage.Store
}

// NewManager creates a new session manager
func NewManager(store storage.Store) *Manager {
	return &Manager{store: store}
}

// Create creates a new session
func (m *Manager) Create(agentID string) (*Session, error) {
	sess := New(agentID)
	if err := m.store.SaveSession(sess.ToStorage()); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}
	return sess, nil
}

// CreateWithParent creates a new sub-session
func (m *Manager) CreateWithParent(agentID, parentID string) (*Session, error) {
	sess := NewWithParent(agentID, parentID)
	if err := m.store.SaveSession(sess.ToStorage()); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}
	return sess, nil
}

// CreateWithJob creates a new session associated with a recurring job
func (m *Manager) CreateWithJob(agentID, jobID string) (*Session, error) {
	sess := NewWithJob(agentID, jobID)
	if err := m.store.SaveSession(sess.ToStorage()); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}
	return sess, nil
}

// Get retrieves a session by ID
func (m *Manager) Get(id string) (*Session, error) {
	ss, err := m.store.GetSession(id)
	if err != nil {
		return nil, err
	}
	return FromStorage(ss), nil
}

// Save saves a session
func (m *Manager) Save(sess *Session) error {
	return m.store.SaveSession(sess.ToStorage())
}

// List lists all sessions
func (m *Manager) List() ([]*Session, error) {
	stored, err := m.store.ListSessions()
	if err != nil {
		return nil, err
	}

	sessions := make([]*Session, len(stored))
	for i, ss := range stored {
		sessions[i] = FromStorage(ss)
	}
	return sessions, nil
}

// Delete deletes a session
func (m *Manager) Delete(id string) error {
	return m.store.DeleteSession(id)
}

// QuestionData represents a question asked to the user
type QuestionData struct {
	Question string           `json:"question"`
	Header   string           `json:"header"`
	Options  []QuestionOption `json:"options"`
	Multiple bool             `json:"multiple"`
	Custom   bool             `json:"custom"`
}

// QuestionOption represents a single answer choice
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// SetPendingQuestion stores a pending question in session metadata
func (m *Manager) SetPendingQuestion(sessionID string, data *QuestionData) error {
	sess, err := m.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if sess.Metadata == nil {
		sess.Metadata = make(map[string]interface{})
	}

	sess.Metadata["pending_question"] = data
	return m.Save(sess)
}

// GetPendingQuestion retrieves pending question from session metadata
func (m *Manager) GetPendingQuestion(sessionID string) (*QuestionData, error) {
	sess, err := m.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if sess.Status != StatusInputRequired {
		return nil, nil
	}

	data, ok := sess.Metadata["pending_question"]
	if !ok {
		return nil, nil
	}

	// Convert map to QuestionData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal question data: %w", err)
	}

	var question QuestionData
	if err := json.Unmarshal(jsonBytes, &question); err != nil {
		return nil, fmt.Errorf("failed to unmarshal question data: %w", err)
	}

	return &question, nil
}

// AnswerQuestion handles user's answer to a pending question
func (m *Manager) AnswerQuestion(sessionID string, answer string) error {
	sess, err := m.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if sess.Status != StatusInputRequired {
		return fmt.Errorf("session is not waiting for input")
	}

	// Remove pending question
	delete(sess.Metadata, "pending_question")

	// Add user answer as a message
	sess.AddMessage(Message{
		Role:    "user",
		Content: answer,
	})

	// Resume session
	sess.SetStatus(StatusRunning)

	return m.Save(sess)
}

// SetSessionStatus updates session status (used by question tool)
func (m *Manager) SetSessionStatus(sessionID string, status string) error {
	sess, err := m.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	sess.SetStatus(Status(status))
	return m.Save(sess)
}
