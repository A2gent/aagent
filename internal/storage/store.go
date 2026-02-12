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
	Title     string
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

// RecurringJob represents a scheduled recurring job
type RecurringJob struct {
	ID            string
	Name          string
	ScheduleHuman string // Human-readable schedule (e.g., "every Monday at 9am")
	ScheduleCron  string // Parsed cron expression (e.g., "0 9 * * 1")
	TaskPrompt    string // The actual task instructions for the agent
	Enabled       bool
	LastRunAt     *time.Time
	NextRunAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// JobExecution represents a single execution of a recurring job
type JobExecution struct {
	ID         string
	JobID      string
	SessionID  string // Reference to the agent session created for this execution
	Status     string // "running", "success", "failed"
	Output     string // Summary of what the agent did
	Error      string // Error message if failed
	StartedAt  time.Time
	FinishedAt *time.Time
}

// Store defines the interface for session storage
type Store interface {
	// Session operations
	SaveSession(sess *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() ([]*Session, error)
	DeleteSession(id string) error

	// Recurring job operations
	SaveJob(job *RecurringJob) error
	GetJob(id string) (*RecurringJob, error)
	ListJobs() ([]*RecurringJob, error)
	DeleteJob(id string) error
	GetDueJobs(now time.Time) ([]*RecurringJob, error)

	// Job execution operations
	SaveJobExecution(exec *JobExecution) error
	GetJobExecution(id string) (*JobExecution, error)
	ListJobExecutions(jobID string, limit int) ([]*JobExecution, error)

	// Close closes the store
	Close() error
}
