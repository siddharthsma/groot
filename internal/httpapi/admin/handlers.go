package admin

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
	"groot/internal/httpapi/common"
	"groot/internal/replay"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

func (h *Handlers) adminTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenants, err := h.state.TenantSvc.ListTenants(r.Context())
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list tenants")
			return
		}
		common.WriteJSON(w, http.StatusOK, tenants)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.TenantSvc.CreateTenant(r.Context(), req.Name)
		if err != nil {
			switch {
			case errors.Is(err, tenant.ErrInvalidTenantName), errors.Is(err, tenant.ErrTenantNameExists):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			default:
				common.WriteError(w, http.StatusInternalServerError, "failed to create tenant")
			}
			return
		}
		h.state.AdminAudit(created.Tenant.ID, "admin.tenant.create", "tenant", &created.Tenant.ID, map[string]any{"name": created.Tenant.Name}, r.Context())
		common.WriteJSON(w, http.StatusOK, map[string]any{
			"tenant_id": created.Tenant.ID.String(),
			"api_key":   created.APIKey,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) adminTenant(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := h.state.TenantSvc.GetTenant(r.Context(), tenantID)
		if err != nil {
			if errors.Is(err, tenant.ErrTenantNotFound) {
				common.WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			common.WriteError(w, http.StatusInternalServerError, "failed to get tenant")
			return
		}
		common.WriteJSON(w, http.StatusOK, record)
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := h.state.TenantSvc.UpdateTenantName(r.Context(), tenantID, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, tenant.ErrInvalidTenantName), errors.Is(err, tenant.ErrTenantNameExists):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, tenant.ErrTenantNotFound):
				common.WriteError(w, http.StatusNotFound, "tenant not found")
			default:
				common.WriteError(w, http.StatusInternalServerError, "failed to update tenant")
			}
			return
		}
		h.state.AdminAudit(tenantID, "admin.tenant.update", "tenant", &tenantID, map[string]any{"name": record.Name}, r.Context())
		common.WriteJSON(w, http.StatusOK, record)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) adminTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := h.state.APIKeySvc.List(r.Context(), tenantID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list api keys")
			return
		}
		common.WriteJSON(w, http.StatusOK, keys)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.APIKeySvc.Create(r.Context(), tenantID, req.Name)
		if err != nil {
			if errors.Is(err, apikey.ErrInvalidName) {
				common.WriteError(w, http.StatusBadRequest, err.Error())
			} else {
				common.WriteError(w, http.StatusInternalServerError, "failed to create api key")
			}
			return
		}
		h.state.AdminAudit(tenantID, "admin.api_key.create", "api_key", &created.ID, map[string]any{"name": created.Name, "key_prefix": created.KeyPrefix}, r.Context())
		common.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":         created.ID.String(),
			"name":       created.Name,
			"api_key":    created.Secret,
			"key_prefix": created.KeyPrefix,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) adminTenantAPIKeyRevoke(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	keyID, err := uuid.Parse(r.PathValue("api_key_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid api_key_id")
		return
	}
	key, err := h.state.APIKeySvc.Revoke(r.Context(), tenantID, keyID)
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "api key not found")
		} else {
			common.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")
		}
		return
	}
	h.state.AdminAudit(tenantID, "admin.api_key.revoke", "api_key", &key.ID, map[string]any{"key_prefix": key.KeyPrefix}, r.Context())
	common.WriteJSON(w, http.StatusOK, key)
}

