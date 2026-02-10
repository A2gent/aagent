package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// TaskTool spawns sub-agents for parallel work
type TaskTool struct {
	workDir string
	spawner SubAgentSpawner
}

// SubAgentSpawner interface for spawning sub-agents
type SubAgentSpawner interface {
	Spawn(ctx context.Context, agentType string, prompt string, parentContext []byte) (string, error)
}

// TaskParams defines parameters for the task tool
type TaskParams struct {
	AgentType   string `json:"agent_type"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

// NewTaskTool creates a new task tool
func NewTaskTool(workDir string, spawner SubAgentSpawner) *TaskTool {
	return &TaskTool{
		workDir: workDir,
		spawner: spawner,
	}
}

func (t *TaskTool) Name() string {
	return "task"
}

func (t *TaskTool) Description() string {
	return `Launch a sub-agent to handle a specific task autonomously.
Use this for complex multi-step tasks or to parallelize work.
The sub-agent inherits the parent context and has access to the same tools.

Available agent types:
- general: General-purpose agent for research and multi-step tasks
- explore: Fast read-only agent for codebase exploration
- developer: Code implementation and debugging
- tester: Code review and test writing
- docs: Documentation generation`
}

func (t *TaskTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_type": map[string]interface{}{
				"type":        "string",
				"description": "Type of sub-agent to spawn (general, explore, developer, tester, docs)",
				"enum":        []string{"general", "explore", "developer", "tester", "docs"},
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The task/instruction for the sub-agent",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "A short (3-5 words) description of the task",
			},
		},
		"required": []string{"agent_type", "prompt", "description"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var p TaskParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if p.AgentType == "" {
		return &Result{Success: false, Error: "agent_type is required"}, nil
	}
	if p.Prompt == "" {
		return &Result{Success: false, Error: "prompt is required"}, nil
	}

	// Check if spawner is configured
	if t.spawner == nil {
		return &Result{
			Success: false,
			Error:   "sub-agent spawning not configured",
		}, nil
	}

	// Spawn sub-agent
	result, err := t.spawner.Spawn(ctx, p.AgentType, p.Prompt, nil)
	if err != nil {
		return &Result{
			Success: false,
			Error:   fmt.Sprintf("failed to spawn sub-agent: %v", err),
		}, nil
	}

	return &Result{
		Success: true,
		Output:  result,
	}, nil
}

// Ensure TaskTool implements Tool
var _ Tool = (*TaskTool)(nil)
