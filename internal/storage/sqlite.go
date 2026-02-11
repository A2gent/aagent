package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(dataPath string) (*SQLiteStore, error) {
	dbPath := filepath.Join(dataPath, "aagent.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// migrate runs database migrations
func (s *SQLiteStore) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			parent_id TEXT,
			title TEXT DEFAULT '',
			status TEXT NOT NULL,
			metadata TEXT,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_calls TEXT,
			tool_results TEXT,
			timestamp TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent_id ON sessions(parent_id)`,
		// Migration to add title column if it doesn't exist
		`ALTER TABLE sessions ADD COLUMN title TEXT DEFAULT ''`,
	}

	for _, m := range migrations {
		// Ignore errors for ALTER TABLE (column may already exist)
		_, err := s.db.Exec(m)
		if err != nil && m[:5] != "ALTER" {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// SaveSession saves a session to the database
func (s *SQLiteStore) SaveSession(sess *Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	metadata, _ := json.Marshal(sess.Metadata)

	// Upsert session
	_, err = tx.Exec(`
		INSERT INTO sessions (id, agent_id, parent_id, title, status, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			status = excluded.status,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`, sess.ID, sess.AgentID, sess.ParentID, sess.Title, sess.Status, metadata, sess.CreatedAt, sess.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Delete existing messages and re-insert (simple approach for now)
	_, err = tx.Exec("DELETE FROM messages WHERE session_id = ?", sess.ID)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	// Insert messages
	for _, msg := range sess.Messages {
		_, err = tx.Exec(`
			INSERT INTO messages (id, session_id, role, content, tool_calls, tool_results, timestamp)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, msg.ID, sess.ID, msg.Role, msg.Content, msg.ToolCalls, msg.ToolResults, msg.Timestamp)
		if err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
	}

	return tx.Commit()
}

// GetSession retrieves a session by ID
func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	var sess Session
	var metadata sql.NullString
	var parentID sql.NullString
	var title sql.NullString

	err := s.db.QueryRow(`
		SELECT id, agent_id, parent_id, title, status, metadata, created_at, updated_at
		FROM sessions WHERE id = ?
	`, id).Scan(&sess.ID, &sess.AgentID, &parentID, &title, &sess.Status, &metadata, &sess.CreatedAt, &sess.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		sess.ParentID = &parentID.String
	}
	if title.Valid {
		sess.Title = title.String
	}
	if metadata.Valid {
		json.Unmarshal([]byte(metadata.String), &sess.Metadata)
	}

	// Load messages
	rows, err := s.db.Query(`
		SELECT id, role, content, tool_calls, tool_results, timestamp
		FROM messages WHERE session_id = ? ORDER BY timestamp
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var msg Message
		var toolCalls, toolResults sql.NullString

		err := rows.Scan(&msg.ID, &msg.Role, &msg.Content, &toolCalls, &toolResults, &msg.Timestamp)
		if err != nil {
			return nil, err
		}

		if toolCalls.Valid {
			msg.ToolCalls = json.RawMessage(toolCalls.String)
		}
		if toolResults.Valid {
			msg.ToolResults = json.RawMessage(toolResults.String)
		}

		sess.Messages = append(sess.Messages, msg)
	}

	return &sess, nil
}

// ListSessions lists all sessions
func (s *SQLiteStore) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, agent_id, parent_id, title, status, created_at, updated_at
		FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var sess Session
		var parentID sql.NullString
		var title sql.NullString

		err := rows.Scan(&sess.ID, &sess.AgentID, &parentID, &title, &sess.Status, &sess.CreatedAt, &sess.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if parentID.Valid {
			sess.ParentID = &parentID.String
		}
		if title.Valid {
			sess.Title = title.String
		}

		sessions = append(sessions, &sess)
	}

	return sessions, nil
}

// DeleteSession deletes a session
func (s *SQLiteStore) DeleteSession(id string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Ensure SQLiteStore implements Store
var _ Store = (*SQLiteStore)(nil)
