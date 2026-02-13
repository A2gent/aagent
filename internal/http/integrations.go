package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gratheon/aagent/internal/storage"
)

var supportedIntegrationProviders = map[string]struct{}{
	"telegram": {},
	"slack":    {},
	"discord":  {},
	"whatsapp": {},
	"webhook":  {},
}

var supportedIntegrationModes = map[string]struct{}{
	"notify_only": {},
	"duplex":      {},
}

var requiredConfigFields = map[string][]string{
	"telegram": {"bot_token", "chat_id"},
	"slack":    {"bot_token", "channel_id"},
	"discord":  {"bot_token", "channel_id"},
	"whatsapp": {"access_token", "phone_number_id", "recipient"},
	"webhook":  {"url"},
}

type IntegrationRequest struct {
	Provider string            `json:"provider"`
	Name     string            `json:"name"`
	Mode     string            `json:"mode"`
	Enabled  *bool             `json:"enabled,omitempty"`
	Config   map[string]string `json:"config"`
}

type IntegrationResponse struct {
	ID        string            `json:"id"`
	Provider  string            `json:"provider"`
	Name      string            `json:"name"`
	Mode      string            `json:"mode"`
	Enabled   bool              `json:"enabled"`
	Config    map[string]string `json:"config"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type IntegrationTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	integrations, err := s.store.ListIntegrations()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list integrations: "+err.Error())
		return
	}

	resp := make([]IntegrationResponse, len(integrations))
	for i, integration := range integrations {
		resp[i] = integrationToResponse(integration)
	}

	s.jsonResponse(w, http.StatusOK, resp)
}

func (s *Server) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	var req IntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	integration, err := newIntegrationFromRequest(req)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now()
	integration.ID = uuid.New().String()
	integration.CreatedAt = now
	integration.UpdatedAt = now

	if err := s.store.SaveIntegration(integration); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to save integration: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusCreated, integrationToResponse(integration))
}

func (s *Server) handleGetIntegration(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	integration, err := s.store.GetIntegration(integrationID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Integration not found: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusOK, integrationToResponse(integration))
}

func (s *Server) handleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	existing, err := s.store.GetIntegration(integrationID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Integration not found: "+err.Error())
		return
	}

	var req IntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	next, err := newIntegrationFromRequest(req)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	next.ID = existing.ID
	next.CreatedAt = existing.CreatedAt
	next.UpdatedAt = time.Now()

	if err := s.store.SaveIntegration(next); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to update integration: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusOK, integrationToResponse(next))
}

func (s *Server) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	if err := s.store.DeleteIntegration(integrationID); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to delete integration: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	integrationID := chi.URLParam(r, "integrationID")

	integration, err := s.store.GetIntegration(integrationID)
	if err != nil {
		s.errorResponse(w, http.StatusNotFound, "Integration not found: "+err.Error())
		return
	}

	if err := validateIntegration(*integration); err != nil {
		s.jsonResponse(w, http.StatusBadRequest, IntegrationTestResponse{Success: false, Message: err.Error()})
		return
	}

	s.jsonResponse(w, http.StatusOK, IntegrationTestResponse{Success: true, Message: "Configuration is valid. Live provider connectivity checks are not yet implemented."})
}

func newIntegrationFromRequest(req IntegrationRequest) (*storage.Integration, error) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	name := strings.TrimSpace(req.Name)
	if req.Config == nil {
		req.Config = map[string]string{}
	}

	integration := &storage.Integration{
		Provider: provider,
		Name:     name,
		Mode:     mode,
		Enabled:  true,
		Config:   trimConfig(req.Config),
	}
	if req.Enabled != nil {
		integration.Enabled = *req.Enabled
	}

	if err := validateIntegration(*integration); err != nil {
		return nil, err
	}

	if integration.Name == "" {
		integration.Name = defaultIntegrationName(integration.Provider)
	}

	return integration, nil
}

func validateIntegration(integration storage.Integration) error {
	if integration.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if _, ok := supportedIntegrationProviders[integration.Provider]; !ok {
		return fmt.Errorf("unsupported provider: %s", integration.Provider)
	}

	if integration.Mode == "" {
		return fmt.Errorf("mode is required")
	}
	if _, ok := supportedIntegrationModes[integration.Mode]; !ok {
		return fmt.Errorf("unsupported mode: %s", integration.Mode)
	}
	if integration.Provider == "webhook" && integration.Mode == "duplex" {
		return fmt.Errorf("webhook currently supports notify_only mode")
	}

	requiredFields := requiredConfigFields[integration.Provider]
	for _, field := range requiredFields {
		if strings.TrimSpace(integration.Config[field]) == "" {
			return fmt.Errorf("missing required config field: %s", field)
		}
	}

	if integration.Provider == "webhook" {
		url := strings.ToLower(strings.TrimSpace(integration.Config["url"]))
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("webhook url must start with http:// or https://")
		}
	}

	return nil
}

func trimConfig(config map[string]string) map[string]string {
	out := make(map[string]string, len(config))
	for key, value := range config {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(value)
	}
	return out
}

func integrationToResponse(integration *storage.Integration) IntegrationResponse {
	configCopy := make(map[string]string, len(integration.Config))
	for key, value := range integration.Config {
		configCopy[key] = value
	}

	return IntegrationResponse{
		ID:        integration.ID,
		Provider:  integration.Provider,
		Name:      integration.Name,
		Mode:      integration.Mode,
		Enabled:   integration.Enabled,
		Config:    configCopy,
		CreatedAt: integration.CreatedAt,
		UpdatedAt: integration.UpdatedAt,
	}
}

func defaultIntegrationName(provider string) string {
	switch provider {
	case "telegram":
		return "Telegram"
	case "slack":
		return "Slack"
	case "discord":
		return "Discord"
	case "whatsapp":
		return "WhatsApp"
	case "webhook":
		return "Webhook"
	default:
		return provider
	}
}
