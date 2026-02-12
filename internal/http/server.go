package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gratheon/aagent/internal/agent"
	"github.com/gratheon/aagent/internal/config"
	"github.com/gratheon/aagent/internal/llm"
	"github.com/gratheon/aagent/internal/logging"
	"github.com/gratheon/aagent/internal/session"
	"github.com/gratheon/aagent/internal/tools"
)

// Server represents the HTTP API server
type Server struct {
	config         *config.Config
	llmClient      llm.Client
	toolManager    *tools.Manager
	sessionManager *session.Manager
	router         chi.Router
	port           int
}

// NewServer creates a new HTTP server instance
func NewServer(
	cfg *config.Config,
	llmClient llm.Client,
	toolManager *tools.Manager,
	sessionManager *session.Manager,
	port int,
) *Server {
	s := &Server{
		config:         cfg,
		llmClient:      llmClient,
		toolManager:    toolManager,
		sessionManager: sessionManager,
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
