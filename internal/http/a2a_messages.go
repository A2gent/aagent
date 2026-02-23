package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/A2gent/brute/internal/a2atunnel"
	"github.com/google/uuid"
)

func (s *Server) handleA2AMessageSend(w http.ResponseWriter, r *http.Request) {
	var req a2atunnel.InboundPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(req.Content) == 0 {
		s.errorResponse(w, http.StatusBadRequest, "A2A messages endpoint requires canonical content[]")
		return
	}

	derivedText, derivedImages := a2atunnel.LegacyFromA2AContent(req.Content)
	if strings.TrimSpace(req.Task) == "" {
		req.Task = strings.TrimSpace(derivedText)
	}
	if len(req.Images) == 0 {
		req.Images = derivedImages
	}
	if req.Sender != nil {
		if strings.TrimSpace(req.SourceAgentID) == "" {
			req.SourceAgentID = strings.TrimSpace(req.Sender.AgentID)
		}
		if strings.TrimSpace(req.SourceAgentName) == "" {
			req.SourceAgentName = strings.TrimSpace(req.Sender.Name)
		}
	}
	if strings.TrimSpace(req.A2AVersion) == "" {
		req.A2AVersion = a2atunnel.A2ABridgeVersion
	}
	if strings.TrimSpace(req.MessageID) == "" {
		req.MessageID = uuid.NewString()
	}

	payload, err := json.Marshal(req)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to encode canonical payload: "+err.Error())
		return
	}

	handler := a2atunnel.NewInboundHandler(
		"brute",
		s.sessionManager,
		s.makeA2AAgentFactory(),
		s.toolManagerForSession,
		s.getA2AInboundProjectID,
		s.getA2AInboundSubAgentID,
	)
	respPayload, err := handler.Handle(r.Context(), &a2atunnel.AgentRequest{
		Kind:      a2atunnel.KindTask,
		RequestID: req.MessageID,
		Payload:   payload,
	})
	if err != nil {
		s.errorResponse(w, http.StatusBadGateway, "A2A message handling failed: "+err.Error())
		return
	}

	var response a2atunnel.OutboundPayload
	if err := json.Unmarshal(respPayload, &response); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to decode response payload: "+err.Error())
		return
	}
	if len(response.Content) == 0 {
		response.Content = a2atunnel.BuildA2AContent(response.Result, response.Images)
	}
	if strings.TrimSpace(response.A2AVersion) == "" {
		response.A2AVersion = a2atunnel.A2ABridgeVersion
	}
	if strings.TrimSpace(response.MessageID) == "" {
		response.MessageID = uuid.NewString()
	}

	s.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "completed",
		"message": response,
	})
}
