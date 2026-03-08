package internalapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"groot/internal/agent"
	"groot/internal/httpapi/common"
)

func (h *Handlers) agentRuntimeToolCalls(w http.ResponseWriter, r *http.Request) {
	if h.state.AgentToolSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "agent runtime tool service unavailable")
		return
	}
	expected := strings.TrimSpace(h.state.AgentRuntimeSharedSecret)
	if expected == "" || common.BearerToken(r.Header.Get("Authorization")) != expected {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		TenantID       string          `json:"tenant_id"`
		AgentID        string          `json:"agent_id"`
		AgentSessionID string          `json:"agent_session_id"`
		AgentRunID     string          `json:"agent_run_id"`
		Tool           string          `json:"tool"`
		Arguments      json.RawMessage `json:"arguments"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}
	sessionID, err := uuid.Parse(strings.TrimSpace(req.AgentSessionID))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid agent_session_id")
		return
	}
	runID, err := uuid.Parse(strings.TrimSpace(req.AgentRunID))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid agent_run_id")
		return
	}
	result, err := h.state.AgentToolSvc.ExecuteTool(r.Context(), agent.ToolExecutionRequest{
		TenantID:       tenantID,
		AgentID:        agentID,
		AgentSessionID: sessionID,
		AgentRunID:     runID,
		Tool:           strings.TrimSpace(req.Tool),
		Arguments:      req.Arguments,
	})
	if err != nil {
		h.state.Logger.Error("agent runtime tool call", slog.String("agent_id", agentID.String()), slog.String("agent_run_id", runID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, result)
}
