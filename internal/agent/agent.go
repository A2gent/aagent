package agent

import (
	"context"
	"fmt"

	"github.com/gratheon/aagent/internal/llm"
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
func (a *Agent) Run(ctx context.Context, sess *session.Session, task string) (string, error) {
	// Add user message
	sess.AddUserMessage(task)

	// Run the agentic loop
	return a.loop(ctx, sess)
}

// loop implements the main agentic loop
func (a *Agent) loop(ctx context.Context, sess *session.Session) (string, error) {
	step := 0

	for {
		// Check context
		if ctx.Err() != nil {
			sess.SetStatus(session.StatusPaused)
			a.sessionManager.Save(sess)
			return "", ctx.Err()
		}

		// Check step limit
		if step >= a.config.MaxSteps {
			fmt.Printf("\n[Reached max steps limit (%d)]\n", a.config.MaxSteps)
			sess.SetStatus(session.StatusCompleted)
			a.sessionManager.Save(sess)
			return a.getLastAssistantContent(sess), nil
		}

		step++
		fmt.Printf("\n[Step %d/%d]\n", step, a.config.MaxSteps)

		// Build chat request
		request := a.buildRequest(sess)

		// Call LLM
		response, err := a.llmClient.Chat(ctx, request)
		if err != nil {
			sess.SetStatus(session.StatusFailed)
			a.sessionManager.Save(sess)
			return "", fmt.Errorf("LLM error: %w", err)
		}

		fmt.Printf("Tokens: %d in / %d out\n", response.Usage.InputTokens, response.Usage.OutputTokens)

		// Print assistant content if any
		if response.Content != "" {
			fmt.Printf("\nAssistant: %s\n", response.Content)
		}

		// Check if we have tool calls
		if len(response.ToolCalls) == 0 {
			// No tool calls - agent is done
			sess.AddAssistantMessage(response.Content, nil)
			sess.SetStatus(session.StatusCompleted)
			a.sessionManager.Save(sess)
			return response.Content, nil
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
		fmt.Printf("\nExecuting %d tool(s)...\n", len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			fmt.Printf("  - %s\n", tc.Name)
		}

		toolResults := a.toolManager.ExecuteParallel(ctx, response.ToolCalls)

		// Convert and print results
		sessionResults := make([]session.ToolResult, len(toolResults))
		for i, tr := range toolResults {
			sessionResults[i] = session.ToolResult{
				ToolCallID: tr.ToolCallID,
				Content:    tr.Content,
				IsError:    tr.IsError,
			}

			// Print abbreviated result
			content := tr.Content
			if len(content) > 500 {
				content = content[:500] + "... (truncated)"
			}
			status := "ok"
			if tr.IsError {
				status = "error"
			}
			fmt.Printf("  [%s] %s: %s\n", response.ToolCalls[i].Name, status, content)
		}

		// Add tool results to session
		sess.AddToolResult(sessionResults)

		// Save session after each step
		if err := a.sessionManager.Save(sess); err != nil {
			fmt.Printf("Warning: failed to save session: %v\n", err)
		}
	}
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
