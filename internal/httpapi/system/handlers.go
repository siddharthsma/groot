package system

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"groot/internal/httpapi/common"
)

func (h *Handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) readyz(w http.ResponseWriter, r *http.Request) {
	h.state.RunChecks(w, r, h.state.Checkers)
}

func (h *Handlers) routerHealth(w http.ResponseWriter, r *http.Request) {
	h.state.RunChecks(w, r, h.state.RouterCheckers)
}

func (h *Handlers) deliveryHealth(w http.ResponseWriter, r *http.Request) {
	h.state.RunChecks(w, r, h.state.DeliveryCheckers)
}

func (h *Handlers) metricsEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	if h.state.Metrics == nil {
		return
	}
	_, _ = w.Write([]byte(h.state.Metrics.Prometheus()))
}

func (h *Handlers) systemEdition(w http.ResponseWriter, _ *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"build_edition":     h.state.EditionRuntime.BuildEdition,
		"effective_edition": h.state.EditionRuntime.EffectiveEdition,
		"tenancy_mode":      h.state.EditionRuntime.TenancyMode,
		"license":           h.state.EditionRuntime.License,
		"capabilities":      h.state.EditionRuntime.Capabilities,
	})
}

func (h *Handlers) listIntegrations(w http.ResponseWriter, r *http.Request) {
	if h.state.IntegrationCatalogSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "integration catalog unavailable")
		return
	}
	records, err := h.state.IntegrationCatalogSvc.List(r.Context())
	if err != nil {
		h.state.Logger.Error("list integrations", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list integrations")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) getIntegration(w http.ResponseWriter, r *http.Request) {
	if h.state.IntegrationCatalogSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "integration catalog unavailable")
		return
	}
	record, err := h.state.IntegrationCatalogSvc.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "integration not found")
			return
		}
		h.state.Logger.Error("get integration", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to get integration")
		return
	}
	common.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) listIntegrationOperations(w http.ResponseWriter, r *http.Request) {
	if h.state.IntegrationCatalogSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "integration catalog unavailable")
		return
	}
	records, err := h.state.IntegrationCatalogSvc.ListOperations(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "integration not found")
			return
		}
		h.state.Logger.Error("list integration operations", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list integration operations")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) listIntegrationSchemas(w http.ResponseWriter, r *http.Request) {
	if h.state.IntegrationCatalogSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "integration catalog unavailable")
		return
	}
	records, err := h.state.IntegrationCatalogSvc.ListSchemas(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "integration not found")
			return
		}
		h.state.Logger.Error("list integration schemas", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list integration schemas")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) getIntegrationConfig(w http.ResponseWriter, r *http.Request) {
	if h.state.IntegrationCatalogSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "integration catalog unavailable")
		return
	}
	record, err := h.state.IntegrationCatalogSvc.GetConfig(r.Context(), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "integration not found")
			return
		}
		h.state.Logger.Error("get integration config", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to get integration config")
		return
	}
	common.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) listSchemas(w http.ResponseWriter, r *http.Request) {
	if h.state.SchemaSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "schema service unavailable")
		return
	}
	records, err := h.state.SchemaSvc.List(r.Context())
	if err != nil {
		h.state.Logger.Error("list schemas", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list schemas")
		return
	}
	response := make([]map[string]any, 0, len(records))
	for _, record := range records {
		response = append(response, map[string]any{
			"full_name":  record.FullName,
			"event_type": record.EventType,
			"version":    record.Version,
			"source":     record.Source,
		})
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) getSchema(w http.ResponseWriter, r *http.Request) {
	if h.state.SchemaSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "schema service unavailable")
		return
	}
	record, err := h.state.SchemaSvc.Get(r.Context(), r.PathValue("full_name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "schema not found")
			return
		}
		h.state.Logger.Error("get schema", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to get schema")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"full_name": record.FullName,
		"schema":    record.SchemaJSON,
	})
}

func (h *Handlers) resendBootstrap(w http.ResponseWriter, r *http.Request) {
	if h.state.ResendSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	status, err := h.state.ResendSvc.Bootstrap(r.Context())
	if err != nil {
		h.state.Logger.Error("resend bootstrap", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to bootstrap resend")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (h *Handlers) systemInboundRoutes(w http.ResponseWriter, r *http.Request) {
	if h.state.InboundRouteSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "inbound route service unavailable")
		return
	}
	routes, err := h.state.InboundRouteSvc.ListAll(r.Context())
	if err != nil {
		h.state.Logger.Error("list system inbound routes", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list inbound routes")
		return
	}
	common.WriteJSON(w, http.StatusOK, routes)
}
