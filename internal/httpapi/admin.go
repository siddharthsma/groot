package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/apikey"
	"groot/internal/connectorinstance"
	"groot/internal/graph"
	"groot/internal/replay"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

func (h *Handler) adminTenants(w http.ResponseWriter, r *http.Request) {
	if !adminAuthorized(r.Context()) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		tenants, err := h.tenantSvc.ListTenants(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list tenants")
			return
		}
		writeJSON(w, http.StatusOK, tenants)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.tenantSvc.CreateTenant(r.Context(), req.Name)
		if err != nil {
			switch {
			case errors.Is(err, tenant.ErrInvalidTenantName), errors.Is(err, tenant.ErrTenantNameExists):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, "failed to create tenant")
			}
			return
		}
		h.adminAudit(created.Tenant.ID, "admin.tenant.create", "tenant", &created.Tenant.ID, map[string]any{"name": created.Tenant.Name}, r)
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": created.Tenant.ID.String(),
			"api_key":   created.APIKey,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) adminTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := h.tenantSvc.GetTenant(r.Context(), tenantID)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				writeError(w, http.StatusNotFound, "tenant not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to get tenant")
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := h.tenantSvc.UpdateTenantName(r.Context(), tenantID, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, tenant.ErrInvalidTenantName), errors.Is(err, tenant.ErrTenantNameExists):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, tenant.ErrTenantNotFound):
				writeError(w, http.StatusNotFound, "tenant not found")
			default:
				writeError(w, http.StatusInternalServerError, "failed to update tenant")
			}
			return
		}
		h.adminAudit(tenantID, "admin.tenant.update", "tenant", &tenantID, map[string]any{"name": record.Name}, r)
		writeJSON(w, http.StatusOK, record)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) adminTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := h.apiKeySvc.List(r.Context(), tenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list api keys")
			return
		}
		writeJSON(w, http.StatusOK, keys)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.apiKeySvc.Create(r.Context(), tenantID, req.Name)
		if err != nil {
			if errors.Is(err, apikey.ErrInvalidName) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to create api key")
			return
		}
		h.adminAudit(tenantID, "admin.api_key.create", "api_key", &created.ID, map[string]any{"name": created.Name, "key_prefix": created.KeyPrefix}, r)
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         created.ID.String(),
			"name":       created.Name,
			"api_key":    created.Secret,
			"key_prefix": created.KeyPrefix,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) adminTenantAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	keyID, err := uuid.Parse(r.PathValue("api_key_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid api_key_id")
		return
	}
	key, err := h.apiKeySvc.Revoke(r.Context(), tenantID, keyID)
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	h.adminAudit(tenantID, "admin.api_key.revoke", "api_key", &key.ID, map[string]any{"key_prefix": key.KeyPrefix}, r)
	writeJSON(w, http.StatusOK, key)
}

