package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCodeExecutionTool_Execute(t *testing.T) {
	tool := NewCodeExecutionTool(t.TempDir())
	if !tool.available {
		t.Skipf("python runtime unavailable: %s", tool.lookupError)
	}

	t.Run("returns output_data", func(t *testing.T) {
		params := map[string]interface{}{
			"code": `
nums = input_data.get("nums", [])
output_data = {"sum": sum(nums), "count": len(nums)}
`,
			"input": map[string]interface{}{
				"nums": []int{1, 2, 3, 4},
			},
		}
		raw, _ := json.Marshal(params)
		result, err := tool.Execute(context.Background(), raw)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if !result.Success {
			t.Fatalf("expected success, got error: %s", result.Error)
		}
		if !strings.Contains(result.Output, `"sum": 10`) {
			t.Fatalf("expected output to include sum, got: %s", result.Output)
		}
		if result.Metadata == nil || result.Metadata["python_version"] == nil {
			t.Fatalf("expected python_version metadata, got: %#v", result.Metadata)
		}
	})

	t.Run("blocks dangerous import", func(t *testing.T) {
		params := map[string]interface{}{
			"code": `import os
output_data = os.listdir(".")
`,
		}
		raw, _ := json.Marshal(params)
		result, err := tool.Execute(context.Background(), raw)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.Success {
			t.Fatalf("expected failure for dangerous import, got success: %s", result.Output)
		}
		if !strings.Contains(strings.ToLower(result.Error), "not allowed") {
			t.Fatalf("expected not allowed error, got: %s", result.Error)
		}
	})
}
