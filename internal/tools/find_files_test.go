package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFindFilesToolBasic(t *testing.T) {
	workDir := "/Users/artjom/git/a2gent"
	tool := NewFindFilesTool(workDir)

	// Test default behavior
	params := map[string]interface{}{
		"pattern": "*.md",
	}
	jsonParams, _ := json.Marshal(params)
	result, err := tool.Execute(context.Background(), jsonParams)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Error)
	}

	// Check that pagination info is included
	if !strings.Contains(result.Output, "Page 1 of") && !strings.Contains(result.Output, "showing all") {
		t.Logf("Output: %s", result.Output)
	}
}

func TestFindFilesToolPagination(t *testing.T) {
	workDir := "/Users/artjom/git/a2gent"
	tool := NewFindFilesTool(workDir)

	// Test pagination
	params := map[string]interface{}{
		"pattern":   "**/*.go",
		"page":      1,
		"page_size": 5,
	}
	jsonParams, _ := json.Marshal(params)
	result, err := tool.Execute(context.Background(), jsonParams)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success, got failure: %s", result.Error)
	}

	// Should show pagination info
	if !strings.Contains(result.Output, "Page 1 of") {
		t.Errorf("Expected pagination info, got: %s", result.Output)
	}
}

func TestFindFilesToolHiddenFiles(t *testing.T) {
	workDir := "/Users/artjom/git/a2gent"
	tool := NewFindFilesTool(workDir)

	// Test without hidden files (default)
	params1 := map[string]interface{}{
		"pattern": ".*",
	}
	jsonParams1, _ := json.Marshal(params1)
	result1, err := tool.Execute(context.Background(), jsonParams1)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test with hidden files
	params2 := map[string]interface{}{
		"pattern":     ".*",
		"show_hidden": true,
	}
	jsonParams2, _ := json.Marshal(params2)
	result2, err := tool.Execute(context.Background(), jsonParams2)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Result with hidden files should have more results (or at least same)
	lines1 := strings.Count(result1.Output, "\n")
	lines2 := strings.Count(result2.Output, "\n")

	if lines2 < lines1 {
		t.Errorf("Expected more results with show_hidden=true, got %d vs %d", lines2, lines1)
	}
}
