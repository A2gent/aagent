package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseTaskStats(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantTotal int
		wantDone  int
		wantPct   int
	}{
		{
			name:      "empty content",
			content:   "",
			wantTotal: 0,
			wantDone:  0,
			wantPct:   0,
		},
		{
			name:      "simple pending tasks",
			content:   "[ ] Task 1\n[ ] Task 2\n[ ] Task 3",
			wantTotal: 3,
			wantDone:  0,
			wantPct:   0,
		},
		{
			name:      "simple completed tasks",
			content:   "[x] Task 1\n[x] Task 2\n[x] Task 3",
			wantTotal: 3,
			wantDone:  3,
			wantPct:   100,
		},
		{
			name:      "mixed tasks",
			content:   "[x] Task 1\n[ ] Task 2\n[x] Task 3\n[ ] Task 4",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
		{
			name:      "uppercase X",
			content:   "[X] Task 1\n[X] Task 2",
			wantTotal: 2,
			wantDone:  2,
			wantPct:   100,
		},
		{
			name:      "markdown list format with dash",
			content:   "- [ ] Task 1\n- [x] Task 2\n- [ ] Task 3\n- [x] Task 4",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
		{
			name:      "indented nested tasks",
			content:   "[ ] Parent task\n  [ ] Child task 1\n  [x] Child task 2",
			wantTotal: 3,
			wantDone:  1,
			wantPct:   33,
		},
		{
			name:      "markdown list with indentation",
			content:   "- [x] Step 1\n  - [ ] Sub-task 1.1\n  - [x] Sub-task 1.2\n- [ ] Step 2",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
		{
			name:      "with leading newlines",
			content:   "\n- [x] Sub-task 1\n- [x] Sub-task 2\n- [ ] Sub-task 3\n- [ ] Sub-task 4\n",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
		{
			name:      "windows line endings",
			content:   "- [x] Task 1\r\n- [ ] Task 2\r\n- [x] Task 3",
			wantTotal: 3,
			wantDone:  2,
			wantPct:   66,
		},
		{
			name:      "mixed formats in same content",
			content:   "[ ] Simple task\n- [x] Dash task\n  [ ] Nested simple\n  - [x] Nested dash",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
		{
			name:      "task with extra spaces after dash",
			content:   "- [ ] Task with spaces\n- [x] Done task",
			wantTotal: 2,
			wantDone:  1,
			wantPct:   50,
		},
		{
			name:      "non-task lines ignored",
			content:   "# Header\n- [x] Task 1\nSome text\n- [ ] Task 2\n\nMore text",
			wantTotal: 2,
			wantDone:  1,
			wantPct:   50,
		},
		{
			name:      "only whitespace",
			content:   "   \n\n   \n",
			wantTotal: 0,
			wantDone:  0,
			wantPct:   0,
		},
		{
			name:      "deeply nested tasks",
			content:   "- [ ] Level 0\n  - [x] Level 1\n    - [ ] Level 2\n      - [x] Level 3",
			wantTotal: 4,
			wantDone:  2,
			wantPct:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := parseTaskStats(tt.content)

			if stats.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", stats.Total, tt.wantTotal)
			}
			if stats.Completed != tt.wantDone {
				t.Errorf("Completed = %d, want %d", stats.Completed, tt.wantDone)
			}
			if stats.ProgressPct != tt.wantPct {
				t.Errorf("ProgressPct = %d, want %d", stats.ProgressPct, tt.wantPct)
			}
		})
	}
}

func TestParseTaskStats_RealWorldExamples(t *testing.T) {
	// Test with actual format the LLM might produce
	tests := []struct {
		name      string
		content   string
		wantTotal int
		wantDone  int
	}{
		{
			name: "LLM format with newline prefix",
			content: `
- [x] Sub-task 1
- [x] Sub-task 2
- [ ] Sub-task 3
- [ ] Sub-task 4
`,
			wantTotal: 4,
			wantDone:  2,
		},
		{
			name: "project planning style",
			content: `- [x] Analyze requirements
  - [x] Read existing code
  - [x] Identify patterns
- [ ] Implement solution
  - [ ] Write code
  - [ ] Add tests
- [ ] Deploy`,
			wantTotal: 7,
			wantDone:  3,
		},
		{
			name: "simple checklist",
			content: `[ ] Task A
[ ] Task B
[x] Task C
[x] Task D`,
			wantTotal: 4,
			wantDone:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := parseTaskStats(tt.content)

			if stats.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", stats.Total, tt.wantTotal)
			}
			if stats.Completed != tt.wantDone {
				t.Errorf("Completed = %d, want %d", stats.Completed, tt.wantDone)
			}
		})
	}
}

