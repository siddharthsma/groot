package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/apikey"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	stripeconnector "groot/internal/connectors/providers/stripe"
	"groot/internal/delivery"
	"groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/httpapi/common"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/replay"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

func (h *Handlers) createTenant(w http.ResponseWriter, r *http.Request) {
	if h.state.TenantSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}
	if !h.state.EditionRuntime.Capabilities.TenantCreationAllowed {
		common.WriteError(w, http.StatusForbidden, "community_edition_restriction")
		return
	}
	if h.state.EditionRuntime.MaxTenants > 0 {
		tenants, err := h.state.TenantSvc.ListTenants(r.Context())
		if err != nil {
			h.state.Logger.Error("list tenants for tenant creation limit", slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to validate tenant limit")
			return
		}
		if len(tenants) >= h.state.EditionRuntime.MaxTenants {
			common.WriteError(w, http.StatusForbidden, "tenant_limit_exceeded")
			return
		}
	}

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
			h.state.Logger.Error("create tenant", slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to create tenant")
		}
		return
	}

	h.state.Logger.Info("tenant_created", slog.String("tenant_id", created.Tenant.ID.String()))
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"tenant_id": created.Tenant.ID.String(),
		"api_key":   created.APIKey,
	})
}

func (h *Handlers) listTenants(w http.ResponseWriter, r *http.Request) {
	if h.state.TenantSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}
	tenants, err := h.state.TenantSvc.ListTenants(r.Context())
	if err != nil {
		h.state.Logger.Error("list tenants", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	common.WriteJSON(w, http.StatusOK, tenants)
}

func (h *Handlers) getTenant(w http.ResponseWriter, r *http.Request) {
	if h.state.TenantSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}
	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	record, err := h.state.TenantSvc.GetTenant(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, tenant.ErrTenantNotFound) {
			common.WriteError(w, http.StatusNotFound, "tenant not found")
			return
		}
		h.state.Logger.Error("get tenant", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	common.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) apiKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.APIKeySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "api key service unavailable")
		return
	}

	switch r.Method {
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
				h.state.Logger.Error("create api key", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create api key")
			}
			return
		}
		h.state.Audit("api_key.create", "api_key", &created.ID, map[string]any{
			"name":       created.Name,
			"key_prefix": created.KeyPrefix,
		}, r.Context())
		common.WriteJSON(w, http.StatusCreated, map[string]any{
			"id":         created.ID.String(),
			"name":       created.Name,
			"api_key":    created.Secret,
			"key_prefix": created.KeyPrefix,
		})
	case http.MethodGet:
		keys, err := h.state.APIKeySvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list api keys", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list api keys")
			return
		}
		common.WriteJSON(w, http.StatusOK, keys)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.APIKeySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "api key service unavailable")
		return
	}
	id, err := uuid.Parse(r.PathValue("api_key_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid api_key_id")
		return
	}
	key, err := h.state.APIKeySvc.Revoke(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "api key not found")
		} else {
			h.state.Logger.Error("revoke api key", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to revoke api key")
		}
		return
	}
	h.state.Audit("api_key.revoke", "api_key", &key.ID, map[string]any{"key_prefix": key.KeyPrefix}, r.Context())
	common.WriteJSON(w, http.StatusOK, key)
}

