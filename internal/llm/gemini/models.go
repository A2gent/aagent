package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ListModels fetches available models from Gemini API with fallback
func ListModels(apiKey, baseURL string) ([]string, error) {
	if apiKey == "" {
		// Return fallback list if no API key
		return fallbackModels(), nil
	}

	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return fallbackModels(), nil
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

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

// ListModelsWithContext fetches models with context support
func ListModelsWithContext(ctx context.Context, apiKey, baseURL string) ([]string, error) {
	if apiKey == "" {
		return fallbackModels(), nil
	}

	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err != nil {
		return fallbackModels(), nil
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fallbackModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

// fallbackModels returns a static list of known Gemini models
func fallbackModels() []string {
	return []string{
		// Gemini 3 (Preview - latest generation)
		"gemini-3-pro-preview",
		"gemini-3-flash-preview",
		"gemini-3-pro-image-preview",

		// Gemini 2.5 (Latest stable generation)
		"gemini-2.5-pro",
		"gemini-2.5-pro-preview-03-25",
		"gemini-2.5-flash",
		"gemini-2.5-flash-preview",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash-lite-preview",

		// Gemini 2.0 (Stable generation)
		"gemini-2.0-flash",
		"gemini-2.0-flash-exp",
		"gemini-2.0-flash-lite",
		"gemini-2.0-flash-thinking-exp",
		"gemini-2.0-pro-exp",

		// Gemini 1.5 (Legacy stable generation)
		"gemini-1.5-pro-latest",
		"gemini-1.5-pro",
		"gemini-1.5-pro-001",
		"gemini-1.5-pro-002",
		"gemini-1.5-flash-latest",
		"gemini-1.5-flash",
		"gemini-1.5-flash-001",
		"gemini-1.5-flash-002",
		"gemini-1.5-flash-8b-latest",
		"gemini-1.5-flash-8b",
	}
}

// DefaultModel returns the recommended default model
func DefaultModel() string {
	return "gemini-3-flash-preview"
}

// ValidateModel checks if a model name is valid
func ValidateModel(model string) error {
	validModels := fallbackModels()
	for _, m := range validModels {
		if m == model {
			return nil
		}
	}
	return fmt.Errorf("invalid model: %s", model)
}