// mockTaskProgressStore is a mock implementation of TaskProgressStore
type mockTaskProgressStore struct {
	progress map[string]string
}

func newMockTaskProgressStore() *mockTaskProgressStore {
	return &mockTaskProgressStore{
		progress: make(map[string]string),
	}
}

func (m *mockTaskProgressStore) GetSessionTaskProgress(sessionID string) (string, error) {
	return m.progress[sessionID], nil
}

func (m *mockTaskProgressStore) SetSessionTaskProgress(sessionID string, progress string) error {
	m.progress[sessionID] = progress
	return nil
}

func TestSessionTaskProgressTool_Execute(t *testing.T) {
	store := newMockTaskProgressStore()
	tool := NewSessionTaskProgressTool(store)

	t.Run("set action without session_id in context", func(t *testing.T) {
		params := map[string]interface{}{
			"action":  "set",
			"content": "- [ ] Task 1",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(context.Background(), jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Success {
			t.Error("Expected failure when session_id is missing from context")
		}
		if result.Error != "session_id not found in context" {
			t.Errorf("Unexpected error: %s", result.Error)
		}
	})

	t.Run("set action with session_id in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "session_id", "test-session-1")
		params := map[string]interface{}{
			"action":  "set",
			"content": "- [x] Task 1\n- [ ] Task 2",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(ctx, jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if !result.Success {
			t.Errorf("Expected success, got error: %s", result.Error)
		}

		// Verify store was updated
		stored := store.progress["test-session-1"]
		if stored != "- [x] Task 1\n- [ ] Task 2" {
			t.Errorf("Store not updated correctly, got: %q", stored)
		}
	})

	t.Run("get action", func(t *testing.T) {
		store.progress["test-session-2"] = "- [x] Done\n- [ ] Pending"
		ctx := context.WithValue(context.Background(), "session_id", "test-session-2")
		params := map[string]interface{}{
			"action": "get",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(ctx, jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if !result.Success {
			t.Errorf("Expected success, got error: %s", result.Error)
		}
		if result.Output != "- [x] Done\n- [ ] Pending" {
			t.Errorf("Unexpected output: %q", result.Output)
		}
	})

	t.Run("append action", func(t *testing.T) {
		store.progress["test-session-3"] = "- [x] Task 1"
		ctx := context.WithValue(context.Background(), "session_id", "test-session-3")
		params := map[string]interface{}{
			"action":  "append",
			"content": "- [ ] Task 2",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(ctx, jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if !result.Success {
			t.Errorf("Expected success, got error: %s", result.Error)
		}

		// Verify store was updated with appended content
		stored := store.progress["test-session-3"]
		expected := "- [x] Task 1\n- [ ] Task 2"
		if stored != expected {
			t.Errorf("Store not updated correctly, got: %q, want: %q", stored, expected)
		}
	})

	t.Run("set action without content", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "session_id", "test-session-4")
		params := map[string]interface{}{
			"action": "set",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(ctx, jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Success {
			t.Error("Expected failure when content is missing")
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "session_id", "test-session-5")
		params := map[string]interface{}{
			"action": "invalid",
		}
		jsonParams, _ := json.Marshal(params)

		result, err := tool.Execute(ctx, jsonParams)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Success {
			t.Error("Expected failure for invalid action")
		}
	})
}
