package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/gratheon/aagent/internal/agent"
	"github.com/gratheon/aagent/internal/config"
	"github.com/gratheon/aagent/internal/llm"
	"github.com/gratheon/aagent/internal/logging"
	"github.com/gratheon/aagent/internal/session"
	"github.com/gratheon/aagent/internal/storage"
	"github.com/gratheon/aagent/internal/tools"
	"github.com/robfig/cron/v3"
)

// Server represents the HTTP API server
type Server struct {
	config         *config.Config
	llmClient      llm.Client
	toolManager    *tools.Manager
	sessionManager *session.Manager
	store          storage.Store
	router         chi.Router
	port           int
}

// NewServer creates a new HTTP server instance
func NewServer(
	cfg *config.Config,
	llmClient llm.Client,
	toolManager *tools.Manager,
	sessionManager *session.Manager,
	store storage.Store,
	port int,
) *Server {
	s := &Server{
		config:         cfg,
		llmClient:      llmClient,
		toolManager:    toolManager,
		sessionManager: sessionManager,
		store:          store,
		port:           port,
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Middleware (no logger to avoid polluting TUI output)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Minute))

	// CORS configuration - allow all origins for flexibility
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false, // Must be false when AllowedOrigins is "*"
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", s.handleHealth)

	// Session endpoints
	r.Route("/sessions", func(r chi.Router) {
		r.Get("/", s.handleListSessions)
		r.Post("/", s.handleCreateSession)
		r.Get("/{sessionID}", s.handleGetSession)
		r.Delete("/{sessionID}", s.handleDeleteSession)
		r.Post("/{sessionID}/chat", s.handleChat)
	})

	// Recurring jobs endpoints
	r.Route("/jobs", func(r chi.Router) {
		r.Get("/", s.handleListJobs)
		r.Post("/", s.handleCreateJob)
		r.Get("/{jobID}", s.handleGetJob)
		r.Put("/{jobID}", s.handleUpdateJob)
		r.Delete("/{jobID}", s.handleDeleteJob)
		r.Post("/{jobID}/run", s.handleRunJobNow)
		r.Get("/{jobID}/executions", s.handleListJobExecutions)
		r.Get("/{jobID}/sessions", s.handleListJobSessions)
	})

	s.router = r
}

