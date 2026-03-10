package tenant

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"groot/internal/httpapi/common"
	"groot/internal/tenant"
	"groot/internal/workflow"
	builderapi "groot/internal/workflow/builderapi"
	workflowpublish "groot/internal/workflow/publish"
)

func (h *Handlers) workflows(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.WorkflowSvc.Create(r.Context(), tenantID, req.Name, req.Description)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "create workflow", err)
			return
		}
		h.state.Audit("workflow.create", "workflow", &created.ID, map[string]any{"name": created.Name}, r.Context())
		common.WriteJSON(w, http.StatusCreated, created)
	case http.MethodGet:
		records, err := h.state.WorkflowSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list workflows", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list workflows")
			return
		}
		common.WriteJSON(w, http.StatusOK, records)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) workflow(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}
	workflowID, err := uuid.Parse(r.PathValue("workflow_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.state.WorkflowSvc.Get(r.Context(), tenantID, workflowID)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "get workflow", err)
			return
		}
		common.WriteJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := h.state.WorkflowSvc.Update(r.Context(), tenantID, workflowID, req.Name, req.Description)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "update workflow", err)
			return
		}
		h.state.Audit("workflow.update", "workflow", &updated.ID, map[string]any{"name": updated.Name}, r.Context())
		common.WriteJSON(w, http.StatusOK, updated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) workflowVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}
	workflowID, err := uuid.Parse(r.PathValue("workflow_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			DefinitionJSON json.RawMessage `json:"definition_json"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.WorkflowSvc.CreateVersion(r.Context(), tenantID, workflowID, req.DefinitionJSON)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "create workflow version", err)
			return
		}
		h.state.Audit("workflow_version.create", "workflow_version", &created.ID, map[string]any{"workflow_id": created.WorkflowID.String(), "version_number": created.VersionNumber}, r.Context())
		common.WriteJSON(w, http.StatusCreated, created)
	case http.MethodGet:
		records, err := h.state.WorkflowSvc.ListVersions(r.Context(), tenantID, workflowID)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "list workflow versions", err)
			return
		}
		common.WriteJSON(w, http.StatusOK, records)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) workflowVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.state.WorkflowSvc.GetVersion(r.Context(), tenantID, versionID)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "get workflow version", err)
			return
		}
		common.WriteJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var req struct {
			DefinitionJSON json.RawMessage `json:"definition_json"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := h.state.WorkflowSvc.UpdateVersion(r.Context(), tenantID, versionID, req.DefinitionJSON)
		if err != nil {
			h.handleWorkflowError(w, tenantID, "update workflow version", err)
			return
		}
		h.state.Audit("workflow_version.update", "workflow_version", &record.ID, map[string]any{"workflow_id": record.WorkflowID.String(), "version_number": record.VersionNumber}, r.Context())
		common.WriteJSON(w, http.StatusOK, record)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) validateWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}
	result, err := h.state.WorkflowSvc.ValidateVersion(r.Context(), tenantID, versionID)
	if err != nil {
		h.handleWorkflowError(w, tenantID, "validate workflow version", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, builderapi.ValidationResponse{
		OK:                result.Valid,
		WorkflowVersionID: versionID.String(),
		Errors:            builderapi.ValidationErrors(result.Errors),
	})
}

func (h *Handlers) compileWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}
	record, err := h.state.WorkflowSvc.CompileVersion(r.Context(), tenantID, versionID)
	if err != nil {
		var failure workflow.ValidationFailedError
		if errors.As(err, &failure) {
			common.WriteJSON(w, http.StatusOK, builderapi.ValidationResponse{
				OK:                false,
				WorkflowVersionID: versionID.String(),
				Errors:            builderapi.ValidationErrors(failure.Issues),
			})
			return
		}
		h.handleWorkflowError(w, tenantID, "compile workflow version", err)
		return
	}
	response, err := builderapi.CompileResponseFromVersion(record)
	if err != nil {
		h.state.Logger.Error("compile workflow version response", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to build compile response")
		return
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) publishWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowPublishSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow publish service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}
	result, err := h.state.WorkflowPublishSvc.Publish(r.Context(), tenantID, versionID)
	if err != nil {
		if errors.Is(err, workflowpublish.ErrVersionNotValid) {
			version, getErr := h.state.WorkflowSvc.GetVersion(r.Context(), tenantID, versionID)
			if getErr == nil {
				common.WriteJSON(w, http.StatusOK, builderapi.ValidationResponse{
					OK:                false,
					WorkflowVersionID: versionID.String(),
					Errors:            builderapi.ValidationErrors(parseWorkflowValidationIssues(version.ValidationErrorsJSON)),
				})
				return
			}
		}
		h.handleWorkflowPublishError(w, tenantID, "publish workflow version", err)
		return
	}
	h.state.Audit("workflow_version.publish", "workflow_version", &result.Version.ID, map[string]any{
		"workflow_id":    result.Workflow.ID.String(),
		"version_number": result.Version.VersionNumber,
		"published_at":   result.Version.PublishedAt,
		"entry_bindings": result.Artifacts.ArtifactsSummary.EntryBindingsActive,
		"subscriptions":  result.Artifacts.ArtifactsSummary.SubscriptionsActive,
	}, r.Context())
	common.WriteJSON(w, http.StatusOK, builderapi.PublishResponseFromResult(result))
}

func (h *Handlers) unpublishWorkflow(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowPublishSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow publish service unavailable")
		return
	}
	workflowID, err := uuid.Parse(r.PathValue("workflow_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}
	result, err := h.state.WorkflowPublishSvc.Unpublish(r.Context(), tenantID, workflowID)
	if err != nil {
		h.handleWorkflowPublishError(w, tenantID, "unpublish workflow", err)
		return
	}
	h.state.Audit("workflow.unpublish", "workflow", &result.Workflow.ID, map[string]any{
		"workflow_id": result.Workflow.ID.String(),
	}, r.Context())
	common.WriteJSON(w, http.StatusOK, result)
}

func (h *Handlers) workflowArtifacts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowPublishSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow publish service unavailable")
		return
	}
	workflowID, err := uuid.Parse(r.PathValue("workflow_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}
	artifacts, err := h.state.WorkflowPublishSvc.ArtifactsByWorkflow(r.Context(), tenantID, workflowID)
	if err != nil {
		h.handleWorkflowPublishError(w, tenantID, "list workflow artifacts", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, artifacts)
}

func (h *Handlers) workflowVersionArtifacts(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowPublishSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow publish service unavailable")
		return
	}
	versionID, err := uuid.Parse(r.PathValue("version_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid version_id")
		return
	}
	artifacts, err := h.state.WorkflowPublishSvc.ArtifactsByVersion(r.Context(), tenantID, versionID)
	if err != nil {
		h.handleWorkflowPublishError(w, tenantID, "list workflow version artifacts", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, artifacts)
}

func (h *Handlers) workflowRuns(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowRuntimeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow runtime service unavailable")
		return
	}
	workflowID, err := uuid.Parse(r.PathValue("workflow_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}
	records, err := h.state.WorkflowRuntimeSvc.ListRuns(r.Context(), tenantID, workflowID, 50)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list workflow runs")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) workflowRun(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowRuntimeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow runtime service unavailable")
		return
	}
	runID, err := uuid.Parse(r.PathValue("run_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	record, err := h.state.WorkflowRuntimeSvc.GetRun(r.Context(), tenantID, runID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) workflowRunSteps(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowRuntimeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow runtime service unavailable")
		return
	}
	runID, err := uuid.Parse(r.PathValue("run_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	records, err := h.state.WorkflowRuntimeSvc.ListRunSteps(r.Context(), tenantID, runID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list workflow run steps")
		return
	}
	waits, err := h.state.WorkflowRuntimeSvc.ListRunWaits(r.Context(), tenantID, runID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list workflow run waits")
		return
	}
	common.WriteJSON(w, http.StatusOK, builderapi.StepResponses(records, waits))
}

func (h *Handlers) workflowRunWaits(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowRuntimeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow runtime service unavailable")
		return
	}
	runID, err := uuid.Parse(r.PathValue("run_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	records, err := h.state.WorkflowRuntimeSvc.ListRunWaits(r.Context(), tenantID, runID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list workflow run waits")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) cancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.WorkflowRuntimeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "workflow runtime service unavailable")
		return
	}
	runID, err := uuid.Parse(r.PathValue("run_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	record, err := h.state.WorkflowRuntimeSvc.CancelRun(r.Context(), tenantID, runID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "workflow run cannot be cancelled")
		return
	}
	h.state.Audit("workflow_run.cancel", "workflow_run", &record.ID, map[string]any{"workflow_id": record.WorkflowID.String()}, r.Context())
	common.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) handleWorkflowError(w http.ResponseWriter, tenantID tenant.ID, action string, err error) {
	switch {
	case errors.Is(err, workflow.ErrInvalidWorkflowName), errors.Is(err, workflow.ErrDuplicateWorkflowName), errors.Is(err, workflow.ErrInvalidDefinition):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case workflowvalidationFailure(err):
		var failure workflow.ValidationFailedError
		_ = errors.As(err, &failure)
		common.WriteJSON(w, http.StatusBadRequest, workflow.ValidateResult{Valid: false, Errors: failure.Issues})
	case errors.Is(err, workflow.ErrWorkflowNotFound):
		common.WriteError(w, http.StatusNotFound, "workflow not found")
	case errors.Is(err, workflow.ErrVersionNotFound):
		common.WriteError(w, http.StatusNotFound, "workflow version not found")
	default:
		h.state.Logger.Error(action, slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "workflow operation failed")
	}
}

func (h *Handlers) handleWorkflowPublishError(w http.ResponseWriter, tenantID tenant.ID, action string, err error) {
	switch {
	case errors.Is(err, workflowpublish.ErrVersionNotCompiled), errors.Is(err, workflowpublish.ErrVersionNotValid):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, workflow.ErrWorkflowNotFound), errors.Is(err, workflow.ErrVersionNotFound):
		common.WriteError(w, http.StatusNotFound, err.Error())
	default:
		h.state.Logger.Error(action, slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "workflow publish failed")
	}
}

func workflowvalidationFailure(err error) bool {
	var failure workflow.ValidationFailedError
	return errors.As(err, &failure)
}

func parseWorkflowValidationIssues(raw json.RawMessage) []workflow.ValidationIssue {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return []workflow.ValidationIssue{}
	}
	var issues []workflow.ValidationIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return []workflow.ValidationIssue{{
			Code:    "validation_errors_unavailable",
			Message: "workflow validation errors are unavailable",
			Path:    "",
		}}
	}
	return issues
}