func (h *Handlers) adminConnectorInstances(w http.ResponseWriter, r *http.Request) {
	var tenantID *tenant.ID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	instances, err := h.state.ConnectorSvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("connector_name"), r.URL.Query().Get("scope"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list connector instances")
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
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) adminConnectorInstance(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		TenantID      string          `json:"tenant_id"`
		ConnectorName string          `json:"connector_name"`
		Scope         string          `json:"scope"`
		Config        json.RawMessage `json:"config"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var tenantID *tenant.ID
	if strings.TrimSpace(req.TenantID) != "" {
		parsed, err := uuid.Parse(req.TenantID)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	instance, err := h.state.ConnectorSvc.AdminUpsert(r.Context(), id, tenantID, req.ConnectorName, req.Scope, req.Config)
	if err != nil {
		switch {
		case errors.Is(err, connectorinstance.ErrUnsupportedConnector), errors.Is(err, connectorinstance.ErrInvalidConfig), errors.Is(err, connectorinstance.ErrMissingBotToken), errors.Is(err, connectorinstance.ErrMissingWebhookSecret), errors.Is(err, connectorinstance.ErrMissingStripeAccount), errors.Is(err, connectorinstance.ErrMissingNotionToken), errors.Is(err, connectorinstance.ErrMissingLLMProviders), errors.Is(err, connectorinstance.ErrInvalidLLMProvider), errors.Is(err, connectorinstance.ErrMissingLLMAPIKey), errors.Is(err, connectorinstance.ErrInvalidScope), errors.Is(err, connectorinstance.ErrGlobalNotAllowed), errors.Is(err, connectorinstance.ErrTenantOnlyConnector), errors.Is(err, connectorinstance.ErrGlobalOnlyConnector), errors.Is(err, connectorinstance.ErrImmutableName), errors.Is(err, connectorinstance.ErrImmutableScope):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			common.WriteError(w, http.StatusInternalServerError, "failed to upsert connector instance")
		}
		return
	}
	targetTenantID := tenant.ID(uuid.Nil)
	if tenantID != nil {
		targetTenantID = *tenantID
	}
	h.state.AdminAudit(targetTenantID, "admin.connector_instance.upsert", "connector_instance", &instance.ID, map[string]any{"connector_name": instance.ConnectorName, "scope": instance.Scope}, r.Context())
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"id":             instance.ID.String(),
		"tenant_id":      instance.TenantID.String(),
		"connector_name": instance.ConnectorName,
		"scope":          instance.Scope,
		"created_at":     instance.CreatedAt,
		"updated_at":     instance.UpdatedAt,
	})
}

func (h *Handlers) adminSubscriptions(w http.ResponseWriter, r *http.Request) {
	var tenantID *tenant.ID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	subs, err := h.state.SubSvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("event_type"), r.URL.Query().Get("destination_type"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	response := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		response = append(response, map[string]any{
			"id":                      sub.ID.String(),
			"tenant_id":               sub.TenantID.String(),
			"destination_type":        sub.DestinationType,
			"connector_instance_id":   common.OptionalUUIDValue(sub.ConnectorInstanceID),
			"connected_app_id":        common.OptionalUUIDValue(sub.ConnectedAppID),
			"function_destination_id": common.OptionalUUIDValue(sub.FunctionDestinationID),
			"operation":               sub.Operation,
			"operation_params":        sub.OperationParams,
			"filter":                  sub.Filter,
			"event_type":              sub.EventType,
			"event_source":            sub.EventSource,
			"emit_success_event":      sub.EmitSuccessEvent,
			"emit_failure_event":      sub.EmitFailureEvent,
			"status":                  sub.Status,
		})
	}
	common.WriteJSON(w, http.StatusOK, response)
}

func (h *Handlers) adminTenantSubscriptions(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	var req struct {
		ConnectedAppID         string          `json:"connected_app_id"`
		DestinationType        string          `json:"destination_type"`
		FunctionDestinationID  string          `json:"function_destination_id"`
		ConnectorInstanceID    string          `json:"connector_instance_id"`
		AgentID                string          `json:"agent_id"`
		SessionKeyTemplate     *string         `json:"session_key_template"`
		SessionCreateIfMissing *bool           `json:"session_create_if_missing"`
		Operation              string          `json:"operation"`
		OperationParams        json.RawMessage `json:"operation_params"`
		Filter                 json.RawMessage `json:"filter"`
		EventType              string          `json:"event_type"`
		EventSource            *string         `json:"event_source"`
		EmitSuccessEvent       bool            `json:"emit_success_event"`
		EmitFailureEvent       bool            `json:"emit_failure_event"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	appID, functionID, connectorInstanceID, agentID, operation, err := common.ParseSubscriptionRequestFields(req.ConnectedAppID, req.FunctionDestinationID, req.ConnectorInstanceID, req.AgentID, req.Operation)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	sessionCreateIfMissing := true
	if req.SessionCreateIfMissing != nil {
		sessionCreateIfMissing = *req.SessionCreateIfMissing
	}
	result, err := h.state.SubSvc.Create(r.Context(), tenantID, req.DestinationType, appID, functionID, connectorInstanceID, agentID, req.SessionKeyTemplate, sessionCreateIfMissing, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		case common.IsFilterValidationError(err):
			common.WriteSubscriptionFilterError(w, err)
		case errors.Is(err, subscription.ErrConnectedAppNotFound), errors.Is(err, subscription.ErrFunctionDestinationNotFound), errors.Is(err, subscription.ErrConnectorInstanceNotFound), errors.Is(err, subscription.ErrConnectorInstanceForbidden):
			common.WriteError(w, http.StatusNotFound, err.Error())
		default:
			common.WriteError(w, http.StatusInternalServerError, "failed to create subscription")
		}
		return
	}
	h.state.AdminAudit(tenantID, "admin.subscription.create_for_tenant", "subscription", &result.Subscription.ID, map[string]any{"destination_type": result.Subscription.DestinationType, "event_type": result.Subscription.EventType}, r.Context())
	common.WriteJSON(w, http.StatusCreated, common.SubscriptionResponse(result))
}

