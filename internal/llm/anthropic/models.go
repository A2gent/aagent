package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Model represents an Anthropic model
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// ModelsResponse represents the response from Anthropic models API
type ModelsResponse struct {
	Data []Model `json:"data"`
}

// ListModels fetches available models from Anthropic API
func ListModels(apiKey string) ([]string, error) {
	if apiKey == "" {
		// Return fallback list if no API key
		return fallbackModels(), nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return fallbackModels(), nil
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		// If API fails, return fallback list
		return fallbackModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If not authorized or error, return fallback
		return fallbackModels(), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fallbackModels(), nil
	}

	var modelsResp ModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return fallbackModels(), nil
	}

	if len(modelsResp.Data) == 0 {
		return fallbackModels(), nil
	}

	models := make([]string, 0, len(modelsResp.Data))
	for _, model := range modelsResp.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

// fallbackModels returns a static list of known models as fallback
func fallbackModels() []string {
	return []string{
		// Claude 4.6 (Latest generation - Feb 2026)
		"claude-opus-4-6",
		"claude-sonnet-4-6",
		"claude-haiku-4-5-20251001",
		"claude-haiku-4-5",

		// Claude 4.5 (Legacy but still available)
		"claude-sonnet-4-5-20250929",
		"claude-sonnet-4-5",
		"claude-opus-4-5-20251101",
		"claude-opus-4-5",
		"claude-opus-4-1-20250805",
		"claude-opus-4-1",

		// Claude 4.0 (Legacy)
		"claude-sonnet-4-20250514",
		"claude-sonnet-4-0",
		"claude-opus-4-20250514",
		"claude-opus-4-0",

		// Claude 3.7 (Legacy)
		"claude-3-7-sonnet-20250219",
		"claude-3-7-sonnet-latest",

		// Claude 3 (Oldest, still available)
		"claude-3-haiku-20240307",
	}
}

// DefaultModel returns the recommended default model
func DefaultModel() string {
	return "claude-sonnet-4-6"
}
