package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/resend"
	"groot/internal/delivery"
	"groot/internal/eventquery"
	"groot/internal/functiondestination"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type Checker interface {
	Check(context.Context) error
}

type NamedChecker struct {
	Name    string
	Checker Checker
}

type TenantService interface {
	CreateTenant(context.Context, string) (tenant.CreatedTenant, error)
	ListTenants(context.Context) ([]tenant.Tenant, error)
	GetTenant(context.Context, uuid.UUID) (tenant.Tenant, error)
	Authenticate(context.Context, string) (tenant.Tenant, error)
}

type EventService interface {
	Ingest(context.Context, ingest.Request) (stream.Event, error)
}

type EventQueryService interface {
	List(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]eventquery.Event, error)
}

type ConnectedAppService interface {
	Create(context.Context, tenant.ID, string, string) (connectedapp.App, error)
	List(context.Context, tenant.ID) ([]connectedapp.App, error)
	Get(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type FunctionDestinationService interface {
	Create(context.Context, tenant.ID, string, string) (functiondestination.CreatedDestination, error)
	List(context.Context, tenant.ID) ([]functiondestination.Destination, error)
	Get(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
	Delete(context.Context, tenant.ID, uuid.UUID) error
}

type SubscriptionService interface {
	Create(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, json.RawMessage, string, *string) (subscription.Subscription, error)
	List(context.Context, tenant.ID) ([]subscription.Subscription, error)
	Pause(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
	Resume(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
}

type ConnectorInstanceService interface {
	Create(context.Context, tenant.ID, string, json.RawMessage) (connectorinstance.Instance, error)
	List(context.Context, tenant.ID) ([]connectorinstance.Instance, error)
}

type DeliveryService interface {
	List(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]delivery.Job, error)
	Get(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
	Retry(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
}

type ResendService interface {
	Bootstrap(context.Context) (string, error)
	Enable(context.Context, tenant.ID) (resend.EnableResult, error)
	HandleWebhook(context.Context, []byte, http.Header) error
}

type Handler struct {
	logger           *slog.Logger
	checkers         []NamedChecker
	routerCheckers   []NamedChecker
	deliveryCheckers []NamedChecker
	tenantSvc        TenantService
	eventSvc         EventService
	eventQuerySvc    EventQueryService
	appSvc           ConnectedAppService
	functionSvc      FunctionDestinationService
	subSvc           SubscriptionService
	connectorSvc     ConnectorInstanceService
	deliverySvc      DeliveryService
	resendSvc        ResendService
	metrics          *observability.Metrics
	authTenantFn     func(context.Context, string) (tenant.Tenant, error)
	systemAPIKey     string
}

type Options struct {
	Logger             *slog.Logger
	Checkers           []NamedChecker
	RouterCheckers     []NamedChecker
	DeliveryCheckers   []NamedChecker
	Tenants            TenantService
	EventSvc           EventService
	EventQuerySvc      EventQueryService
	Apps               ConnectedAppService
	Functions          FunctionDestinationService
	Subs               SubscriptionService
	ConnectorInstances ConnectorInstanceService
	Deliveries         DeliveryService
	Resend             ResendService
	SystemAPIKey       string
	Metrics            *observability.Metrics
}

func NewHandler(opts Options) http.Handler {
	handler := &Handler{
		checkers:         opts.Checkers,
		logger:           opts.Logger,
		routerCheckers:   opts.RouterCheckers,
		deliveryCheckers: opts.DeliveryCheckers,
		tenantSvc:        opts.Tenants,
		eventSvc:         opts.EventSvc,
		eventQuerySvc:    opts.EventQuerySvc,
		appSvc:           opts.Apps,
		functionSvc:      opts.Functions,
		subSvc:           opts.Subs,
		connectorSvc:     opts.ConnectorInstances,
		deliverySvc:      opts.Deliveries,
		resendSvc:        opts.Resend,
		metrics:          opts.Metrics,
		systemAPIKey:     opts.SystemAPIKey,
	}
	if handler.logger == nil {
		handler.logger = slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	}
	if opts.Tenants != nil {
		handler.authTenantFn = opts.Tenants.Authenticate
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.healthz)
	mux.HandleFunc("GET /readyz", handler.readyz)
	mux.HandleFunc("GET /health/router", handler.routerHealth)
	mux.HandleFunc("GET /health/delivery", handler.deliveryHealth)
	mux.HandleFunc("GET /metrics", handler.metricsEndpoint)
	mux.HandleFunc("POST /webhooks/resend", handler.resendWebhook)
	mux.HandleFunc("POST /tenants", handler.createTenant)
	mux.HandleFunc("GET /tenants", handler.listTenants)
	mux.HandleFunc("GET /tenants/{tenant_id}", handler.getTenant)

	var eventsHandler http.Handler = http.HandlerFunc(handler.createEvent)
	var listEventsHandler http.Handler = http.HandlerFunc(handler.listEvents)
	var appsHandler http.Handler = http.HandlerFunc(handler.connectedApps)
	var functionsHandler http.Handler = http.HandlerFunc(handler.functions)
	var functionHandler http.Handler = http.HandlerFunc(handler.function)
	var subsHandler http.Handler = http.HandlerFunc(handler.subscriptions)
	var subStatusHandler http.Handler = http.HandlerFunc(handler.subscriptionStatus)
	var deliveriesHandler http.Handler = http.HandlerFunc(handler.deliveries)
	var deliveryHandler http.Handler = http.HandlerFunc(handler.delivery)
	var retryDeliveryHandler http.Handler = http.HandlerFunc(handler.retryDelivery)
	var resendEnableHandler http.Handler = http.HandlerFunc(handler.resendEnable)
	var resendBootstrapHandler http.Handler = http.HandlerFunc(handler.resendBootstrap)
	var connectorInstancesHandler http.Handler = http.HandlerFunc(handler.connectorInstances)
	if handler.systemAPIKey != "" {
		resendBootstrapHandler = handler.requireSystemAuth(resendBootstrapHandler)
	}
	if handler.authTenantFn != nil {
		eventsHandler = handler.requireTenantAuth(eventsHandler)
		listEventsHandler = handler.requireTenantAuth(listEventsHandler)
		appsHandler = handler.requireTenantAuth(appsHandler)
		functionsHandler = handler.requireTenantAuth(functionsHandler)
		functionHandler = handler.requireTenantAuth(functionHandler)
		subsHandler = handler.requireTenantAuth(subsHandler)
		subStatusHandler = handler.requireTenantAuth(subStatusHandler)
		deliveriesHandler = handler.requireTenantAuth(deliveriesHandler)
		deliveryHandler = handler.requireTenantAuth(deliveryHandler)
		retryDeliveryHandler = handler.requireTenantAuth(retryDeliveryHandler)
		resendEnableHandler = handler.requireTenantAuth(resendEnableHandler)
		connectorInstancesHandler = handler.requireTenantAuth(connectorInstancesHandler)
	}
	mux.Handle("POST /events", eventsHandler)
	mux.Handle("GET /events", listEventsHandler)
	mux.Handle("POST /connected-apps", appsHandler)
	mux.Handle("GET /connected-apps", appsHandler)
	mux.Handle("POST /functions", functionsHandler)
	mux.Handle("GET /functions", functionsHandler)
	mux.Handle("GET /functions/{function_id}", functionHandler)
	mux.Handle("DELETE /functions/{function_id}", functionHandler)
	mux.Handle("POST /subscriptions", subsHandler)
	mux.Handle("GET /subscriptions", subsHandler)
	mux.Handle("POST /subscriptions/{subscription_id}/pause", subStatusHandler)
	mux.Handle("POST /subscriptions/{subscription_id}/resume", subStatusHandler)
	mux.Handle("GET /deliveries", deliveriesHandler)
	mux.Handle("GET /deliveries/{delivery_id}", deliveryHandler)
	mux.Handle("POST /deliveries/{delivery_id}/retry", retryDeliveryHandler)
	mux.Handle("POST /connectors/resend/enable", resendEnableHandler)
	mux.Handle("POST /system/resend/bootstrap", resendBootstrapHandler)
	mux.Handle("POST /connector-instances", connectorInstancesHandler)
	mux.Handle("GET /connector-instances", connectorInstancesHandler)

	return mux
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	h.runChecks(w, r, h.checkers)
}

func (h *Handler) routerHealth(w http.ResponseWriter, r *http.Request) {
	h.runChecks(w, r, h.routerCheckers)
}

func (h *Handler) deliveryHealth(w http.ResponseWriter, r *http.Request) {
	h.runChecks(w, r, h.deliveryCheckers)
}

func (h *Handler) metricsEndpoint(w http.ResponseWriter, _ *http.Request) {
	if h.metrics == nil {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(h.metrics.Prometheus()))
}

func (h *Handler) runChecks(w http.ResponseWriter, r *http.Request, checkers []NamedChecker) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	failures := make(map[string]string)
	for _, checker := range checkers {
		if err := checker.Checker.Check(ctx); err != nil {
			failures[checker.Name] = err.Error()
		}
	}

	if len(failures) > 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error",
			"checks": failures,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) createTenant(w http.ResponseWriter, r *http.Request) {
	if h.tenantSvc == nil {
		writeError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}

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
		case errors.Is(err, tenant.ErrInvalidTenantName):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, tenant.ErrTenantNameExists):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error("create tenant", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to create tenant")
		}
		return
	}

	h.logger.Info("tenant_created", slog.String("tenant_id", created.Tenant.ID.String()))
	writeJSON(w, http.StatusOK, map[string]string{
		"tenant_id": created.Tenant.ID.String(),
		"api_key":   created.APIKey,
	})
}

func (h *Handler) listTenants(w http.ResponseWriter, r *http.Request) {
	if h.tenantSvc == nil {
		writeError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}

	tenants, err := h.tenantSvc.ListTenants(r.Context())
	if err != nil {
		h.logger.Error("list tenants", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}

	writeJSON(w, http.StatusOK, tenants)
}

func (h *Handler) getTenant(w http.ResponseWriter, r *http.Request) {
	if h.tenantSvc == nil {
		writeError(w, http.StatusNotImplemented, "tenant service unavailable")
		return
	}

	tenantID, err := uuid.Parse(r.PathValue("tenant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}

	record, err := h.tenantSvc.GetTenant(r.Context(), tenantID)
	if err != nil {
		switch {
		case errors.Is(err, tenant.ErrTenantNotFound):
			writeError(w, http.StatusNotFound, "tenant not found")
		default:
			h.logger.Error("get tenant", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to get tenant")
		}
		return
	}

	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	if h.eventSvc == nil {
		writeError(w, http.StatusNotImplemented, "event service unavailable")
		return
	}

	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Type    string          `json:"type"`
		Source  string          `json:"source"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.metrics != nil {
		h.metrics.IncEventsReceived()
	}

	event, err := h.eventSvc.Ingest(r.Context(), ingest.Request{
		TenantID: tenantID,
		Type:     req.Type,
		Source:   req.Source,
		Payload:  req.Payload,
	})
	if err != nil {
		switch {
		case errors.Is(err, ingest.ErrInvalidType), errors.Is(err, ingest.ErrInvalidSource):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error("event_publish_failed",
				slog.String("tenant_id", tenantID.String()),
				slog.String("error", err.Error()),
			)
			writeError(w, http.StatusInternalServerError, "failed to publish event")
		}
		return
	}

	h.logger.Info("event_received",
		slog.String("event_id", event.EventID.String()),
		slog.String("tenant_id", event.TenantID.String()),
		slog.String("event_type", event.Type),
	)
	writeJSON(w, http.StatusOK, map[string]string{"event_id": event.EventID.String()})
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.eventQuerySvc == nil {
		writeError(w, http.StatusNotImplemented, "event query service unavailable")
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
	limit, err := optionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	events, err := h.eventQuerySvc.List(r.Context(), tenantID, r.URL.Query().Get("type"), r.URL.Query().Get("source"), from, to, limit)
	if err != nil {
		h.logger.Error("list events", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}

	h.logger.Info("events_listed", slog.String("tenant_id", tenantID.String()), slog.Int("count", len(events)))
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) connectedApps(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.appSvc == nil {
		writeError(w, http.StatusNotImplemented, "connected app service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name           string `json:"name"`
			DestinationURL string `json:"destination_url"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		app, err := h.appSvc.Create(r.Context(), tenantID, req.Name, req.DestinationURL)
		if err != nil {
			switch {
			case errors.Is(err, connectedapp.ErrInvalidName), errors.Is(err, connectedapp.ErrInvalidDestinationURL):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("create connected app", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create connected app")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"id":              app.ID.String(),
			"name":            app.Name,
			"destination_url": app.DestinationURL,
		})
	case http.MethodGet:
		apps, err := h.appSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list connected apps", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list connected apps")
			return
		}
		writeJSON(w, http.StatusOK, apps)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) subscriptions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.subSvc == nil {
		writeError(w, http.StatusNotImplemented, "subscription service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectedAppID        string          `json:"connected_app_id"`
			DestinationType       string          `json:"destination_type"`
			FunctionDestinationID string          `json:"function_destination_id"`
			ConnectorInstanceID   string          `json:"connector_instance_id"`
			Operation             string          `json:"operation"`
			OperationParams       json.RawMessage `json:"operation_params"`
			EventType             string          `json:"event_type"`
			EventSource           *string         `json:"event_source"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var appID *uuid.UUID
		if strings.TrimSpace(req.ConnectedAppID) != "" {
			parsed, err := uuid.Parse(req.ConnectedAppID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid connected_app_id")
				return
			}
			appID = &parsed
		}
		var functionID *uuid.UUID
		if strings.TrimSpace(req.FunctionDestinationID) != "" {
			parsed, err := uuid.Parse(req.FunctionDestinationID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid function_destination_id")
				return
			}
			functionID = &parsed
		}
		var connectorInstanceID *uuid.UUID
		if strings.TrimSpace(req.ConnectorInstanceID) != "" {
			parsed, err := uuid.Parse(req.ConnectorInstanceID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid connector_instance_id")
				return
			}
			connectorInstanceID = &parsed
		}
		var operation *string
		if strings.TrimSpace(req.Operation) != "" {
			trimmed := strings.TrimSpace(req.Operation)
			operation = &trimmed
		}
		sub, err := h.subSvc.Create(r.Context(), tenantID, req.DestinationType, appID, functionID, connectorInstanceID, operation, req.OperationParams, req.EventType, req.EventSource)
		if err != nil {
			switch {
			case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, subscription.ErrConnectedAppNotFound):
				writeError(w, http.StatusNotFound, "connected app not found")
			case errors.Is(err, subscription.ErrFunctionDestinationNotFound):
				writeError(w, http.StatusNotFound, "function destination not found")
			case errors.Is(err, subscription.ErrConnectorInstanceNotFound):
				writeError(w, http.StatusNotFound, "connector instance not found")
			default:
				h.logger.Error("create subscription", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create subscription")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": sub.ID.String()})
	case http.MethodGet:
		subs, err := h.subSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list subscriptions", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list subscriptions")
			return
		}
		writeJSON(w, http.StatusOK, subs)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) connectorInstances(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.connectorSvc == nil {
		writeError(w, http.StatusNotImplemented, "connector instance service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectorName string          `json:"connector_name"`
			Config        json.RawMessage `json:"config"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		instance, err := h.connectorSvc.Create(r.Context(), tenantID, req.ConnectorName, req.Config)
		if err != nil {
			switch {
			case errors.Is(err, connectorinstance.ErrUnsupportedConnector), errors.Is(err, connectorinstance.ErrInvalidConfig), errors.Is(err, connectorinstance.ErrMissingBotToken):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, connectorinstance.ErrDuplicateInstance):
				writeError(w, http.StatusConflict, err.Error())
			default:
				h.logger.Error("create connector instance", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create connector instance")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": instance.ID.String(), "connector_name": instance.ConnectorName})
	case http.MethodGet:
		instances, err := h.connectorSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list connector instances", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list connector instances")
			return
		}
		writeJSON(w, http.StatusOK, instances)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) functions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.functionSvc == nil {
		writeError(w, http.StatusNotImplemented, "function destination service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.functionSvc.Create(r.Context(), tenantID, req.Name, req.URL)
		if err != nil {
			switch {
			case errors.Is(err, functiondestination.ErrInvalidName), errors.Is(err, functiondestination.ErrInvalidURL):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("create function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create function destination")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     created.Destination.ID.String(),
			"name":   created.Destination.Name,
			"url":    created.Destination.URL,
			"secret": created.Secret,
		})
	case http.MethodGet:
		destinations, err := h.functionSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list function destinations", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list function destinations")
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
		writeJSON(w, http.StatusOK, response)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) function(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.functionSvc == nil {
		writeError(w, http.StatusNotImplemented, "function destination service unavailable")
		return
	}

	functionID, err := uuid.Parse(r.PathValue("function_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid function_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		destination, err := h.functionSvc.Get(r.Context(), tenantID, functionID)
		if err != nil {
			switch {
			case errors.Is(err, functiondestination.ErrNotFound):
				writeError(w, http.StatusNotFound, "function destination not found")
			default:
				h.logger.Error("get function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to get function destination")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"id":   destination.ID.String(),
			"name": destination.Name,
			"url":  destination.URL,
		})
	case http.MethodDelete:
		err := h.functionSvc.Delete(r.Context(), tenantID, functionID)
		if err != nil {
			switch {
			case errors.Is(err, functiondestination.ErrNotFound):
				writeError(w, http.StatusNotFound, "function destination not found")
			case errors.Is(err, functiondestination.ErrInUse):
				writeError(w, http.StatusBadRequest, "function destination has active subscriptions")
			default:
				h.logger.Error("delete function destination", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to delete function destination")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) subscriptionStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.subSvc == nil {
		writeError(w, http.StatusNotImplemented, "subscription service unavailable")
		return
	}

	subscriptionID, err := uuid.Parse(r.PathValue("subscription_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}

	var sub subscription.Subscription
	switch {
	case strings.HasSuffix(r.URL.Path, "/pause"):
		sub, err = h.subSvc.Pause(r.Context(), tenantID, subscriptionID)
		if err == nil {
			h.logger.Info("subscription_paused", slog.String("tenant_id", tenantID.String()), slog.String("subscription_id", subscriptionID.String()))
		}
	case strings.HasSuffix(r.URL.Path, "/resume"):
		sub, err = h.subSvc.Resume(r.Context(), tenantID, subscriptionID)
		if err == nil {
			h.logger.Info("subscription_resumed", slog.String("tenant_id", tenantID.String()), slog.String("subscription_id", subscriptionID.String()))
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrSubscriptionNotFound):
			writeError(w, http.StatusNotFound, "subscription not found")
		default:
			h.logger.Error("update subscription status", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to update subscription")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": sub.Status})
}

func (h *Handler) deliveries(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.deliverySvc == nil {
		writeError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}

	limit, err := optionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	subscriptionID, err := optionalUUID(r.URL.Query().Get("subscription_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription_id")
		return
	}
	eventID, err := optionalUUID(r.URL.Query().Get("event_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event_id")
		return
	}

	jobs, err := h.deliverySvc.List(r.Context(), tenantID, r.URL.Query().Get("status"), subscriptionID, eventID, limit)
	if err != nil {
		h.logger.Error("list deliveries", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list deliveries")
		return
	}

	h.logger.Info("deliveries_listed", slog.String("tenant_id", tenantID.String()), slog.Int("count", len(jobs)))
	writeJSON(w, http.StatusOK, mapJobs(jobs))
}

func (h *Handler) delivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.deliverySvc == nil {
		writeError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}

	deliveryID, err := uuid.Parse(r.PathValue("delivery_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid delivery_id")
		return
	}
	job, err := h.deliverySvc.Get(r.Context(), tenantID, deliveryID)
	if err != nil {
		switch {
		case errors.Is(err, delivery.ErrJobNotFound):
			writeError(w, http.StatusNotFound, "delivery not found")
		default:
			h.logger.Error("get delivery", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to get delivery")
		}
		return
	}

	writeJSON(w, http.StatusOK, mapJob(job))
}

func (h *Handler) retryDelivery(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.deliverySvc == nil {
		writeError(w, http.StatusNotImplemented, "delivery service unavailable")
		return
	}

	deliveryID, err := uuid.Parse(r.PathValue("delivery_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid delivery_id")
		return
	}
	job, err := h.deliverySvc.Retry(r.Context(), tenantID, deliveryID)
	if err != nil {
		switch {
		case errors.Is(err, delivery.ErrJobNotFound):
			writeError(w, http.StatusNotFound, "delivery not found")
		case errors.Is(err, delivery.ErrRetryNotAllowed):
			writeError(w, http.StatusBadRequest, "delivery retry not allowed")
		default:
			h.logger.Error("retry delivery", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to retry delivery")
		}
		return
	}

	h.logger.Info("delivery_retried", slog.String("tenant_id", tenantID.String()), slog.String("delivery_id", deliveryID.String()))
	writeJSON(w, http.StatusOK, map[string]string{"status": job.Status})
}

func (h *Handler) resendBootstrap(w http.ResponseWriter, r *http.Request) {
	if h.resendSvc == nil {
		writeError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	status, err := h.resendSvc.Bootstrap(r.Context())
	if err != nil {
		h.logger.Error("resend bootstrap", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to bootstrap resend")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (h *Handler) resendEnable(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.resendSvc == nil {
		writeError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	result, err := h.resendSvc.Enable(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("enable resend connector", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to enable resend connector")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"address": result.Address})
}

func (h *Handler) resendWebhook(w http.ResponseWriter, r *http.Request) {
	if h.resendSvc == nil {
		writeError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	if err := h.resendSvc.HandleWebhook(r.Context(), rawBody, r.Header.Clone()); err != nil {
		h.logger.Error("handle resend webhook", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid JSON request body")
	}

	if decoder.More() {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(body.Bytes()); err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
	}
}

func optionalTime(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func optionalLimit(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func optionalUUID(value string) (*uuid.UUID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func mapJobs(jobs []delivery.Job) []map[string]any {
	result := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, mapJob(job))
	}
	return result
}

func mapJob(job delivery.Job) map[string]any {
	return map[string]any{
		"id":               job.ID.String(),
		"subscription_id":  job.SubscriptionID.String(),
		"event_id":         job.EventID.String(),
		"status":           job.Status,
		"attempts":         job.Attempts,
		"last_error":       job.LastError,
		"external_id":      job.ExternalID,
		"last_status_code": job.LastStatusCode,
		"created_at":       job.CreatedAt,
		"completed_at":     job.CompletedAt,
	}
}
