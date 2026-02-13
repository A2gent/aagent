package agent

import (
	"context"
	"fmt"

	"github.com/gratheon/aagent/internal/llm"
	"github.com/gratheon/aagent/internal/logging"
	"github.com/gratheon/aagent/internal/session"
	"github.com/gratheon/aagent/internal/tools"
)

// Config holds agent configuration
type Config struct {
	Name         string
	Description  string
	Model        string
	SystemPrompt string
	Temperature  float64
	MaxSteps     int
}

// Agent represents an AI agent that can execute tasks
type Agent struct {
	config         Config
	llmClient      llm.Client
	toolManager    *tools.Manager
	sessionManager *session.Manager
}

// EventType is emitted while the agent executes a run.
type EventType string

const (
	EventAssistantDelta EventType = "assistant_delta"
	EventStepCompleted  EventType = "step_completed"
	EventToolExecuting  EventType = "tool_executing"
	EventToolCompleted  EventType = "tool_completed"
)

// Event describes a streaming update from the agent.
type Event struct {
	Type  EventType
	Step  int
	Delta string
}

// New creates a new agent
func New(config Config, llmClient llm.Client, toolManager *tools.Manager, sessionManager *session.Manager) *Agent {
	if config.MaxSteps == 0 {
		config.MaxSteps = 50
	}
	if config.SystemPrompt == "" {
		config.SystemPrompt = defaultSystemPrompt
	}

	return &Agent{
		config:         config,
		llmClient:      llmClient,
		toolManager:    toolManager,
		sessionManager: sessionManager,
	}
}

// Run executes the agent with the given task
// Returns the response content and total token usage
func (a *Agent) Run(ctx context.Context, sess *session.Session, task string) (string, llm.TokenUsage, error) {
	return a.RunWithEvents(ctx, sess, task, nil)
}

// RunWithEvents executes the agent and emits streaming events when available.
func (a *Agent) RunWithEvents(ctx context.Context, sess *session.Session, task string, onEvent func(Event)) (string, llm.TokenUsage, error) {
	logging.Info("Agent run started: session=%s", sess.ID)
	// Note: User message is already added by the TUI before calling Run
	// Run the agentic loop
	result, usage, err := a.loop(ctx, sess, onEvent)
	if err != nil {
		logging.Error("Agent run failed: %v", err)
	} else {
		logging.Info("Agent run completed: total_input=%d total_output=%d", usage.InputTokens, usage.OutputTokens)
	}
	return result, usage, err
}

// loop implements the main agentic loop
// Returns the response content and total token usage
func (a *Agent) loop(ctx context.Context, sess *session.Session, onEvent func(Event)) (string, llm.TokenUsage, error) {
	step := 0
	totalUsage := llm.TokenUsage{}

	// Clean up incomplete tool calls before starting
	a.cleanupIncompleteToolCalls(sess)

	for {
		// Check context
		if ctx.Err() != nil {
			sess.SetStatus(session.StatusPaused)
			a.sessionManager.Save(sess)
			return "", totalUsage, ctx.Err()
		}

		// Check step limit
		if step >= a.config.MaxSteps {
			sess.SetStatus(session.StatusCompleted)
			a.sessionManager.Save(sess)
			return a.getLastAssistantContent(sess), totalUsage, nil
		}

		step++
		logging.Debug("Agent step %d/%d", step, a.config.MaxSteps)

		// Build chat request
		request := a.buildRequest(sess)

		// Call LLM (streaming when supported)
		response, err := a.callLLM(ctx, request, step, onEvent)
		if err != nil {
			sess.SetStatus(session.StatusFailed)
			a.sessionManager.Save(sess)
			return "", totalUsage, fmt.Errorf("LLM error: %w", err)
		}

		// Accumulate token usage
		totalUsage.InputTokens += response.Usage.InputTokens
		totalUsage.OutputTokens += response.Usage.OutputTokens

		// Check if we have tool calls
		if len(response.ToolCalls) == 0 {
			// No tool calls - agent is done
			sess.AddAssistantMessage(response.Content, nil)
			sess.SetStatus(session.StatusCompleted)
			a.sessionManager.Save(sess)
			if onEvent != nil {
				onEvent(Event{Type: EventStepCompleted, Step: step})
			}
			return response.Content, totalUsage, nil
		}

		// Convert tool calls for session storage
		sessionToolCalls := make([]session.ToolCall, len(response.ToolCalls))
		for i, tc := range response.ToolCalls {
			sessionToolCalls[i] = session.ToolCall{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: []byte(tc.Input),
			}
		}

		// Add assistant message with tool calls
		sess.AddAssistantMessage(response.Content, sessionToolCalls)

		// Execute tools
		if onEvent != nil {
			onEvent(Event{Type: EventToolExecuting, Step: step})
		}
		toolResults := a.toolManager.ExecuteParallel(ctx, response.ToolCalls)

		// Convert results
		sessionResults := make([]session.ToolResult, len(toolResults))
		for i, tr := range toolResults {
			sessionResults[i] = session.ToolResult{
				ToolCallID: tr.ToolCallID,
				Content:    tr.Content,
				IsError:    tr.IsError,
			}
		}

		// Add tool results to session
		sess.AddToolResult(sessionResults)

		// Save session after each step
		if err := a.sessionManager.Save(sess); err != nil {
			// Silently continue on save errors
			_ = err
		}
		if onEvent != nil {
			onEvent(Event{Type: EventToolCompleted, Step: step})
			onEvent(Event{Type: EventStepCompleted, Step: step})
		}
	}
}

