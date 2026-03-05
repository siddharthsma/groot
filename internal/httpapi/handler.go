package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/ingest"
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

type ConnectedAppService interface {
	Create(context.Context, tenant.ID, string, string) (connectedapp.App, error)
	List(context.Context, tenant.ID) ([]connectedapp.App, error)
	Get(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type SubscriptionService interface {
	Create(context.Context, tenant.ID, uuid.UUID, string, *string) (subscription.Subscription, error)
	List(context.Context, tenant.ID) ([]subscription.Subscription, error)
}

type Handler struct {
	logger       *slog.Logger
	checkers     []NamedChecker
	tenantSvc    TenantService
	eventSvc     EventService
	appSvc       ConnectedAppService
	subSvc       SubscriptionService
	authTenantFn func(context.Context, string) (tenant.Tenant, error)
}

type Options struct {
	Logger   *slog.Logger
	Checkers []NamedChecker
	Tenants  TenantService
	EventSvc EventService
	Apps     ConnectedAppService
	Subs     SubscriptionService
}

func NewHandler(opts Options) http.Handler {
	handler := &Handler{
		logger:    opts.Logger,
		checkers:  opts.Checkers,
		tenantSvc: opts.Tenants,
		eventSvc:  opts.EventSvc,
		appSvc:    opts.Apps,
		subSvc:    opts.Subs,
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
	mux.HandleFunc("POST /tenants", handler.createTenant)
	mux.HandleFunc("GET /tenants", handler.listTenants)
	mux.HandleFunc("GET /tenants/{tenant_id}", handler.getTenant)

	var eventsHandler http.Handler = http.HandlerFunc(handler.createEvent)
	var appsHandler http.Handler = http.HandlerFunc(handler.connectedApps)
	var subsHandler http.Handler = http.HandlerFunc(handler.subscriptions)
	if handler.authTenantFn != nil {
		eventsHandler = handler.requireTenantAuth(eventsHandler)
		appsHandler = handler.requireTenantAuth(appsHandler)
		subsHandler = handler.requireTenantAuth(subsHandler)
	}
	mux.Handle("POST /events", eventsHandler)
	mux.Handle("POST /connected-apps", appsHandler)
	mux.Handle("GET /connected-apps", appsHandler)
	mux.Handle("POST /subscriptions", subsHandler)
	mux.Handle("GET /subscriptions", subsHandler)

	return mux
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	failures := make(map[string]string)
	for _, checker := range h.checkers {
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
			ConnectedAppID string  `json:"connected_app_id"`
			EventType      string  `json:"event_type"`
			EventSource    *string `json:"event_source"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		appID, err := uuid.Parse(req.ConnectedAppID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connected_app_id")
			return
		}
		sub, err := h.subSvc.Create(r.Context(), tenantID, appID, req.EventType, req.EventSource)
		if err != nil {
			switch {
			case errors.Is(err, subscription.ErrInvalidEventType):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, subscription.ErrConnectedAppNotFound):
				writeError(w, http.StatusNotFound, "connected app not found")
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
