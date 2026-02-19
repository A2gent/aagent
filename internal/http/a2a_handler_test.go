package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/A2gent/brute/internal/config"
	"github.com/A2gent/brute/internal/session"
	"github.com/A2gent/brute/internal/speechcache"
	"github.com/A2gent/brute/internal/storage"
	"github.com/A2gent/brute/internal/tools"
)

func TestHandleAgentCard(t *testing.T) {
	// Create temporary directory for SQLite store
	tempDir, err := os.MkdirTemp("", "a2a-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create minimal server dependencies
	cfg := &config.Config{}
	toolManager := tools.NewManager(".")
	store, err := storage.NewSQLiteStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	sessionManager := session.NewManager(store)
	speechClips := speechcache.New(0)

	server := NewServer(cfg, nil, toolManager, sessionManager, store, speechClips, 0)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	rec := httptest.NewRecorder()

	// Call handler
	server.handleAgentCard(rec, req)

	// Check status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Check content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse response
	var card AgentCard
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("Failed to parse agent card: %v", err)
	}

	// Verify required fields
	if card.Name == "" {
		t.Error("Expected Name to be set")
	}
	if card.Description == "" {
		t.Error("Expected Description to be set")
	}
	if card.Version == "" {
		t.Error("Expected Version to be set")
	}
	if len(card.SupportedInterfaces) == 0 {
		t.Error("Expected at least one SupportedInterface")
	}
	if len(card.Skills) == 0 {
		t.Error("Expected at least one Skill")
	}

	// Verify interface fields
	iface := card.SupportedInterfaces[0]
	if iface.URL == "" {
		t.Error("Expected Interface URL to be set")
	}
	if iface.ProtocolBinding == "" {
		t.Error("Expected Interface ProtocolBinding to be set")
	}
	if iface.ProtocolVersion == "" {
		t.Error("Expected Interface ProtocolVersion to be set")
	}

	// Verify skills have required fields
	for _, skill := range card.Skills {
		if skill.ID == "" {
			t.Error("Expected Skill ID to be set")
		}
		if skill.Name == "" {
			t.Error("Expected Skill Name to be set")
		}
		if skill.Description == "" {
			t.Error("Expected Skill Description to be set")
		}
	}
}
