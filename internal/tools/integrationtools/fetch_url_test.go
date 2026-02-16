package integrationtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchURLTool(t *testing.T) {
	// Start a local test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			<html>
				<body>
					<h1>Hello World</h1>
					<p>This is a test.</p>
					<a href="https://example.com">Link</a>
				</body>
			</html>
		`))
	}))
	defer server.Close()

	tool := NewFetchURLTool()

	// Test valid fetch
	params := map[string]interface{}{
		"url": server.URL,
	}
	paramsJSON, _ := json.Marshal(params)

	result, err := tool.Execute(context.TODO(), paramsJSON)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.Error)
	}

	// Simple check for markdown elements
	expectedSnippets := []string{
		"# Hello World",
		"This is a test.",
		"[Link](https://example.com)",
	}

	for _, snippet := range expectedSnippets {
		if !contains(result.Output, snippet) {
			t.Errorf("Expected output to contain %q, but got:\n%s", snippet, result.Output)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr || search(s, substr)))
}

func search(s, substr string) bool {
	for i := 0; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
