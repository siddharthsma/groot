package tenant

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/httpapi/common"
	"groot/internal/workflow"
)

func (h *Handlers) workflowBuilderNodeTypes(w http.ResponseWriter, r *http.Request) {
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	common.WriteJSON(w, http.StatusOK, h.state.WorkflowBuilderSvc.NodeTypes())
}

func (h *Handlers) workflowBuilderTriggerIntegrations(w http.ResponseWriter, r *http.Request) {
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	common.WriteJSON(w, http.StatusOK, h.state.WorkflowBuilderSvc.TriggerIntegrations())
}

func (h *Handlers) workflowBuilderActionIntegrations(w http.ResponseWriter, r *http.Request) {
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	common.WriteJSON(w, http.StatusOK, h.state.WorkflowBuilderSvc.ActionIntegrations())
}

func (h *Handlers) workflowBuilderConnections(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	response, err := h.state.WorkflowBuilderSvc.ListConnections(r.Context(), tenantID, strings.TrimSpace(r.URL.Query().Get("integration")), strings.TrimSpace(r.URL.Query().Get("scope")), strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		h.state.Logger.Error("list workflow builder connections", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list builder connections")
		return
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) workflowBuilderAgents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	response, err := h.state.WorkflowBuilderSvc.ListAgents(r.Context(), tenantID)
	if err != nil {
		h.state.Logger.Error("list workflow builder agents", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list builder agents")
		return
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) workflowBuilderAgentVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	agentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	response, err := h.state.WorkflowBuilderSvc.ListAgentVersions(r.Context(), tenantID, agentID)
	if err != nil {
		if errors.Is(err, agent.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "agent not found")
			return
		}
		h.state.Logger.Error("list workflow builder agent versions", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list agent versions")
		return
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) workflowBuilderWaitStrategies(w http.ResponseWriter, r *http.Request) {
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	common.WriteJSON(w, http.StatusOK, h.state.WorkflowBuilderSvc.WaitStrategies())
}

func (h *Handlers) workflowArtifactMap(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowBuilderSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow builder service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}
	response, err := h.state.WorkflowBuilderSvc.ArtifactMap(r.Context(), tenantID, versionID)
	if err != nil {
		if errors.Is(err, workflow.ErrVersionNotFound) {
			common.WriteError(w, http.StatusNotFound, "workflow version not found")
			return
		}
		h.state.Logger.Error("get workflow artifact map", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to get workflow artifact map")
		return
	}
	common.WriteJSON(w, http.StatusOK, response)
}