func (h *Handler) adminConnectorInstances(w http.ResponseWriter, r *http.Request) {
	var tenantID *tenant.ID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	instances, err := h.connectorSvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("connector_name"), r.URL.Query().Get("scope"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list connector instances")
		return
	}
	response := make([]map[string]any, 0, len(instances))
	for _, instance := range instances {
		response = append(response, map[string]any{
			"id":             instance.ID.String(),
			"tenant_id":      instance.TenantID.String(),
			"connector_name": instance.ConnectorName,
			"scope":          instance.Scope,
			"created_at":     instance.CreatedAt,
			"updated_at":     instance.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) adminConnectorInstance(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		TenantID      string          `json:"tenant_id"`
		ConnectorName string          `json:"connector_name"`
		Scope         string          `json:"scope"`
		Config        json.RawMessage `json:"config"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var tenantID *tenant.ID
	if strings.TrimSpace(req.TenantID) != "" {
		parsed, err := uuid.Parse(req.TenantID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	instance, err := h.connectorSvc.AdminUpsert(r.Context(), id, tenantID, req.ConnectorName, req.Scope, req.Config)
	if err != nil {
		switch {
		case errors.Is(err, connectorinstance.ErrUnsupportedConnector), errors.Is(err, connectorinstance.ErrInvalidConfig), errors.Is(err, connectorinstance.ErrMissingBotToken), errors.Is(err, connectorinstance.ErrMissingWebhookSecret), errors.Is(err, connectorinstance.ErrMissingStripeAccount), errors.Is(err, connectorinstance.ErrMissingNotionToken), errors.Is(err, connectorinstance.ErrMissingLLMProviders), errors.Is(err, connectorinstance.ErrInvalidLLMProvider), errors.Is(err, connectorinstance.ErrMissingLLMAPIKey), errors.Is(err, connectorinstance.ErrInvalidScope), errors.Is(err, connectorinstance.ErrGlobalNotAllowed), errors.Is(err, connectorinstance.ErrTenantOnlyConnector), errors.Is(err, connectorinstance.ErrGlobalOnlyConnector), errors.Is(err, connectorinstance.ErrImmutableName), errors.Is(err, connectorinstance.ErrImmutableScope):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to upsert connector instance")
		}
		return
	}
	targetTenantID := tenant.ID(uuid.Nil)
	if tenantID != nil {
		targetTenantID = *tenantID
	}
	h.adminAudit(targetTenantID, "admin.connector_instance.upsert", "connector_instance", &instance.ID, map[string]any{
		"connector_name": instance.ConnectorName,
		"scope":          instance.Scope,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             instance.ID.String(),
		"tenant_id":      instance.TenantID.String(),
		"connector_name": instance.ConnectorName,
		"scope":          instance.Scope,
		"created_at":     instance.CreatedAt,
		"updated_at":     instance.UpdatedAt,
	})
}

func (h *Handler) adminSubscriptions(w http.ResponseWriter, r *http.Request) {
	var tenantID *tenant.ID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	subs, err := h.subSvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("event_type"), r.URL.Query().Get("destination_type"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	response := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		record := map[string]any{
			"id":                      sub.ID.String(),
			"tenant_id":               sub.TenantID.String(),
			"destination_type":        sub.DestinationType,
			"connector_instance_id":   optionalUUIDValue(sub.ConnectorInstanceID),
			"connected_app_id":        optionalUUIDValue(sub.ConnectedAppID),
			"function_destination_id": optionalUUIDValue(sub.FunctionDestinationID),
			"operation":               sub.Operation,
			"operation_params":        sub.OperationParams,
			"filter":                  sub.Filter,
			"event_type":              sub.EventType,
			"event_source":            sub.EventSource,
			"emit_success_event":      sub.EmitSuccessEvent,
			"emit_failure_event":      sub.EmitFailureEvent,
			"status":                  sub.Status,
		}
		response = append(response, record)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) adminTenantSubscriptions(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	var req struct {
		ConnectedAppID        string          `json:"connected_app_id"`
		DestinationType       string          `json:"destination_type"`
		FunctionDestinationID string          `json:"function_destination_id"`
		ConnectorInstanceID   string          `json:"connector_instance_id"`
		Operation             string          `json:"operation"`
		OperationParams       json.RawMessage `json:"operation_params"`
		Filter                json.RawMessage `json:"filter"`
		EventType             string          `json:"event_type"`
		EventSource           *string         `json:"event_source"`
		EmitSuccessEvent      bool            `json:"emit_success_event"`
		EmitFailureEvent      bool            `json:"emit_failure_event"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	appID, functionID, connectorInstanceID, operation, ok := parseSubscriptionRequestIDs(w, req.ConnectedAppID, req.FunctionDestinationID, req.ConnectorInstanceID, req.Operation)
	if !ok {
		return
	}
	result, err := h.subSvc.Create(r.Context(), tenantID, req.DestinationType, appID, functionID, connectorInstanceID, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
			writeError(w, http.StatusBadRequest, err.Error())
		case isFilterValidationError(err):
			writeSubscriptionFilterError(w, err)
		case errors.Is(err, subscription.ErrConnectedAppNotFound), errors.Is(err, subscription.ErrFunctionDestinationNotFound), errors.Is(err, subscription.ErrConnectorInstanceNotFound), errors.Is(err, subscription.ErrConnectorInstanceForbidden):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to create subscription")
		}
		return
	}
	h.adminAudit(tenantID, "admin.subscription.create_for_tenant", "subscription", &result.Subscription.ID, map[string]any{
		"destination_type": result.Subscription.DestinationType,
		"event_type":       result.Subscription.EventType,
	}, r)
	writeJSON(w, http.StatusCreated, subscriptionResponse(result))
}

func (h *Handler) adminEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requiredTenantQuery(r.URL.Query().Get("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := optionalTime(r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := optionalTime(r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to")
		return
	}
	limit, err := optionalAdminLimit(r.URL.Query().Get("limit"), 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	events, err := h.eventQuerySvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("event_type"), from, to, limit, h.adminAllowViewPayloads)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) adminDeliveryJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requiredTenantQuery(r.URL.Query().Get("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := optionalTime(r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := optionalTime(r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to")
		return
	}
	limit, err := optionalAdminLimit(r.URL.Query().Get("limit"), 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	jobs, err := h.deliverySvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("status"), from, to, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list delivery jobs")
		return
	}
	writeJSON(w, http.StatusOK, mapJobs(jobs))
}

