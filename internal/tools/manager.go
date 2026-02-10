package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gratheon/aagent/internal/llm"
)

// Tool defines the interface for executable tools
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]interface{}
	Execute(ctx context.Context, params json.RawMessage) (*Result, error)
}

// Result represents a tool execution result
type Result struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// Manager manages available tools
type Manager struct {
	tools   map[string]Tool
	workDir string
	mu      sync.RWMutex
}

// NewManager creates a new tool manager
func NewManager(workDir string) *Manager {
	m := &Manager{
		tools:   make(map[string]Tool),
		workDir: workDir,
	}

	// Register built-in tools
	m.Register(NewBashTool(workDir))
	m.Register(NewReadTool(workDir))
	m.Register(NewWriteTool(workDir))
	m.Register(NewEditTool(workDir))
	m.Register(NewGlobTool(workDir))
	m.Register(NewGrepTool(workDir))

	return m
}

// Register adds a tool to the manager
func (m *Manager) Register(tool Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[tool.Name()] = tool
}

// Get returns a tool by name
func (m *Manager) Get(name string) (Tool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tool, ok := m.tools[name]
	return tool, ok
}

// Execute executes a tool by name with the given parameters
func (m *Manager) Execute(ctx context.Context, name string, params json.RawMessage) (*Result, error) {
	tool, ok := m.Get(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool.Execute(ctx, params)
}

// ExecuteParallel executes multiple tool calls in parallel
func (m *Manager) ExecuteParallel(ctx context.Context, calls []llm.ToolCall) []llm.ToolResult {
	results := make([]llm.ToolResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc llm.ToolCall) {
			defer wg.Done()

			result, err := m.Execute(ctx, tc.Name, json.RawMessage(tc.Input))

			tr := llm.ToolResult{
				ToolCallID: tc.ID,
			}

			if err != nil {
				tr.Content = fmt.Sprintf("Error: %v", err)
				tr.IsError = true
			} else if !result.Success {
				tr.Content = fmt.Sprintf("Error: %s", result.Error)
				tr.IsError = true
			} else {
				tr.Content = result.Output
			}

			results[idx] = tr
		}(i, call)
	}

	wg.Wait()
	return results
}

// GetDefinitions returns tool definitions for LLM
func (m *Manager) GetDefinitions() []llm.ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	defs := make([]llm.ToolDefinition, 0, len(m.tools))
	for _, tool := range m.tools {
		defs = append(defs, llm.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return defs
}