func (a *Agent) callLLM(ctx context.Context, request *llm.ChatRequest, step int, onEvent func(Event)) (*llm.ChatResponse, error) {
	streamClient, ok := a.llmClient.(llm.StreamingClient)
	if !ok {
		return a.llmClient.Chat(ctx, request)
	}

	return streamClient.ChatStream(ctx, request, func(ev llm.StreamEvent) error {
		if onEvent == nil {
			return nil
		}
		if ev.Type == llm.StreamEventContentDelta && ev.ContentDelta != "" {
			onEvent(Event{
				Type:  EventAssistantDelta,
				Step:  step,
				Delta: ev.ContentDelta,
			})
		}
		return nil
	})
}

// buildRequest builds a chat request from the session
func (a *Agent) buildRequest(sess *session.Session) *llm.ChatRequest {
	// Convert session messages to LLM messages
	messages := make([]llm.Message, 0, len(sess.Messages))

	for _, m := range sess.Messages {
		msg := llm.Message{
			Role:    m.Role,
			Content: m.Content,
		}

		// Convert tool calls
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.ToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				msg.ToolCalls[i] = llm.ToolCall{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: string(tc.Input),
				}
			}
		}

		// Convert tool results
		if len(m.ToolResults) > 0 {
			msg.ToolResults = make([]llm.ToolResult, len(m.ToolResults))
			for i, tr := range m.ToolResults {
				msg.ToolResults[i] = llm.ToolResult{
					ToolCallID: tr.ToolCallID,
					Content:    tr.Content,
					IsError:    tr.IsError,
				}
			}
		}

		messages = append(messages, msg)
	}

	return &llm.ChatRequest{
		Model:        a.config.Model,
		Messages:     messages,
		Tools:        a.toolManager.GetDefinitions(),
		Temperature:  a.config.Temperature,
		SystemPrompt: a.config.SystemPrompt,
	}
}

// getLastAssistantContent returns the content of the last assistant message
func (a *Agent) getLastAssistantContent(sess *session.Session) string {
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		if sess.Messages[i].Role == "assistant" && sess.Messages[i].Content != "" {
			return sess.Messages[i].Content
		}
	}
	return ""
}

// cleanupIncompleteToolCalls removes assistant messages with tool calls that don't have corresponding tool results
// This can happen when the user interrupts a tool execution
func (a *Agent) cleanupIncompleteToolCalls(sess *session.Session) {
	if len(sess.Messages) == 0 {
		return
	}

	// Find the last assistant message with tool calls
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		msg := sess.Messages[i]

		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Check if there's a following tool message with results
			hasResults := false
			if i+1 < len(sess.Messages) && sess.Messages[i+1].Role == "tool" {
				hasResults = true
			}

			if !hasResults {
				// Remove this incomplete assistant message
				logging.Warn("Removing incomplete tool call message (no results)")
				sess.Messages = append(sess.Messages[:i], sess.Messages[i+1:]...)
				// Continue checking in case there are more
				continue
			}
		}
	}
}

// defaultSystemPrompt is the default system prompt for the agent
const defaultSystemPrompt = `You are an AI coding assistant. You help users with software engineering tasks by using the available tools.

Guidelines:
- Use tools to explore and modify the codebase
- Read files before editing to understand context
- Make minimal, targeted changes
- Explain your reasoning before making changes
- If a task is unclear, ask for clarification
- If you encounter errors, try to understand and fix them

Available tools allow you to:
- Execute shell commands (bash)
- Read file contents (read)
- Write new files (write)
- Edit existing files with string replacement (edit)
- Find files by pattern (glob)
- Search file contents (grep)

Be concise but thorough. Complete the user's task step by step.`