// Run starts the HTTP server
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	logging.Info("Starting HTTP server on %s", addr)
	fmt.Printf("HTTP API server running on http://0.0.0.0:%d (accessible from any host)\n", s.port)

	server := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		logging.Info("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}

// --- Request/Response types ---

// CreateSessionRequest represents a request to create a new session
type CreateSessionRequest struct {
	AgentID string `json:"agent_id"`
	Task    string `json:"task,omitempty"`
}

// CreateSessionResponse represents a response after creating a session
type CreateSessionResponse struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionResponse represents a session with its messages
type SessionResponse struct {
	ID        string            `json:"id"`
	AgentID   string            `json:"agent_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Title     string            `json:"title"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Messages  []MessageResponse `json:"messages"`
}

// MessageResponse represents a message in a session
type MessageResponse struct {
	Role        string               `json:"role"`
	Content     string               `json:"content"`
	ToolCalls   []ToolCallResponse   `json:"tool_calls,omitempty"`
	ToolResults []ToolResultResponse `json:"tool_results,omitempty"`
	Timestamp   time.Time            `json:"timestamp"`
}

// ToolCallResponse represents a tool call
type ToolCallResponse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResultResponse represents a tool result
type ToolResultResponse struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// ChatRequest represents a chat message request
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse represents a chat response
type ChatResponse struct {
	Content  string            `json:"content"`
	Messages []MessageResponse `json:"messages"`
	Status   string            `json:"status"`
	Usage    UsageResponse     `json:"usage"`
}

// UsageResponse represents token usage
type UsageResponse struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// SessionListItem represents a session in the list
type SessionListItem struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// --- Recurring Jobs Request/Response types ---

// CreateJobRequest represents a request to create a recurring job
type CreateJobRequest struct {
	Name         string `json:"name"`
	ScheduleText string `json:"schedule_text"` // Natural language schedule
	TaskPrompt   string `json:"task_prompt"`
	Enabled      bool   `json:"enabled"`
}

// UpdateJobRequest represents a request to update a recurring job
type UpdateJobRequest struct {
	Name         string `json:"name"`
	ScheduleText string `json:"schedule_text"`
	TaskPrompt   string `json:"task_prompt"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

// JobResponse represents a recurring job response
type JobResponse struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	ScheduleHuman string     `json:"schedule_human"`
	ScheduleCron  string     `json:"schedule_cron"`
	TaskPrompt    string     `json:"task_prompt"`
	Enabled       bool       `json:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// JobExecutionResponse represents a job execution response
type JobExecutionResponse struct {
	ID         string     `json:"id"`
	JobID      string     `json:"job_id"`
	SessionID  string     `json:"session_id,omitempty"`
	Status     string     `json:"status"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessionManager.List()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list sessions: "+err.Error())
		return
	}

	items := make([]SessionListItem, len(sessions))
	for i, sess := range sessions {
		items[i] = SessionListItem{
			ID:        sess.ID,
			AgentID:   sess.AgentID,
			Title:     sess.Title,
			Status:    string(sess.Status),
			CreatedAt: sess.CreatedAt,
			UpdatedAt: sess.UpdatedAt,
		}
	}

	s.jsonResponse(w, http.StatusOK, items)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.AgentID == "" {
		req.AgentID = "build" // Default agent
	}

	sess, err := s.sessionManager.Create(req.AgentID)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to create session: "+err.Error())
		return
	}

	// If an initial task is provided, add it as the first message
	if req.Task != "" {
		sess.AddUserMessage(req.Task)
		if err := s.sessionManager.Save(sess); err != nil {
			logging.Error("Failed to save session with initial task: %v", err)
		}
	}

	logging.LogSession("created", sess.ID, fmt.Sprintf("agent=%s via HTTP", req.AgentID))

	s.jsonResponse(w, http.StatusCreated, CreateSessionResponse{
		ID:        sess.ID,
		AgentID:   sess.AgentID,
		Status:    string(sess.Status),
		CreatedAt: sess.CreatedAt,
	})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	sess, err := s.sessionManager.Get(sessionID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Session not found: "+err.Error())
		return
	}

	resp := s.sessionToResponse(sess)
	s.jsonResponse(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	if err := s.sessionManager.Delete(sessionID); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to delete session: "+err.Error())
		return
	}

	logging.LogSession("deleted", sessionID, "via HTTP")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Message == "" {
		s.errorResponse(w, http.StatusBadRequest, "Message is required")
		return
	}

	// Get the session
	sess, err := s.sessionManager.Get(sessionID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Session not found: "+err.Error())
		return
	}

	// Add user message to session
	sess.AddUserMessage(req.Message)

	// Create agent config
	agentConfig := agent.Config{
		Name:        sess.AgentID,
		Model:       s.config.DefaultModel,
		MaxSteps:    s.config.MaxSteps,
		Temperature: s.config.Temperature,
	}

	// Create agent instance
	ag := agent.New(agentConfig, s.llmClient, s.toolManager, s.sessionManager)

	// Run the agent (this is synchronous for now)
	ctx := r.Context()
	content, usage, err := ag.Run(ctx, sess, req.Message)
	if err != nil {
		// Save session state even on error
		s.sessionManager.Save(sess)
		s.errorResponse(w, http.StatusInternalServerError, "Agent error: "+err.Error())
		return
	}

	// Build response with updated messages
	resp := ChatResponse{
		Content:  content,
		Messages: s.messagesToResponse(sess.Messages),
		Status:   string(sess.Status),
		Usage: UsageResponse{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
		},
	}

	s.jsonResponse(w, http.StatusOK, resp)
}

// --- Recurring Jobs Handlers ---

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list jobs: "+err.Error())
		return
	}

	resp := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		resp[i] = s.jobToResponse(job)
	}

	s.jsonResponse(w, http.StatusOK, resp)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		s.errorResponse(w, http.StatusBadRequest, "Name is required")
		return
	}
	if req.ScheduleText == "" {
		s.errorResponse(w, http.StatusBadRequest, "Schedule text is required")
		return
	}
	if req.TaskPrompt == "" {
		s.errorResponse(w, http.StatusBadRequest, "Task prompt is required")
		return
	}

	// Parse natural language schedule to cron using the agent
	cronExpr, err := s.parseScheduleToCron(r.Context(), req.ScheduleText)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Failed to parse schedule: "+err.Error())
		return
	}

	now := time.Now()
	job := &storage.RecurringJob{
		ID:            uuid.New().String(),
		Name:          req.Name,
		ScheduleHuman: req.ScheduleText,
		ScheduleCron:  cronExpr,
		TaskPrompt:    req.TaskPrompt,
		Enabled:       req.Enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Calculate next run time
	nextRun, err := s.calculateNextRun(cronExpr, now)
	if err == nil {
		job.NextRunAt = &nextRun
	}

	if err := s.store.SaveJob(job); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to save job: "+err.Error())
		return
	}

	logging.Info("Created recurring job: %s (%s)", job.Name, job.ID)
	s.jsonResponse(w, http.StatusCreated, s.jobToResponse(job))
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(jobID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Job not found: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusOK, s.jobToResponse(job))
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(jobID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Job not found: "+err.Error())
		return
	}

	var req UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Update fields if provided
	if req.Name != "" {
		job.Name = req.Name
	}
	if req.TaskPrompt != "" {
		job.TaskPrompt = req.TaskPrompt
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}

	// Re-parse schedule if changed
	if req.ScheduleText != "" && req.ScheduleText != job.ScheduleHuman {
		cronExpr, err := s.parseScheduleToCron(r.Context(), req.ScheduleText)
		if err != nil {
			s.errorResponse(w, http.StatusBadRequest, "Failed to parse schedule: "+err.Error())
			return
		}
		job.ScheduleHuman = req.ScheduleText
		job.ScheduleCron = cronExpr

		// Recalculate next run time
		nextRun, err := s.calculateNextRun(cronExpr, time.Now())
		if err == nil {
			job.NextRunAt = &nextRun
		}
	}

	job.UpdatedAt = time.Now()

	if err := s.store.SaveJob(job); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to update job: "+err.Error())
		return
	}

	logging.Info("Updated recurring job: %s (%s)", job.Name, job.ID)
	s.jsonResponse(w, http.StatusOK, s.jobToResponse(job))
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	if err := s.store.DeleteJob(jobID); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to delete job: "+err.Error())
		return
	}

	logging.Info("Deleted recurring job: %s", jobID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunJobNow(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	job, err := s.store.GetJob(jobID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Job not found: "+err.Error())
		return
	}

	// Execute the job immediately (in a goroutine so we don't block)
	exec, err := s.executeJob(r.Context(), job)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to execute job: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusOK, s.executionToResponse(exec))
}

func (s *Server) handleListJobExecutions(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	// Get limit from query params, default to 20
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	executions, err := s.store.ListJobExecutions(jobID, limit)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list executions: "+err.Error())
		return
	}

	resp := make([]JobExecutionResponse, len(executions))
	for i, exec := range executions {
		resp[i] = s.executionToResponse(exec)
	}

	s.jsonResponse(w, http.StatusOK, resp)
}

func (s *Server) handleListJobSessions(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	// First verify the job exists
	_, err := s.store.GetJob(jobID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Job not found: "+err.Error())
		return
	}

	sessions, err := s.store.ListSessionsByJob(jobID)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list sessions: "+err.Error())
		return
	}

	resp := make([]SessionListItem, len(sessions))
	for i, sess := range sessions {
		resp[i] = SessionListItem{
			ID:        sess.ID,
			AgentID:   sess.AgentID,
			Title:     sess.Title,
			Status:    sess.Status,
			CreatedAt: sess.CreatedAt,
			UpdatedAt: sess.UpdatedAt,
		}
	}

	s.jsonResponse(w, http.StatusOK, resp)
}

// parseScheduleToCron uses the LLM to convert natural language schedule to cron expression
func (s *Server) parseScheduleToCron(ctx context.Context, scheduleText string) (string, error) {
	prompt := fmt.Sprintf(`Convert the following natural language schedule to a standard 5-field cron expression.
Only respond with the cron expression, nothing else. No explanation, no formatting, just the cron expression.

Schedule: "%s"

Examples:
- "every day at 7pm" -> "0 19 * * *"
- "every Monday at 9am" -> "0 9 * * 1"
- "every hour" -> "0 * * * *"
- "every weekday at 8:30am" -> "30 8 * * 1-5"
- "every 15 minutes" -> "*/15 * * * *"

Cron expression:`, scheduleText)

	// Create a minimal session for this parsing task
	sess, err := s.sessionManager.Create("scheduler")
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer s.sessionManager.Delete(sess.ID)

	sess.AddUserMessage(prompt)

	// Create agent config for parsing
	agentConfig := agent.Config{
		Name:        "scheduler",
		Model:       s.config.DefaultModel,
		MaxSteps:    1, // Only need one response
		Temperature: 0, // Deterministic output
	}

	ag := agent.New(agentConfig, s.llmClient, s.toolManager, s.sessionManager)
	cronExpr, _, err := ag.Run(ctx, sess, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to parse schedule: %w", err)
	}

	// Clean up the response (trim whitespace)
	cronExpr = strings.TrimSpace(cronExpr)

	// Basic validation: should have 5 fields
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return "", fmt.Errorf("invalid cron expression: %s", cronExpr)
	}

	return cronExpr, nil
}

// calculateNextRun calculates the next run time based on cron expression
func (s *Server) calculateNextRun(cronExpr string, after time.Time) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return schedule.Next(after), nil
}

// executeJob runs a job and returns the execution record
func (s *Server) executeJob(ctx context.Context, job *storage.RecurringJob) (*storage.JobExecution, error) {
	now := time.Now()

	// Create execution record
	exec := &storage.JobExecution{
		ID:        uuid.New().String(),
		JobID:     job.ID,
		Status:    "running",
		StartedAt: now,
	}

	if err := s.store.SaveJobExecution(exec); err != nil {
		return nil, fmt.Errorf("failed to create execution record: %w", err)
	}

	// Create a session for this job execution
	sess, err := s.sessionManager.Create("job-runner")
	if err != nil {
		exec.Status = "failed"
		exec.Error = "Failed to create session: " + err.Error()
		finishedAt := time.Now()
		exec.FinishedAt = &finishedAt
		s.store.SaveJobExecution(exec)
		return exec, nil
	}

	exec.SessionID = sess.ID

	// Run the agent with the job's task prompt
	agentConfig := agent.Config{
		Name:        "job-runner",
		Model:       s.config.DefaultModel,
		MaxSteps:    s.config.MaxSteps,
		Temperature: s.config.Temperature,
	}

	ag := agent.New(agentConfig, s.llmClient, s.toolManager, s.sessionManager)
	output, _, err := ag.Run(ctx, sess, job.TaskPrompt)

	finishedAt := time.Now()
	exec.FinishedAt = &finishedAt

	if err != nil {
		exec.Status = "failed"
		exec.Error = err.Error()
	} else {
		exec.Status = "success"
		exec.Output = output
	}

	// Update execution record
	if err := s.store.SaveJobExecution(exec); err != nil {
		logging.Error("Failed to update execution record: %v", err)
	}

	// Update job's last run time and calculate next run
	job.LastRunAt = &now
	nextRun, err := s.calculateNextRun(job.ScheduleCron, now)
	if err == nil {
		job.NextRunAt = &nextRun
	}
	job.UpdatedAt = now

	if err := s.store.SaveJob(job); err != nil {
		logging.Error("Failed to update job after execution: %v", err)
	}

	return exec, nil
}

// jobToResponse converts a storage job to API response
func (s *Server) jobToResponse(job *storage.RecurringJob) JobResponse {
	return JobResponse{
		ID:            job.ID,
		Name:          job.Name,
		ScheduleHuman: job.ScheduleHuman,
		ScheduleCron:  job.ScheduleCron,
		TaskPrompt:    job.TaskPrompt,
		Enabled:       job.Enabled,
		LastRunAt:     job.LastRunAt,
		NextRunAt:     job.NextRunAt,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
	}
}

// executionToResponse converts a storage execution to API response
func (s *Server) executionToResponse(exec *storage.JobExecution) JobExecutionResponse {
	return JobExecutionResponse{
		ID:         exec.ID,
		JobID:      exec.JobID,
		SessionID:  exec.SessionID,
		Status:     exec.Status,
		Output:     exec.Output,
		Error:      exec.Error,
		StartedAt:  exec.StartedAt,
		FinishedAt: exec.FinishedAt,
	}
}

// --- Helper methods ---

func (s *Server) sessionToResponse(sess *session.Session) SessionResponse {
	parentID := ""
	if sess.ParentID != nil {
		parentID = *sess.ParentID
	}
	return SessionResponse{
		ID:        sess.ID,
		AgentID:   sess.AgentID,
		ParentID:  parentID,
		Title:     sess.Title,
		Status:    string(sess.Status),
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
		Messages:  s.messagesToResponse(sess.Messages),
	}
}

func (s *Server) messagesToResponse(messages []session.Message) []MessageResponse {
	resp := make([]MessageResponse, len(messages))
	for i, m := range messages {
		msg := MessageResponse{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp,
		}

		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]ToolCallResponse, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				msg.ToolCalls[j] = ToolCallResponse{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Input,
				}
			}
		}

		if len(m.ToolResults) > 0 {
			msg.ToolResults = make([]ToolResultResponse, len(m.ToolResults))
			for j, tr := range m.ToolResults {
				msg.ToolResults[j] = ToolResultResponse{
					ToolCallID: tr.ToolCallID,
					Content:    tr.Content,
					IsError:    tr.IsError,
				}
			}
		}

		resp[i] = msg
	}
	return resp
}

func (s *Server) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) errorResponse(w http.ResponseWriter, status int, message string) {
	logging.Error("HTTP error: %d - %s", status, message)
	s.jsonResponse(w, status, map[string]string{"error": message})
}
