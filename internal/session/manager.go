package session

import (
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