func (h *Handlers) createEvent(w http.ResponseWriter, r *http.Request) {
	if h.state.EventSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "event service unavailable")
		return
	}
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Type    string          `json:"type"`
		Source  string          `json:"source"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.state.Metrics != nil {
		h.state.Metrics.IncEventsReceived()
	}

	event, err := h.state.EventSvc.Ingest(r.Context(), ingest.Request{
		TenantID:   tenantID,
		Type:       req.Type,
		Source:     req.Source,
		SourceKind: event.SourceKindExternal,
		ChainDepth: 0,
		Payload:    req.Payload,
	})
	if err != nil {
		switch {
		case errors.Is(err, ingest.ErrInvalidType), errors.Is(err, ingest.ErrInvalidSource), errors.Is(err, ingest.ErrInvalidSourceKind), errors.Is(err, ingest.ErrInvalidChainDepth), errors.Is(err, ingest.ErrInvalidVersionedType):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		case common.IsSchemaReject(err):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			h.state.Logger.Error("event_publish_failed", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to publish event")
		}
		return
	}

	h.state.Logger.Info("event_received", slog.String("event_id", event.EventID.String()), slog.String("tenant_id", event.TenantID.String()), slog.String("event_type", event.Type))
	common.WriteJSON(w, http.StatusOK, map[string]string{"event_id": event.EventID.String()})
}

func (h *Handlers) listEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.EventQuerySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "event query service unavailable")
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
	limit, err := common.OptionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	events, err := h.state.EventQuerySvc.List(r.Context(), tenantID, r.URL.Query().Get("type"), r.URL.Query().Get("source"), from, to, limit)
	if err != nil {
		h.state.Logger.Error("list events", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	h.state.Logger.Info("events_listed", slog.String("tenant_id", tenantID.String()), slog.Int("count", len(events)))
	common.WriteJSON(w, http.StatusOK, events)
}

func (h *Handlers) connectedApps(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.AppSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "connected app service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name           string `json:"name"`
			DestinationURL string `json:"destination_url"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		app, err := h.state.AppSvc.Create(r.Context(), tenantID, req.Name, req.DestinationURL)
		if err != nil {
			if errors.Is(err, connectedapp.ErrInvalidName) || errors.Is(err, connectedapp.ErrInvalidDestinationURL) {
				common.WriteError(w, http.StatusBadRequest, err.Error())
			} else {
				h.state.Logger.Error("create connected app", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create connected app")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{
			"id":              app.ID.String(),
			"name":            app.Name,
			"destination_url": app.DestinationURL,
		})
	case http.MethodGet:
		apps, err := h.state.AppSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list connected apps", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list connected apps")
			return
		}
		common.WriteJSON(w, http.StatusOK, apps)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) agents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.AgentSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req agent.CreateRequest
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.AgentSvc.Create(r.Context(), tenantID, req)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrInvalidName), errors.Is(err, agent.ErrInvalidInstructions), errors.Is(err, agent.ErrInvalidAllowedTools), errors.Is(err, agent.ErrDuplicateName):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, agent.ErrFunctionDestinationMissing):
				common.WriteError(w, http.StatusNotFound, "function destination not found")
			default:
				h.state.Logger.Error("create agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create agent")
			}
			return
		}
		h.state.Audit("agent.create", "agent", &created.ID, map[string]any{"name": created.Name}, r.Context())
		common.WriteJSON(w, http.StatusCreated, created)
	case http.MethodGet:
		records, err := h.state.AgentSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list agents", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list agents")
			return
		}
		common.WriteJSON(w, http.StatusOK, records)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) agent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.AgentSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}
	agentID, err := uuid.Parse(r.PathValue("agent_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.state.AgentSvc.Get(r.Context(), tenantID, agentID)
		if err != nil {
			if errors.Is(err, agent.ErrNotFound) {
				common.WriteError(w, http.StatusNotFound, "agent not found")
			} else {
				h.state.Logger.Error("get agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to get agent")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var req agent.CreateRequest
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := h.state.AgentSvc.Update(r.Context(), tenantID, agentID, req)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrNotFound):
				common.WriteError(w, http.StatusNotFound, "agent not found")
			case errors.Is(err, agent.ErrInvalidName), errors.Is(err, agent.ErrInvalidInstructions), errors.Is(err, agent.ErrInvalidAllowedTools), errors.Is(err, agent.ErrDuplicateName):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, agent.ErrFunctionDestinationMissing):
				common.WriteError(w, http.StatusNotFound, "function destination not found")
			default:
				h.state.Logger.Error("update agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to update agent")
			}
			return
		}
		h.state.Audit("agent.update", "agent", &updated.ID, map[string]any{"name": updated.Name}, r.Context())
		common.WriteJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.state.AgentSvc.Delete(r.Context(), tenantID, agentID); err != nil {
			switch {
			case errors.Is(err, agent.ErrNotFound):
				common.WriteError(w, http.StatusNotFound, "agent not found")
			case errors.Is(err, agent.ErrSubscriptionReferences), errors.Is(err, agent.ErrActiveSessionsExist):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			default:
				h.state.Logger.Error("delete agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to delete agent")
			}
			return
		}
		h.state.Audit("agent.delete", "agent", &agentID, nil, r.Context())
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) agentSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.AgentSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}
	var agentID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("agent_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		agentID = &parsed
	}
	limit, err := common.OptionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	records, err := h.state.AgentSvc.ListSessions(r.Context(), tenantID, agentID, r.URL.Query().Get("status"), limit)
	if err != nil {
		h.state.Logger.Error("list agent sessions", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list agent sessions")
		return
	}
	common.WriteJSON(w, http.StatusOK, records)
}