func (h *Handler) adminTopology(w http.ResponseWriter, r *http.Request) {
	if h.graphSvc == nil {
		writeError(w, http.StatusNotImplemented, "graph service unavailable")
		return
	}
	var tenantID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	includeGlobal := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_global")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid include_global")
			return
		}
		includeGlobal = parsed
	}
	limit, err := optionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	topology, err := h.graphSvc.BuildTopology(r.Context(), graph.TopologyRequest{
		TenantID:        tenantID,
		ConnectorName:   r.URL.Query().Get("connector_name"),
		EventTypePrefix: r.URL.Query().Get("event_type_prefix"),
		IncludeGlobal:   includeGlobal,
		Limit:           limit,
	})
	if err != nil {
		if graph.IsGraphTooLarge(err) {
			writeError(w, http.StatusBadRequest, "graph_too_large")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to build topology")
		return
	}
	writeJSON(w, http.StatusOK, topology)
}

func (h *Handler) adminExecutionGraph(w http.ResponseWriter, r *http.Request) {
	if h.graphSvc == nil {
		writeError(w, http.StatusNotImplemented, "graph service unavailable")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	maxDepth, err := optionalInt(r.URL.Query().Get("max_depth"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid max_depth")
		return
	}
	maxEvents, err := optionalInt(r.URL.Query().Get("max_events"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid max_events")
		return
	}
	executionGraph, err := h.graphSvc.BuildExecution(r.Context(), eventID, graph.ExecutionRequest{
		MaxDepth:  maxDepth,
		MaxEvents: maxEvents,
	})
	if err != nil {
		if graph.IsGraphTooLarge(err) {
			writeError(w, http.StatusBadRequest, "graph_too_large")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to build execution graph")
		return
	}
	writeJSON(w, http.StatusOK, executionGraph)
}

func (h *Handler) adminReplayEvent(w http.ResponseWriter, r *http.Request) {
	if !h.adminReplayEnabled {
		writeError(w, http.StatusForbidden, "admin replay disabled")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := h.eventQuerySvc.AdminGet(r.Context(), eventID, false)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get event")
		return
	}
	tenantID := tenant.ID(uuid.MustParse(event.TenantID))
	result, err := h.adminReplaySvc.ReplayEvent(r.Context(), tenantID, eventID)
	if err != nil {
		if errors.Is(err, replay.ErrEventNotFound) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to replay event")
		return
	}
	h.adminAudit(tenantID, "admin.event.replay", "event", &eventID, map[string]any{"reason": strings.TrimSpace(req.Reason)}, r)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) adminReplayEvents(w http.ResponseWriter, r *http.Request) {
	if !h.adminReplayEnabled {
		writeError(w, http.StatusForbidden, "admin replay disabled")
		return
	}
	var req struct {
		TenantID  string `json:"tenant_id"`
		EventType string `json:"event_type"`
		From      string `json:"from"`
		To        string `json:"to"`
		Limit     int    `json:"limit"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	if req.Limit <= 0 || req.Limit > h.adminReplayMaxEvents {
		writeError(w, http.StatusBadRequest, "replay request exceeds configured limits")
		return
	}
	from, err := time.Parse(time.RFC3339, strings.TrimSpace(req.From))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := time.Parse(time.RFC3339, strings.TrimSpace(req.To))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to")
		return
	}
	result, err := h.adminReplaySvc.ReplayQuery(r.Context(), tenantID, replay.QueryRequest{
		From: from,
		To:   to,
		Type: strings.TrimSpace(req.EventType),
	})
	if err != nil {
		switch {
		case errors.Is(err, replay.ErrInvalidWindow), errors.Is(err, replay.ErrReplayLimitExceeded):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to replay events")
		}
		return
	}
	h.adminAudit(tenantID, "admin.event.replay", "event", nil, map[string]any{
		"reason":     strings.TrimSpace(req.Reason),
		"event_type": strings.TrimSpace(req.EventType),
		"limit":      req.Limit,
	}, r)
	writeJSON(w, http.StatusOK, result)
}

func requiredTenantQuery(value string) (tenant.ID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, errors.New("invalid tenant_id")
	}
	return parsed, nil
}

func optionalAdminLimit(value string, max int) (int, error) {
	limit, err := optionalLimit(value)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 50, nil
	}
	if limit > max {
		return max, nil
	}
	return limit, nil
}

func optionalInt(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed < 1 {
		return 0, errors.New("must be at least 1")
	}
	return parsed, nil
}
