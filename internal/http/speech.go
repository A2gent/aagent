package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	elevenLabsVoicesURL = "https://api.elevenlabs.io/v1/voices"
)

type speechCompletionRequest struct {
	Text string `json:"text"`
}

type elevenLabsVoice struct {
	VoiceID    string `json:"voice_id"`
	Name       string `json:"name"`
	PreviewURL string `json:"preview_url,omitempty"`
}

type elevenLabsVoicesResponse struct {
	Voices []elevenLabsVoice `json:"voices"`
}

type elevenLabsTTSRequest struct {
	Text          string                  `json:"text"`
	ModelID       string                  `json:"model_id"`
	VoiceSettings elevenLabsVoiceSettings `json:"voice_settings,omitempty"`
}

type elevenLabsVoiceSettings struct {
	Speed float64 `json:"speed,omitempty"`
}

func (s *Server) handleListSpeechVoices(w http.ResponseWriter, r *http.Request) {
	apiKey := s.resolveElevenLabsAPIKey()
	if apiKey == "" {
		s.errorResponse(w, http.StatusBadRequest, "ElevenLabs API key is not configured. Add an enabled ElevenLabs integration in Integrations.")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, elevenLabsVoicesURL, nil)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to build ElevenLabs request: "+err.Error())
		return
	}
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.errorResponse(w, http.StatusBadGateway, "Failed to fetch ElevenLabs voices: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.proxyElevenLabsError(w, resp, "Failed to fetch ElevenLabs voices")
		return
	}

	var payload elevenLabsVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		s.errorResponse(w, http.StatusBadGateway, "Failed to decode ElevenLabs voices response: "+err.Error())
		return
	}

	s.jsonResponse(w, http.StatusOK, payload.Voices)
}

func (s *Server) handleCompletionSpeech(w http.ResponseWriter, r *http.Request) {
	apiKey := s.resolveElevenLabsAPIKey()
	voiceID := strings.TrimSpace(os.Getenv("ELEVENLABS_VOICE_ID"))
	if apiKey == "" {
		s.errorResponse(w, http.StatusBadRequest, "ElevenLabs API key is not configured. Add an enabled ElevenLabs integration in Integrations.")
		return
	}
	if voiceID == "" {
		s.errorResponse(w, http.StatusBadRequest, "ELEVENLABS_VOICE_ID is not configured")
		return
	}

	var reqBody speechCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	text := strings.TrimSpace(reqBody.Text)
	if text == "" {
		s.errorResponse(w, http.StatusBadRequest, "text is required")
		return
	}

	ttsReq := elevenLabsTTSRequest{
		Text:    text,
		ModelID: "eleven_multilingual_v2",
	}
	if speedRaw := strings.TrimSpace(os.Getenv("ELEVENLABS_SPEED")); speedRaw != "" {
		if speed, err := strconv.ParseFloat(speedRaw, 64); err == nil && speed > 0 {
			ttsReq.VoiceSettings = elevenLabsVoiceSettings{Speed: speed}
		}
	}

	jsonBody, err := json.Marshal(ttsReq)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to build ElevenLabs request payload: "+err.Error())
		return
	}

	ttsURL := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", url.PathEscape(voiceID))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, ttsURL, bytes.NewReader(jsonBody))
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to build ElevenLabs request: "+err.Error())
		return
	}
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.errorResponse(w, http.StatusBadGateway, "Failed to call ElevenLabs: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.proxyElevenLabsError(w, resp, "ElevenLabs playback failed")
		return
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, resp.Body); err != nil {
		// Client may disconnect mid-stream; nothing actionable for handler.
		return
	}
}

func (s *Server) resolveElevenLabsAPIKey() string {
	integrations, err := s.store.ListIntegrations()
	if err == nil {
		for _, integration := range integrations {
			if integration == nil || !integration.Enabled || integration.Provider != "elevenlabs" {
				continue
			}
			apiKey := strings.TrimSpace(integration.Config["api_key"])
			if apiKey != "" {
				return apiKey
			}
		}
	}

	return strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
}

func (s *Server) proxyElevenLabsError(w http.ResponseWriter, upstream *http.Response, fallback string) {
	body, _ := io.ReadAll(io.LimitReader(upstream.Body, 8192))
	statusDetail := strings.TrimSpace(string(body))
	if statusDetail == "" {
		statusDetail = upstream.Status
	}
	s.errorResponse(w, upstream.StatusCode, fmt.Sprintf("%s: %s", fallback, statusDetail))
}