func (h *Handlers) agentSession(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.AgentSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}
	sessionID, err := uuid.Parse(r.PathValue("session_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid session_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := h.state.AgentSvc.GetSession(r.Context(), tenantID, sessionID)
		if err != nil {
			if errors.Is(err, agent.ErrSessionNotFound) {
				common.WriteError(w, http.StatusNotFound, "agent session not found")
			} else {
				h.state.Logger.Error("get agent session", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to get agent session")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, record)
	case http.MethodPost:
		record, err := h.state.AgentSvc.CloseSession(r.Context(), tenantID, sessionID)
		if err != nil {
			if errors.Is(err, agent.ErrSessionNotFound) {
				common.WriteError(w, http.StatusNotFound, "agent session not found")
			} else {
				h.state.Logger.Error("close agent session", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to close agent session")
			}
			return
		}
		h.state.Audit("agent_session.close", "agent_session", &record.ID, map[string]any{"agent_id": record.AgentID.String()}, r.Context())
		common.WriteJSON(w, http.StatusOK, record)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) subscriptions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.SubSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "subscription service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req subscriptionRequest
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := h.createOrUpdateSubscription(r.Context(), tenantID, uuid.Nil, req, false)
		if err != nil {
			h.handleSubscriptionError(w, tenantID, "create subscription", err)
			return
		}
		h.state.Audit("subscription.create", "subscription", &result.Subscription.ID, map[string]any{
			"destination_type": result.Subscription.DestinationType,
			"event_type":       result.Subscription.EventType,
		}, r.Context())
		common.WriteJSON(w, http.StatusCreated, common.SubscriptionResponse(result))
	case http.MethodGet:
		subs, err := h.state.SubSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list subscriptions", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list subscriptions")
			return
		}
		common.WriteJSON(w, http.StatusOK, subs)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) replaceSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.SubSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "subscription service unavailable")
		return
	}
	subscriptionID, err := uuid.Parse(r.PathValue("subscription_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}
	var req subscriptionRequest
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.createOrUpdateSubscription(r.Context(), tenantID, subscriptionID, req, true)
	if err != nil {
		h.handleReplaceSubscriptionError(w, tenantID, err)
		return
	}
	h.state.Audit("subscription.update", "subscription", &result.Subscription.ID, map[string]any{
		"destination_type": result.Subscription.DestinationType,
		"event_type":       result.Subscription.EventType,
	}, r.Context())
	common.WriteJSON(w, http.StatusOK, common.SubscriptionResponse(result))
}

type subscriptionRequest struct {
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

func (h *Handlers) createOrUpdateSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, req subscriptionRequest, replace bool) (subscription.Result, error) {
	appID, functionID, connectorInstanceID, agentID, operation, err := common.ParseSubscriptionRequestFields(req.ConnectedAppID, req.FunctionDestinationID, req.ConnectorInstanceID, req.AgentID, req.Operation)
	if err != nil {
		return subscription.Result{}, err
	}
	sessionCreateIfMissing := true
	if req.SessionCreateIfMissing != nil {
		sessionCreateIfMissing = *req.SessionCreateIfMissing
	}
	if replace {
		return h.state.SubSvc.Update(ctx, tenantID, subscriptionID, req.DestinationType, appID, functionID, connectorInstanceID, agentID, req.SessionKeyTemplate, sessionCreateIfMissing, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
	}
	return h.state.SubSvc.Create(ctx, tenantID, req.DestinationType, appID, functionID, connectorInstanceID, agentID, req.SessionKeyTemplate, sessionCreateIfMissing, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
}

func (h *Handlers) handleSubscriptionError(w http.ResponseWriter, tenantID tenant.ID, logMsg string, err error) {
	switch {
	case strings.HasPrefix(err.Error(), "invalid ") && (strings.Contains(err.Error(), "_id") || strings.Contains(err.Error(), "agent_id")):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case common.IsFilterValidationError(err):
		common.WriteSubscriptionFilterError(w, err)
	case errors.Is(err, subscription.ErrConnectedAppNotFound):
		common.WriteError(w, http.StatusNotFound, "connected app not found")
	case errors.Is(err, subscription.ErrFunctionDestinationNotFound):
		common.WriteError(w, http.StatusNotFound, "function destination not found")
	case errors.Is(err, subscription.ErrConnectorInstanceNotFound), errors.Is(err, subscription.ErrConnectorInstanceForbidden):
		common.WriteError(w, http.StatusNotFound, "connector instance not found")
	case errors.Is(err, subscription.ErrGlobalConnectorNotAllowed):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		h.state.Logger.Error(logMsg, slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to create subscription")
	}
}

func (h *Handlers) handleReplaceSubscriptionError(w http.ResponseWriter, tenantID tenant.ID, err error) {
	switch {
	case strings.HasPrefix(err.Error(), "invalid ") && (strings.Contains(err.Error(), "_id") || strings.Contains(err.Error(), "agent_id")):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	case common.IsFilterValidationError(err):
		common.WriteSubscriptionFilterError(w, err)
	case errors.Is(err, subscription.ErrConnectedAppNotFound):
		common.WriteError(w, http.StatusNotFound, "connected app not found")
	case errors.Is(err, subscription.ErrFunctionDestinationNotFound):
		common.WriteError(w, http.StatusNotFound, "function destination not found")
	case errors.Is(err, subscription.ErrConnectorInstanceNotFound), errors.Is(err, subscription.ErrConnectorInstanceForbidden), errors.Is(err, subscription.ErrSubscriptionNotFound):
		common.WriteError(w, http.StatusNotFound, "subscription not found")
	case errors.Is(err, subscription.ErrGlobalConnectorNotAllowed):
		common.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		h.state.Logger.Error("replace subscription", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to replace subscription")
	}
}

func (h *Handlers) connectorInstances(w http.ResponseWriter, r *http.Request) {
	if h.state.ConnectorSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "connector instance service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectorName string          `json:"connector_name"`
			Scope         string          `json:"scope"`
			Config        json.RawMessage `json:"config"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		var tenantID *tenant.ID
		switch strings.TrimSpace(req.Scope) {
		case "", connectorinstance.ScopeTenant:
			resolvedTenantID, ok := tenantIDFromContext(r.Context())
			if !ok {
				authenticated, resolved, authenticatedOK := h.authenticateTenantRequest(r)
				if authenticatedOK {
					r = authenticated
					resolvedTenantID = resolved
				}
				ok = authenticatedOK
			}
			if !ok {
				common.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tenantID = &resolvedTenantID
		case connectorinstance.ScopeGlobal:
			if common.BearerToken(r.Header.Get("Authorization")) != h.state.SystemAPIKey {
				common.WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		default:
			common.WriteError(w, http.StatusBadRequest, connectorinstance.ErrInvalidScope.Error())
			return
		}

		instance, err := h.state.ConnectorSvc.Create(r.Context(), tenantID, req.ConnectorName, req.Scope, req.Config)
		if err != nil {
			switch {
			case errors.Is(err, connectorinstance.ErrUnsupportedConnector), errors.Is(err, connectorinstance.ErrInvalidConfig), errors.Is(err, connectorinstance.ErrMissingBotToken), errors.Is(err, connectorinstance.ErrMissingWebhookSecret), errors.Is(err, connectorinstance.ErrMissingStripeAccount), errors.Is(err, connectorinstance.ErrMissingNotionToken), errors.Is(err, connectorinstance.ErrMissingLLMProviders), errors.Is(err, connectorinstance.ErrInvalidLLMProvider), errors.Is(err, connectorinstance.ErrMissingLLMAPIKey), errors.Is(err, connectorinstance.ErrInvalidScope), errors.Is(err, connectorinstance.ErrGlobalNotAllowed), errors.Is(err, connectorinstance.ErrTenantOnlyConnector), errors.Is(err, connectorinstance.ErrGlobalOnlyConnector):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, connectorinstance.ErrDuplicateInstance):
				common.WriteError(w, http.StatusConflict, err.Error())
			default:
				h.state.Logger.Error("create connector instance", slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create connector instance")
			}
			return
		}
		h.state.Audit("connector_instance.create", "connector_instance", &instance.ID, map[string]any{"connector_name": instance.ConnectorName, "scope": instance.Scope}, r.Context())
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": instance.ID.String()})
	case http.MethodGet:
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			common.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		instances, err := h.state.ConnectorSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list connector instances", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list connector instances")
			return
		}
		common.WriteJSON(w, http.StatusOK, instances)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) inboundRoutes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.InboundRouteSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "inbound route service unavailable")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectorName       string `json:"connector_name"`
			RouteKey            string `json:"route_key"`
			ConnectorInstanceID string `json:"connector_instance_id"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		var connectorInstanceID *uuid.UUID
		if strings.TrimSpace(req.ConnectorInstanceID) != "" {
			parsed, err := uuid.Parse(req.ConnectorInstanceID)
			if err != nil {
				common.WriteError(w, http.StatusBadRequest, "invalid connector_instance_id")
				return
			}
			connectorInstanceID = &parsed
		}
		route, err := h.state.InboundRouteSvc.Create(r.Context(), tenantID, req.ConnectorName, req.RouteKey, connectorInstanceID)
		if err != nil {
			switch {
			case errors.Is(err, inboundroute.ErrInvalidConnectorName), errors.Is(err, inboundroute.ErrInvalidRouteKey), errors.Is(err, inboundroute.ErrInvalidConnectorInstance):
				common.WriteError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, inboundroute.ErrConnectorInstanceNotFound):
				common.WriteError(w, http.StatusNotFound, "connector instance not found")
			case errors.Is(err, inboundroute.ErrDuplicateRoute):
				common.WriteError(w, http.StatusConflict, err.Error())
			default:
				h.state.Logger.Error("create inbound route", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create inbound route")
			}
			return
		}
		h.state.Logger.Info("inbound_route_created", slog.String("tenant_id", tenantID.String()), slog.String("connector_name", route.ConnectorName), slog.String("route_key", route.RouteKey))
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": route.ID.String()})
	case http.MethodGet:
		routes, err := h.state.InboundRouteSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list inbound routes", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list inbound routes")
			return
		}
		common.WriteJSON(w, http.StatusOK, routes)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) functions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.FunctionSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "function destination service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := common.DecodeJSON(r, &req); err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.state.FunctionSvc.Create(r.Context(), tenantID, req.Name, req.URL)
		if err != nil {
			if errors.Is(err, functiondestination.ErrInvalidName) || errors.Is(err, functiondestination.ErrInvalidURL) {
				common.WriteError(w, http.StatusBadRequest, err.Error())
			} else {
				h.state.Logger.Error("create function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to create function destination")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]any{
			"id":     created.Destination.ID.String(),
			"name":   created.Destination.Name,
			"url":    created.Destination.URL,
			"secret": created.Secret,
		})
	case http.MethodGet:
		destinations, err := h.state.FunctionSvc.List(r.Context(), tenantID)
		if err != nil {
			h.state.Logger.Error("list function destinations", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to list function destinations")
			return
		}
		response := make([]map[string]string, 0, len(destinations))
		for _, destination := range destinations {
			response = append(response, map[string]string{
				"id":   destination.ID.String(),
				"name": destination.Name,
				"url":  destination.URL,
			})
		}
		common.WriteJSON(w, http.StatusOK, response)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) function(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.FunctionSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "function destination service unavailable")
		return
	}
	functionID, err := uuid.Parse(r.PathValue("function_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid function_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		destination, err := h.state.FunctionSvc.Get(r.Context(), tenantID, functionID)
		if err != nil {
			if errors.Is(err, functiondestination.ErrNotFound) {
				common.WriteError(w, http.StatusNotFound, "function destination not found")
			} else {
				h.state.Logger.Error("get function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to get function destination")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": destination.ID.String(), "name": destination.Name, "url": destination.URL})
	case http.MethodDelete:
		err := h.state.FunctionSvc.Delete(r.Context(), tenantID, functionID)
		if err != nil {
			switch {
			case errors.Is(err, functiondestination.ErrNotFound):
				common.WriteError(w, http.StatusNotFound, "function destination not found")
			case errors.Is(err, functiondestination.ErrInUse):
				common.WriteError(w, http.StatusBadRequest, "function destination has active subscriptions")
			default:
				h.state.Logger.Error("delete function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				common.WriteError(w, http.StatusInternalServerError, "failed to delete function destination")
			}
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) subscriptionStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.SubSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "subscription service unavailable")
		return
	}
	subscriptionID, err := uuid.Parse(r.PathValue("subscription_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}

	var sub subscription.Subscription
	switch {
	case strings.HasSuffix(r.URL.Path, "/pause"):
		sub, err = h.state.SubSvc.Pause(r.Context(), tenantID, subscriptionID)
		if err == nil {
			h.state.Logger.Info("subscription_paused", slog.String("tenant_id", tenantID.String()), slog.String("subscription_id", subscriptionID.String()))
		}
	case strings.HasSuffix(r.URL.Path, "/resume"):
		sub, err = h.state.SubSvc.Resume(r.Context(), tenantID, subscriptionID)
		if err == nil {
			h.state.Logger.Info("subscription_resumed", slog.String("tenant_id", tenantID.String()), slog.String("subscription_id", subscriptionID.String()))
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		if errors.Is(err, subscription.ErrSubscriptionNotFound) {
			common.WriteError(w, http.StatusNotFound, "subscription not found")
		} else {
			h.state.Logger.Error("update subscription status", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to update subscription")
		}
		return
	}
	action := "subscription.resume"
	if strings.HasSuffix(r.URL.Path, "/pause") {
		action = "subscription.pause"
	}
	h.state.Audit(action, "subscription", &sub.ID, map[string]any{"status": sub.Status}, r.Context())
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": sub.Status})
}

func (h *Handlers) deliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.DeliverySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}
	limit, err := common.OptionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	subscriptionID, err := common.OptionalUUID(r.URL.Query().Get("subscription_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}
	eventID, err := common.OptionalUUID(r.URL.Query().Get("event_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid event_id")
		return
	}

	jobs, err := h.state.DeliverySvc.List(r.Context(), tenantID, r.URL.Query().Get("status"), subscriptionID, eventID, limit)
	if err != nil {
		h.state.Logger.Error("list deliveries", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to list deliveries")
		return
	}
	h.state.Logger.Info("deliveries_listed", slog.String("tenant_id", tenantID.String()), slog.Int("count", len(jobs)))
	common.WriteJSON(w, http.StatusOK, common.MapJobs(jobs))
}

func (h *Handlers) delivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.DeliverySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}
	deliveryID, err := uuid.Parse(r.PathValue("delivery_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid delivery_id")
		return
	}
	job, err := h.state.DeliverySvc.Get(r.Context(), tenantID, deliveryID)
	if err != nil {
		if errors.Is(err, delivery.ErrJobNotFound) {
			common.WriteError(w, http.StatusNotFound, "delivery not found")
		} else {
			h.state.Logger.Error("get delivery", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to get delivery")
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, common.MapJob(job))
}

func (h *Handlers) retryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.DeliverySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}
	deliveryID, err := uuid.Parse(r.PathValue("delivery_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid delivery_id")
		return
	}
	job, err := h.state.DeliverySvc.Retry(r.Context(), tenantID, deliveryID)
	if err != nil {
		switch {
		case errors.Is(err, delivery.ErrJobNotFound):
			common.WriteError(w, http.StatusNotFound, "delivery not found")
		case errors.Is(err, delivery.ErrRetryNotAllowed):
			common.WriteError(w, http.StatusBadRequest, "delivery retry not allowed")
		default:
			h.state.Logger.Error("retry delivery", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to retry delivery")
		}
		return
	}
	h.state.Logger.Info("delivery_retried", slog.String("tenant_id", tenantID.String()), slog.String("delivery_id", deliveryID.String()))
	h.state.Logger.Info("delivery_retry_requested", slog.String("tenant_id", tenantID.String()), slog.String("delivery_id", deliveryID.String()))
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": job.Status})
}

func (h *Handlers) replayEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.ReplaySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "replay service unavailable")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	h.state.Logger.Info("event_replay_single_requested", slog.String("tenant_id", tenantID.String()), slog.String("event_id", eventID.String()))
	result, err := h.state.ReplaySvc.ReplayEvent(r.Context(), tenantID, eventID)
	if err != nil {
		if errors.Is(err, replay.ErrEventNotFound) {
			common.WriteError(w, http.StatusNotFound, "event not found")
		} else {
			h.state.Logger.Error("replay event", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to replay event")
		}
		return
	}
	h.state.Logger.Info("event_replay_completed", slog.String("tenant_id", tenantID.String()), slog.String("event_id", result.EventID.String()), slog.Int("jobs_created", result.JobsCreated))
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"event_id":              result.EventID.String(),
		"matched_subscriptions": result.MatchedSubscriptions,
		"jobs_created":          result.JobsCreated,
	})
}

func (h *Handlers) replayEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.ReplaySvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "replay service unavailable")
		return
	}
	var req struct {
		From           string `json:"from"`
		To             string `json:"to"`
		Type           string `json:"type"`
		Source         string `json:"source"`
		SubscriptionID string `json:"subscription_id"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
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
	var subscriptionID *uuid.UUID
	if strings.TrimSpace(req.SubscriptionID) != "" {
		parsed, err := uuid.Parse(req.SubscriptionID)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid subscription_id")
			return
		}
		subscriptionID = &parsed
	}
	h.state.Logger.Info("event_replay_query_requested", slog.String("tenant_id", tenantID.String()), slog.String("from", from.Format(time.RFC3339)), slog.String("to", to.Format(time.RFC3339)))
	result, err := h.state.ReplaySvc.ReplayQuery(r.Context(), tenantID, replay.QueryRequest{
		From:           from,
		To:             to,
		Type:           strings.TrimSpace(req.Type),
		Source:         strings.TrimSpace(req.Source),
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		switch {
		case errors.Is(err, replay.ErrInvalidWindow), errors.Is(err, replay.ErrReplayLimitExceeded), errors.Is(err, replay.ErrSubscriptionInactive):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, replay.ErrSubscriptionNotFound):
			common.WriteError(w, http.StatusNotFound, "subscription not found")
		default:
			h.state.Logger.Error("replay events", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to replay events")
		}
		return
	}
	h.state.Logger.Info("event_replay_completed", slog.String("tenant_id", tenantID.String()), slog.String("from", from.Format(time.RFC3339)), slog.String("to", to.Format(time.RFC3339)), slog.Int("events_scanned", result.EventsScanned), slog.Int("jobs_created", result.JobsCreated))
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"events_scanned": result.EventsScanned,
		"jobs_created":   result.JobsCreated,
	})
}

func (h *Handlers) resendEnable(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.ResendSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	result, err := h.state.ResendSvc.Enable(r.Context(), tenantID)
	if err != nil {
		h.state.Logger.Error("enable resend connector", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to enable resend connector")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"address": result.Address})
}

func (h *Handlers) stripeEnable(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.state.StripeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "stripe service unavailable")
		return
	}
	var req struct {
		StripeAccountID string `json:"stripe_account_id"`
		WebhookSecret   string `json:"webhook_secret"`
	}
	if err := common.DecodeJSON(r, &req); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	instanceID, err := h.state.StripeSvc.Enable(r.Context(), tenantID, req.StripeAccountID, req.WebhookSecret)
	if err != nil {
		switch {
		case errors.Is(err, stripeconnector.ErrInvalidAccountID), errors.Is(err, stripeconnector.ErrInvalidSecret):
			common.WriteError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, stripeconnector.ErrRouteConflict):
			common.WriteError(w, http.StatusConflict, err.Error())
		default:
			h.state.Logger.Error("enable stripe connector", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			common.WriteError(w, http.StatusInternalServerError, "failed to enable stripe connector")
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"connector_instance_id": instanceID.String()})
}