func requiredTenantQuery(value string) (tenant.ID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, errors.New("invalid tenant_id")
	}
	return parsed, nil
}

func (h *Handlers) adminEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requiredTenantQuery(r.URL.Query().Get("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := common.OptionalTime(r.URL.Query().Get("from"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := common.OptionalTime(r.URL.Query().Get("to"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid to")
		return
	}
	limit, err := common.OptionalAdminLimit(r.URL.Query().Get("limit"), 500)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	events, err := h.state.EventQuerySvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("event_type"), from, to, limit, h.state.AdminAllowViewPayloads)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	common.WriteJSON(w, http.StatusOK, events)
}

func (h *Handlers) adminDeliveryJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, err := requiredTenantQuery(r.URL.Query().Get("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := common.OptionalTime(r.URL.Query().Get("from"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := common.OptionalTime(r.URL.Query().Get("to"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid to")
		return
	}
	limit, err := common.OptionalAdminLimit(r.URL.Query().Get("limit"), 500)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	jobs, err := h.state.DeliverySvc.AdminList(r.Context(), tenantID, r.URL.Query().Get("status"), from, to, limit)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list delivery jobs")
		return
	}
	common.WriteJSON(w, http.StatusOK, common.MapJobs(jobs))
}

func (h *Handlers) adminTopology(w http.ResponseWriter, r *http.Request) {
	if h.state.GraphSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "graph service unavailable")
		return
	}
	var tenantID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("tenant_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
			return
		}
		tenantID = &parsed
	}
	includeGlobal := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_global")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid include_global")
			return
		}
		includeGlobal = parsed
	}
	limit, err := common.OptionalPositiveInt(r.URL.Query().Get("limit"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	topology, err := h.state.GraphSvc.BuildTopology(r.Context(), graph.TopologyRequest{
		TenantID:        tenantID,
		ConnectorName:   r.URL.Query().Get("connector_name"),
		EventTypePrefix: r.URL.Query().Get("event_type_prefix"),
		IncludeGlobal:   includeGlobal,
		Limit:           limit,
	})
	if err != nil {
		if graph.IsGraphTooLarge(err) {
			common.WriteError(w, http.StatusBadRequest, "graph_too_large")
		} else {
			common.WriteError(w, http.StatusInternalServerError, "failed to build topology")
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, topology)
}

func (h *Handlers) adminExecutionGraph(w http.ResponseWriter, r *http.Request) {
	if h.state.GraphSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "graph service unavailable")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	maxDepth, err := common.OptionalPositiveInt(r.URL.Query().Get("max_depth"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid max_depth")
		return
	}
	maxEvents, err := common.OptionalPositiveInt(r.URL.Query().Get("max_events"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid max_events")
		return
	}
	executionGraph, err := h.state.GraphSvc.BuildExecution(r.Context(), eventID, graph.ExecutionRequest{MaxDepth: maxDepth, MaxEvents: maxEvents})
	if err != nil {
		switch {
		case graph.IsGraphTooLarge(err):
			common.WriteError(w, http.StatusBadRequest, "graph_too_large")
		case errors.Is(err, sql.ErrNoRows):
			common.WriteError(w, http.StatusNotFound, "event not found")
		default:
			common.WriteError(w, http.StatusInternalServerError, "failed to build execution graph")
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, executionGraph)
}

func (h *Handlers) adminReplayEvent(w http.ResponseWriter, r *http.Request) {
	if !h.state.AdminReplayEnabled {
		common.WriteError(w, http.StatusForbidden, "admin replay disabled")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := common.DecodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, err := h.state.EventQuerySvc.AdminGet(r.Context(), eventID, false)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			common.WriteError(w, http.StatusNotFound, "event not found")
		default:
			common.WriteError(w, http.StatusInternalServerError, "failed to get event")
		}
		return
	}
	tenantID := tenant.ID(uuid.MustParse(event.TenantID))
	result, err := h.state.AdminReplaySvc.ReplayEvent(r.Context(), tenantID, eventID)
	if err != nil {
		if errors.Is(err, replay.ErrEventNotFound) {
			common.WriteError(w, http.StatusNotFound, "event not found")
		} else {
			common.WriteError(w, http.StatusInternalServerError, "failed to replay event")
		}
		return
	}
	h.state.AdminAudit(tenantID, "admin.event.replay", "event", &eventID, map[string]any{"reason": strings.TrimSpace(req.Reason)}, r.Context())
	common.WriteJSON(w, http.StatusOK, result)
}

func (h *Handlers) adminReplayEvents(w http.ResponseWriter, r *http.Request) {
	if !h.state.AdminReplayEnabled {
		common.WriteError(w, http.StatusForbidden, "admin replay disabled")
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
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	if req.Limit <= 0 || req.Limit > h.state.AdminReplayMaxEvents {
		common.WriteError(w, http.StatusBadRequest, "replay request exceeds configured limits")
		return
	}
	from, err := time.Parse(time.RFC3339, strings.TrimSpace(req.From))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := time.Parse(time.RFC3339, strings.TrimSpace(req.To))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid to")
		return
	}
	result, err := h.state.AdminReplaySvc.ReplayQuery(r.Context(), tenantID, replay.QueryRequest{
		From: from,
		To:   to,
		Type: strings.TrimSpace(req.EventType),
	})
	if err != nil {
		if errors.Is(err, replay.ErrInvalidWindow) || errors.Is(err, replay.ErrReplayLimitExceeded) {
			common.WriteError(w, http.StatusBadRequest, err.Error())
		} else {
			common.WriteError(w, http.StatusInternalServerError, "failed to replay events")
		}
		return
	}
	h.state.AdminAudit(tenantID, "admin.event.replay", "event", nil, map[string]any{
		"reason":     strings.TrimSpace(req.Reason),
		"event_type": strings.TrimSpace(req.EventType),
		"limit":      req.Limit,
	}, r.Context())
	common.WriteJSON(w, http.StatusOK, result)
}
