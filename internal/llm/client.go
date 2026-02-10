package llm

import (
	"context"
)

// Client defines the interface for LLM providers
type Client interface {
	Chat(ctx context.Context, request *ChatRequest) (*ChatResponse, error)
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model        string
	Messages     []Message
	Tools        []ToolDefinition
	Temperature  float64
	MaxTokens    int
	SystemPrompt string
}

// Message represents a chat message
type Message struct {
	Role        string       `json:"role"` // "user", "assistant", "tool"
	Content     string       `json:"content"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // JSON string
}

// ToolResult represents a tool result
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// ToolDefinition defines a tool for the LLM
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	Usage      TokenUsage
	StopReason string
}

// TokenUsage tracks token consumption
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}
