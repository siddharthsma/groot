package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"groot/internal/agent"

	"groot/internal/apikey"
	iauth "groot/internal/auth"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	slackconnector "groot/internal/connectors/inbound/slack"
	stripeconnector "groot/internal/connectors/inbound/stripe"
	"groot/internal/connectors/resend"
	"groot/internal/delivery"
	"groot/internal/edition"
	"groot/internal/eventquery"
	"groot/internal/functiondestination"
	"groot/internal/graph"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/replay"
	"groot/internal/schemas"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
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
	UpdateTenantName(context.Context, uuid.UUID, string) (tenant.Tenant, error)
	Authenticate(context.Context, string) (tenant.Tenant, error)
}

type EventService interface {
	Ingest(context.Context, ingest.Request) (stream.Event, error)
}

type EventQueryService interface {
	List(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]eventquery.Event, error)
	AdminList(context.Context, tenant.ID, string, *time.Time, *time.Time, int, bool) ([]eventquery.AdminEvent, error)
	AdminGet(context.Context, uuid.UUID, bool) (eventquery.AdminEvent, error)
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
	Create(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error)
	Update(context.Context, tenant.ID, uuid.UUID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error)
	List(context.Context, tenant.ID) ([]subscription.Subscription, error)
	AdminList(context.Context, *tenant.ID, string, string) ([]subscription.Subscription, error)
	Pause(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
	Resume(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
}

type AgentService interface {
	Create(context.Context, tenant.ID, agent.CreateRequest) (agent.Definition, error)
	Update(context.Context, tenant.ID, uuid.UUID, agent.CreateRequest) (agent.Definition, error)
	Get(context.Context, tenant.ID, uuid.UUID) (agent.Definition, error)
	List(context.Context, tenant.ID) ([]agent.Definition, error)
	Delete(context.Context, tenant.ID, uuid.UUID) error
	ListSessions(context.Context, tenant.ID, *uuid.UUID, string, int) ([]agent.Session, error)
	GetSession(context.Context, tenant.ID, uuid.UUID) (agent.Session, error)
	CloseSession(context.Context, tenant.ID, uuid.UUID) (agent.Session, error)
}

type AgentToolService interface {
	ExecuteTool(context.Context, agent.ToolExecutionRequest) (agent.ToolExecutionResult, error)
}

type ConnectorInstanceService interface {
	Create(context.Context, *tenant.ID, string, string, json.RawMessage) (connectorinstance.Instance, error)
	List(context.Context, tenant.ID) ([]connectorinstance.Instance, error)
	ListAll(context.Context) ([]connectorinstance.Instance, error)
	AdminList(context.Context, *tenant.ID, string, string) ([]connectorinstance.Instance, error)
	AdminUpsert(context.Context, uuid.UUID, *tenant.ID, string, string, json.RawMessage) (connectorinstance.Instance, error)
}

type InboundRouteService interface {
	Create(context.Context, tenant.ID, string, string, *uuid.UUID) (inboundroute.Route, error)
	List(context.Context, tenant.ID) ([]inboundroute.Route, error)
	ListAll(context.Context) ([]inboundroute.Route, error)
}

type DeliveryService interface {
	List(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]delivery.Job, error)
	AdminList(context.Context, tenant.ID, string, *time.Time, *time.Time, int) ([]delivery.Job, error)
	Get(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
	Retry(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
}

type ReplayService interface {
	ReplayEvent(context.Context, tenant.ID, uuid.UUID) (replay.SingleResult, error)
	ReplayQuery(context.Context, tenant.ID, replay.QueryRequest) (replay.QueryResult, error)
}

type ResendService interface {
	Bootstrap(context.Context) (string, error)
	Enable(context.Context, tenant.ID) (resend.EnableResult, error)
	HandleWebhook(context.Context, []byte, http.Header) error
}

type StripeService interface {
	Enable(context.Context, tenant.ID, string, string) (uuid.UUID, error)
	HandleWebhook(context.Context, []byte, http.Header) error
}

type SchemaService interface {
	List(context.Context) ([]schemas.Schema, error)
	Get(context.Context, string) (schemas.Schema, error)
}

type SlackService interface {
	HandleEvents(context.Context, []byte, http.Header) (slackconnector.Result, error)
}

type Authenticator interface {
	AuthenticateRequest(*http.Request) (iauth.Principal, error)
}

type APIKeyService interface {
	Create(context.Context, tenant.ID, string) (apikey.CreatedAPIKey, error)
	List(context.Context, tenant.ID) ([]apikey.APIKey, error)
	Revoke(context.Context, tenant.ID, uuid.UUID) (apikey.APIKey, error)
}

type GraphService interface {
	BuildTopology(context.Context, graph.TopologyRequest) (graph.Topology, error)
	BuildExecution(context.Context, uuid.UUID, graph.ExecutionRequest) (graph.ExecutionGraph, error)
}

type AuditService interface {
	Audit(context.Context, string, string, *uuid.UUID, map[string]any) error
	AuditForTenant(context.Context, tenant.ID, string, string, *uuid.UUID, map[string]any) error
}

type Handler struct {
	logger                   *slog.Logger
	checkers                 []NamedChecker
	routerCheckers           []NamedChecker
	deliveryCheckers         []NamedChecker
	tenantSvc                TenantService
	eventSvc                 EventService
	eventQuerySvc            EventQueryService
	appSvc                   ConnectedAppService
	functionSvc              FunctionDestinationService
	subSvc                   SubscriptionService
	connectorSvc             ConnectorInstanceService
	inboundRouteSvc          InboundRouteService
	deliverySvc              DeliveryService
	replaySvc                ReplayService
	adminReplaySvc           ReplayService
	schemaSvc                SchemaService
	resendSvc                ResendService
	slackSvc                 SlackService
	stripeSvc                StripeService
	metrics                  *observability.Metrics
	authTenantFn             func(context.Context, string) (tenant.Tenant, error)
	authSvc                  Authenticator
	adminAuthSvc             Authenticator
	adminLimiter             *rate.Limiter
	apiKeySvc                APIKeyService
	graphSvc                 GraphService
	auditSvc                 AuditService
	agentSvc                 AgentService
	agentToolSvc             AgentToolService
	systemAPIKey             string
	agentRuntimeSharedSecret string
	editionRuntime           edition.Runtime
	communityBootstrapTenant *uuid.UUID
	adminEnabled             bool
	adminAllowViewPayloads   bool
	adminReplayEnabled       bool
	adminReplayMaxEvents     int
}

type Options struct {
	Logger                   *slog.Logger
	Checkers                 []NamedChecker
	RouterCheckers           []NamedChecker
	DeliveryCheckers         []NamedChecker
	Tenants                  TenantService
	EventSvc                 EventService
	EventQuerySvc            EventQueryService
	Apps                     ConnectedAppService
	Functions                FunctionDestinationService
	Subs                     SubscriptionService
	ConnectorInstances       ConnectorInstanceService
	InboundRoutes            InboundRouteService
	Deliveries               DeliveryService
	Replay                   ReplayService
	AdminReplay              ReplayService
	Schemas                  SchemaService
	Resend                   ResendService
	Slack                    SlackService
	Stripe                   StripeService
	Auth                     Authenticator
	AdminAuth                Authenticator
	AdminEnabled             bool
	AdminAllowViewPayloads   bool
	AdminReplayEnabled       bool
	AdminRateLimitRPS        int
	AdminReplayMaxEvents     int
	APIKeys                  APIKeyService
	Graph                    GraphService
	Audit                    AuditService
	Agents                   AgentService
	AgentTools               AgentToolService
	SystemAPIKey             string
	AgentRuntimeSharedSecret string
	Edition                  edition.Runtime
	CommunityBootstrapTenant *uuid.UUID
	Metrics                  *observability.Metrics
}

func NewHandler(opts Options) http.Handler {
	handler := &Handler{
		checkers:                 opts.Checkers,
		logger:                   opts.Logger,
		routerCheckers:           opts.RouterCheckers,
		deliveryCheckers:         opts.DeliveryCheckers,
		tenantSvc:                opts.Tenants,
		eventSvc:                 opts.EventSvc,
		eventQuerySvc:            opts.EventQuerySvc,
		appSvc:                   opts.Apps,
		functionSvc:              opts.Functions,
		subSvc:                   opts.Subs,
		connectorSvc:             opts.ConnectorInstances,
		inboundRouteSvc:          opts.InboundRoutes,
		deliverySvc:              opts.Deliveries,
		replaySvc:                opts.Replay,
		adminReplaySvc:           opts.AdminReplay,
		schemaSvc:                opts.Schemas,
		resendSvc:                opts.Resend,
		slackSvc:                 opts.Slack,
		stripeSvc:                opts.Stripe,
		authSvc:                  opts.Auth,
		adminAuthSvc:             opts.AdminAuth,
		adminLimiter:             newAdminLimiter(opts.AdminRateLimitRPS),
		apiKeySvc:                opts.APIKeys,
		graphSvc:                 opts.Graph,
		auditSvc:                 opts.Audit,
		agentSvc:                 opts.Agents,
		agentToolSvc:             opts.AgentTools,
		metrics:                  opts.Metrics,
		systemAPIKey:             opts.SystemAPIKey,
		agentRuntimeSharedSecret: strings.TrimSpace(opts.AgentRuntimeSharedSecret),
		editionRuntime:           opts.Edition,
		communityBootstrapTenant: opts.CommunityBootstrapTenant,
		adminEnabled:             opts.AdminEnabled,
		adminAllowViewPayloads:   opts.AdminAllowViewPayloads,
		adminReplayEnabled:       opts.AdminReplayEnabled,
		adminReplayMaxEvents:     opts.AdminReplayMaxEvents,
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
	mux.HandleFunc("GET /system/edition", handler.systemEdition)
	mux.HandleFunc("GET /schemas", handler.listSchemas)
	mux.HandleFunc("GET /schemas/{full_name}", handler.getSchema)
	mux.HandleFunc("POST /webhooks/resend", handler.resendWebhook)
	mux.HandleFunc("POST /webhooks/slack/events", handler.slackWebhook)
	mux.HandleFunc("POST /webhooks/stripe", handler.stripeWebhook)
	mux.Handle("POST /tenants", handler.communityEditionRestriction(http.HandlerFunc(handler.createTenant)))
	mux.Handle("GET /tenants", handler.communityEditionRestriction(http.HandlerFunc(handler.listTenants)))
	mux.Handle("GET /tenants/{tenant_id}", handler.communityEditionRestriction(http.HandlerFunc(handler.getTenant)))

	var eventsHandler http.Handler = http.HandlerFunc(handler.createEvent)
	var listEventsHandler http.Handler = http.HandlerFunc(handler.listEvents)
	var appsHandler http.Handler = http.HandlerFunc(handler.connectedApps)
	var functionsHandler http.Handler = http.HandlerFunc(handler.functions)
	var functionHandler http.Handler = http.HandlerFunc(handler.function)
	var subsHandler http.Handler = http.HandlerFunc(handler.subscriptions)
	var subReplaceHandler http.Handler = http.HandlerFunc(handler.replaceSubscription)
	var subStatusHandler http.Handler = http.HandlerFunc(handler.subscriptionStatus)
	var deliveriesHandler http.Handler = http.HandlerFunc(handler.deliveries)
	var deliveryHandler http.Handler = http.HandlerFunc(handler.delivery)
	var retryDeliveryHandler http.Handler = http.HandlerFunc(handler.retryDelivery)
	var replayEventHandler http.Handler = http.HandlerFunc(handler.replayEvent)
	var replayEventsHandler http.Handler = http.HandlerFunc(handler.replayEvents)
	var resendEnableHandler http.Handler = http.HandlerFunc(handler.resendEnable)
	var stripeEnableHandler http.Handler = http.HandlerFunc(handler.stripeEnable)
	var resendBootstrapHandler http.Handler = http.HandlerFunc(handler.resendBootstrap)
	var apiKeysHandler http.Handler = http.HandlerFunc(handler.apiKeys)
	var revokeAPIKeyHandler http.Handler = http.HandlerFunc(handler.revokeAPIKey)
	var connectorInstancesGetHandler http.Handler = http.HandlerFunc(handler.connectorInstances)
	var connectorInstancesPostHandler http.Handler = http.HandlerFunc(handler.connectorInstances)
	var inboundRoutesHandler http.Handler = http.HandlerFunc(handler.inboundRoutes)
	var systemInboundRoutesHandler http.Handler = http.HandlerFunc(handler.systemInboundRoutes)
	var agentsHandler http.Handler = http.HandlerFunc(handler.agents)
	var agentHandler http.Handler = http.HandlerFunc(handler.agent)
	var agentSessionsHandler http.Handler = http.HandlerFunc(handler.agentSessions)
	var agentSessionHandler http.Handler = http.HandlerFunc(handler.agentSession)
	var runtimeToolHandler http.Handler = http.HandlerFunc(handler.agentRuntimeToolCalls)
	if handler.systemAPIKey != "" {
		resendBootstrapHandler = handler.requireSystemAuth(resendBootstrapHandler)
		systemInboundRoutesHandler = handler.requireSystemAuth(systemInboundRoutesHandler)
	}
	if handler.authSvc != nil || handler.authTenantFn != nil {
		eventsHandler = handler.requireTenantAuth(eventsHandler)
		listEventsHandler = handler.requireTenantAuth(listEventsHandler)
		appsHandler = handler.requireTenantAuth(appsHandler)
		functionsHandler = handler.requireTenantAuth(functionsHandler)
		functionHandler = handler.requireTenantAuth(functionHandler)
		subsHandler = handler.requireTenantAuth(subsHandler)
		subReplaceHandler = handler.requireTenantAuth(subReplaceHandler)
		subStatusHandler = handler.requireTenantAuth(subStatusHandler)
		deliveriesHandler = handler.requireTenantAuth(deliveriesHandler)
		deliveryHandler = handler.requireTenantAuth(deliveryHandler)
		retryDeliveryHandler = handler.requireTenantAuth(retryDeliveryHandler)
		replayEventHandler = handler.requireTenantAuth(replayEventHandler)
		replayEventsHandler = handler.requireTenantAuth(replayEventsHandler)
		apiKeysHandler = handler.requireTenantAuth(apiKeysHandler)
		revokeAPIKeyHandler = handler.requireTenantAuth(revokeAPIKeyHandler)
		resendEnableHandler = handler.requireTenantAuth(resendEnableHandler)
		stripeEnableHandler = handler.requireTenantAuth(stripeEnableHandler)
		connectorInstancesGetHandler = handler.requireTenantAuth(connectorInstancesGetHandler)
		inboundRoutesHandler = handler.requireTenantAuth(inboundRoutesHandler)
		agentsHandler = handler.requireTenantAuth(agentsHandler)
		agentHandler = handler.requireTenantAuth(agentHandler)
		agentSessionsHandler = handler.requireTenantAuth(agentSessionsHandler)
		agentSessionHandler = handler.requireTenantAuth(agentSessionHandler)
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
	mux.Handle("PUT /subscriptions/{subscription_id}", subReplaceHandler)
	mux.Handle("POST /subscriptions/{subscription_id}/pause", subStatusHandler)
	mux.Handle("POST /subscriptions/{subscription_id}/resume", subStatusHandler)
	mux.Handle("GET /deliveries", deliveriesHandler)
	mux.Handle("GET /deliveries/{delivery_id}", deliveryHandler)
	mux.Handle("POST /deliveries/{delivery_id}/retry", retryDeliveryHandler)
	mux.Handle("POST /events/{event_id}/replay", replayEventHandler)
	mux.Handle("POST /events/replay", replayEventsHandler)
	mux.Handle("POST /connectors/resend/enable", resendEnableHandler)
	mux.Handle("POST /connectors/stripe/enable", stripeEnableHandler)
	mux.Handle("POST /system/resend/bootstrap", resendBootstrapHandler)
	mux.Handle("POST /api-keys", apiKeysHandler)
	mux.Handle("GET /api-keys", apiKeysHandler)
	mux.Handle("POST /api-keys/{api_key_id}/revoke", revokeAPIKeyHandler)
	mux.Handle("POST /connector-instances", connectorInstancesPostHandler)
	mux.Handle("GET /connector-instances", connectorInstancesGetHandler)
	mux.Handle("POST /routes/inbound", inboundRoutesHandler)
	mux.Handle("GET /routes/inbound", inboundRoutesHandler)
	mux.Handle("GET /system/routes/inbound", systemInboundRoutesHandler)
	mux.Handle("POST /agents", agentsHandler)
	mux.Handle("GET /agents", agentsHandler)
	mux.Handle("GET /agents/{agent_id}", agentHandler)
	mux.Handle("PUT /agents/{agent_id}", agentHandler)
	mux.Handle("DELETE /agents/{agent_id}", agentHandler)
	mux.Handle("GET /agent-sessions", agentSessionsHandler)
	mux.Handle("GET /agent-sessions/{session_id}", agentSessionHandler)
	mux.Handle("POST /agent-sessions/{session_id}/close", agentSessionHandler)
	mux.Handle("POST /internal/agent-runtime/tool-calls", runtimeToolHandler)
	if handler.adminEnabled {
		mux.Handle("GET /admin/tenants", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenants))))
		mux.Handle("POST /admin/tenants", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenants))))
		mux.Handle("GET /admin/tenants/{tenant_id}", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenant))))
		mux.Handle("PATCH /admin/tenants/{tenant_id}", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenant))))
		mux.Handle("POST /admin/tenants/{tenant_id}/api-keys", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenantAPIKeys))))
		mux.Handle("GET /admin/tenants/{tenant_id}/api-keys", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenantAPIKeys))))
		mux.Handle("POST /admin/tenants/{tenant_id}/api-keys/{api_key_id}/revoke", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenantAPIKeyRevoke))))
		mux.Handle("GET /admin/connector-instances", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminConnectorInstances))))
		mux.Handle("PUT /admin/connector-instances/{id}", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminConnectorInstance))))
		mux.Handle("GET /admin/subscriptions", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminSubscriptions))))
		mux.Handle("POST /admin/tenants/{tenant_id}/subscriptions", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTenantSubscriptions))))
		mux.Handle("GET /admin/events", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminEvents))))
		mux.Handle("GET /admin/delivery-jobs", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminDeliveryJobs))))
		mux.Handle("GET /admin/topology", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminTopology))))
		mux.Handle("GET /admin/events/{event_id}/execution-graph", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminExecutionGraph))))
		mux.Handle("POST /admin/events/{event_id}/replay", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminReplayEvent))))
		mux.Handle("POST /admin/events/replay", handler.requireAdmin(handler.communityEditionRestriction(http.HandlerFunc(handler.adminReplayEvents))))
	}

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

func (h *Handler) systemEdition(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"build_edition":     h.editionRuntime.BuildEdition,
		"effective_edition": h.editionRuntime.EffectiveEdition,
		"tenancy_mode":      h.editionRuntime.TenancyMode,
		"license":           h.editionRuntime.License,
		"capabilities":      h.editionRuntime.Capabilities,
	})
}

func (h *Handler) communityEditionRestriction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.editionRuntime.IsCommunity() {
			writeError(w, http.StatusForbidden, "community_edition_restriction")
			return
		}
		next.ServeHTTP(w, r)
	})
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
	if !h.editionRuntime.Capabilities.TenantCreationAllowed {
		writeError(w, http.StatusForbidden, "community_edition_restriction")
		return
	}
	if h.editionRuntime.MaxTenants > 0 {
		tenants, err := h.tenantSvc.ListTenants(r.Context())
		if err != nil {
			h.logger.Error("list tenants for tenant creation limit", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to validate tenant limit")
			return
		}
		if len(tenants) >= h.editionRuntime.MaxTenants {
			writeError(w, http.StatusForbidden, "tenant_limit_exceeded")
			return
		}
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

func (h *Handler) apiKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.apiKeySvc == nil {
		writeError(w, http.StatusNotImplemented, "api key service unavailable")
		return
	}

	switch r.Method {
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
			switch {
			case errors.Is(err, apikey.ErrInvalidName):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("create api key", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create api key")
			}
			return
		}
		h.audit("api_key.create", "api_key", &created.ID, map[string]any{
			"name":       created.Name,
			"key_prefix": created.KeyPrefix,
		}, r.Context())
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         created.ID.String(),
			"name":       created.Name,
			"api_key":    created.Secret,
			"key_prefix": created.KeyPrefix,
		})
	case http.MethodGet:
		keys, err := h.apiKeySvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list api keys", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list api keys")
			return
		}
		writeJSON(w, http.StatusOK, keys)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.apiKeySvc == nil {
		writeError(w, http.StatusNotImplemented, "api key service unavailable")
		return
	}
	id, err := uuid.Parse(r.PathValue("api_key_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid api_key_id")
		return
	}
	key, err := h.apiKeySvc.Revoke(r.Context(), tenantID, id)
	if err != nil {
		switch {
		case errors.Is(err, apikey.ErrNotFound):
			writeError(w, http.StatusNotFound, "api key not found")
		default:
			h.logger.Error("revoke api key", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to revoke api key")
		}
		return
	}
	h.audit("api_key.revoke", "api_key", &key.ID, map[string]any{
		"key_prefix": key.KeyPrefix,
	}, r.Context())
	writeJSON(w, http.StatusOK, key)
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
		TenantID:   tenantID,
		Type:       req.Type,
		Source:     req.Source,
		SourceKind: stream.SourceKindExternal,
		ChainDepth: 0,
		Payload:    req.Payload,
	})
	if err != nil {
		switch {
		case errors.Is(err, ingest.ErrInvalidType), errors.Is(err, ingest.ErrInvalidSource), errors.Is(err, ingest.ErrInvalidSourceKind), errors.Is(err, ingest.ErrInvalidChainDepth):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ingest.ErrInvalidVersionedType):
			writeError(w, http.StatusBadRequest, err.Error())
		case isSchemaReject(err):
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

func (h *Handler) listSchemas(w http.ResponseWriter, r *http.Request) {
	if h.schemaSvc == nil {
		writeError(w, http.StatusNotImplemented, "schema service unavailable")
		return
	}
	records, err := h.schemaSvc.List(r.Context())
	if err != nil {
		h.logger.Error("list schemas", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list schemas")
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
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) getSchema(w http.ResponseWriter, r *http.Request) {
	if h.schemaSvc == nil {
		writeError(w, http.StatusNotImplemented, "schema service unavailable")
		return
	}
	record, err := h.schemaSvc.Get(r.Context(), r.PathValue("full_name"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "schema not found")
			return
		}
		h.logger.Error("get schema", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to get schema")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"full_name": record.FullName,
		"schema":    json.RawMessage(record.SchemaJSON),
	})
}

func isSchemaReject(err error) bool {
	var rejectErr schemas.RejectError
	return errors.As(err, &rejectErr)
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

func (h *Handler) agents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.agentSvc == nil {
		writeError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req agent.CreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		created, err := h.agentSvc.Create(r.Context(), tenantID, req)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrInvalidName), errors.Is(err, agent.ErrInvalidInstructions), errors.Is(err, agent.ErrInvalidAllowedTools):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, agent.ErrFunctionDestinationMissing):
				writeError(w, http.StatusNotFound, "function destination not found")
			case errors.Is(err, agent.ErrDuplicateName):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("create agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create agent")
			}
			return
		}
		h.audit("agent.create", "agent", &created.ID, map[string]any{"name": created.Name}, r.Context())
		writeJSON(w, http.StatusCreated, created)
	case http.MethodGet:
		records, err := h.agentSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list agents", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list agents")
			return
		}
		writeJSON(w, http.StatusOK, records)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) agent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.agentSvc == nil {
		writeError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}
	agentID, err := uuid.Parse(r.PathValue("agent_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := h.agentSvc.Get(r.Context(), tenantID, agentID)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrNotFound):
				writeError(w, http.StatusNotFound, "agent not found")
			default:
				h.logger.Error("get agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to get agent")
			}
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPut:
		var req agent.CreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := h.agentSvc.Update(r.Context(), tenantID, agentID, req)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrNotFound):
				writeError(w, http.StatusNotFound, "agent not found")
			case errors.Is(err, agent.ErrInvalidName), errors.Is(err, agent.ErrInvalidInstructions), errors.Is(err, agent.ErrInvalidAllowedTools), errors.Is(err, agent.ErrDuplicateName):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, agent.ErrFunctionDestinationMissing):
				writeError(w, http.StatusNotFound, "function destination not found")
			default:
				h.logger.Error("update agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to update agent")
			}
			return
		}
		h.audit("agent.update", "agent", &updated.ID, map[string]any{"name": updated.Name}, r.Context())
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.agentSvc.Delete(r.Context(), tenantID, agentID); err != nil {
			switch {
			case errors.Is(err, agent.ErrNotFound):
				writeError(w, http.StatusNotFound, "agent not found")
			case errors.Is(err, agent.ErrSubscriptionReferences), errors.Is(err, agent.ErrActiveSessionsExist):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("delete agent", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to delete agent")
			}
			return
		}
		h.audit("agent.delete", "agent", &agentID, nil, r.Context())
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) agentSessions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.agentSvc == nil {
		writeError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}

	var agentID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("agent_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		agentID = &parsed
	}
	limit, err := optionalLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	records, err := h.agentSvc.ListSessions(r.Context(), tenantID, agentID, r.URL.Query().Get("status"), limit)
	if err != nil {
		h.logger.Error("list agent sessions", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list agent sessions")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (h *Handler) agentSession(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.agentSvc == nil {
		writeError(w, http.StatusNotImplemented, "agent service unavailable")
		return
	}
	sessionID, err := uuid.Parse(r.PathValue("session_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid session_id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := h.agentSvc.GetSession(r.Context(), tenantID, sessionID)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrSessionNotFound):
				writeError(w, http.StatusNotFound, "agent session not found")
			default:
				h.logger.Error("get agent session", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to get agent session")
			}
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPost:
		record, err := h.agentSvc.CloseSession(r.Context(), tenantID, sessionID)
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrSessionNotFound):
				writeError(w, http.StatusNotFound, "agent session not found")
			default:
				h.logger.Error("close agent session", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to close agent session")
			}
			return
		}
		h.audit("agent_session.close", "agent_session", &record.ID, map[string]any{"agent_id": record.AgentID.String()}, r.Context())
		writeJSON(w, http.StatusOK, record)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) agentRuntimeToolCalls(w http.ResponseWriter, r *http.Request) {
	if h.agentToolSvc == nil {
		writeError(w, http.StatusNotImplemented, "agent runtime tool service unavailable")
		return
	}
	expected := strings.TrimSpace(h.agentRuntimeSharedSecret)
	if expected == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if bearerToken(r.Header.Get("Authorization")) != expected {
		writeError(w, http.StatusUnauthorized, "unauthorized")
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_id")
		return
	}
	sessionID, err := uuid.Parse(strings.TrimSpace(req.AgentSessionID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_session_id")
		return
	}
	runID, err := uuid.Parse(strings.TrimSpace(req.AgentRunID))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid agent_run_id")
		return
	}
	result, err := h.agentToolSvc.ExecuteTool(r.Context(), agent.ToolExecutionRequest{
		TenantID:       tenantID,
		AgentID:        agentID,
		AgentSessionID: sessionID,
		AgentRunID:     runID,
		Tool:           strings.TrimSpace(req.Tool),
		Arguments:      req.Arguments,
	})
	if err != nil {
		h.logger.Error("agent runtime tool call", slog.String("agent_id", agentID.String()), slog.String("agent_run_id", runID.String()), slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		appID, functionID, connectorInstanceID, agentID, operation, ok := parseSubscriptionRequestFields(w, req.ConnectedAppID, req.FunctionDestinationID, req.ConnectorInstanceID, req.AgentID, req.Operation)
		if !ok {
			return
		}
		sessionCreateIfMissing := true
		if req.SessionCreateIfMissing != nil {
			sessionCreateIfMissing = *req.SessionCreateIfMissing
		}
		result, err := h.subSvc.Create(r.Context(), tenantID, req.DestinationType, appID, functionID, connectorInstanceID, agentID, req.SessionKeyTemplate, sessionCreateIfMissing, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
		if err != nil {
			switch {
			case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
				writeError(w, http.StatusBadRequest, err.Error())
			case isFilterValidationError(err):
				writeSubscriptionFilterError(w, err)
			case errors.Is(err, subscription.ErrConnectedAppNotFound):
				writeError(w, http.StatusNotFound, "connected app not found")
			case errors.Is(err, subscription.ErrFunctionDestinationNotFound):
				writeError(w, http.StatusNotFound, "function destination not found")
			case errors.Is(err, subscription.ErrConnectorInstanceNotFound):
				writeError(w, http.StatusNotFound, "connector instance not found")
			case errors.Is(err, subscription.ErrConnectorInstanceForbidden):
				writeError(w, http.StatusNotFound, "connector instance not found")
			case errors.Is(err, subscription.ErrGlobalConnectorNotAllowed):
				writeError(w, http.StatusBadRequest, err.Error())
			default:
				h.logger.Error("create subscription", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create subscription")
			}
			return
		}
		h.audit("subscription.create", "subscription", &result.Subscription.ID, map[string]any{
			"destination_type": result.Subscription.DestinationType,
			"event_type":       result.Subscription.EventType,
		}, r.Context())
		writeJSON(w, http.StatusCreated, subscriptionResponse(result))
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

func (h *Handler) replaceSubscription(w http.ResponseWriter, r *http.Request) {
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	appID, functionID, connectorInstanceID, agentID, operation, ok := parseSubscriptionRequestFields(w, req.ConnectedAppID, req.FunctionDestinationID, req.ConnectorInstanceID, req.AgentID, req.Operation)
	if !ok {
		return
	}
	sessionCreateIfMissing := true
	if req.SessionCreateIfMissing != nil {
		sessionCreateIfMissing = *req.SessionCreateIfMissing
	}
	result, err := h.subSvc.Update(r.Context(), tenantID, subscriptionID, req.DestinationType, appID, functionID, connectorInstanceID, agentID, req.SessionKeyTemplate, sessionCreateIfMissing, operation, req.OperationParams, req.Filter, req.EventType, req.EventSource, req.EmitSuccessEvent, req.EmitFailureEvent)
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrInvalidEventType), errors.Is(err, subscription.ErrInvalidDestinationType), errors.Is(err, subscription.ErrInvalidOperation), errors.Is(err, subscription.ErrInvalidOperationParams):
			writeError(w, http.StatusBadRequest, err.Error())
		case isFilterValidationError(err):
			writeSubscriptionFilterError(w, err)
		case errors.Is(err, subscription.ErrConnectedAppNotFound):
			writeError(w, http.StatusNotFound, "connected app not found")
		case errors.Is(err, subscription.ErrFunctionDestinationNotFound):
			writeError(w, http.StatusNotFound, "function destination not found")
		case errors.Is(err, subscription.ErrConnectorInstanceNotFound), errors.Is(err, subscription.ErrConnectorInstanceForbidden), errors.Is(err, subscription.ErrSubscriptionNotFound):
			writeError(w, http.StatusNotFound, "subscription not found")
		case errors.Is(err, subscription.ErrGlobalConnectorNotAllowed):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error("replace subscription", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to replace subscription")
		}
		return
	}
	h.audit("subscription.update", "subscription", &result.Subscription.ID, map[string]any{
		"destination_type": result.Subscription.DestinationType,
		"event_type":       result.Subscription.EventType,
	}, r.Context())
	writeJSON(w, http.StatusOK, subscriptionResponse(result))
}

func parseSubscriptionRequestFields(w http.ResponseWriter, connectedAppIDRaw, functionDestinationIDRaw, connectorInstanceIDRaw, agentIDRaw, operationRaw string) (*uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool) {
	var appID *uuid.UUID
	if strings.TrimSpace(connectedAppIDRaw) != "" {
		parsed, err := uuid.Parse(connectedAppIDRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connected_app_id")
			return nil, nil, nil, nil, nil, false
		}
		appID = &parsed
	}
	var functionID *uuid.UUID
	if strings.TrimSpace(functionDestinationIDRaw) != "" {
		parsed, err := uuid.Parse(functionDestinationIDRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid function_destination_id")
			return nil, nil, nil, nil, nil, false
		}
		functionID = &parsed
	}
	var connectorInstanceID *uuid.UUID
	if strings.TrimSpace(connectorInstanceIDRaw) != "" {
		parsed, err := uuid.Parse(connectorInstanceIDRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connector_instance_id")
			return nil, nil, nil, nil, nil, false
		}
		connectorInstanceID = &parsed
	}
	var agentID *uuid.UUID
	if strings.TrimSpace(agentIDRaw) != "" {
		parsed, err := uuid.Parse(agentIDRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid agent_id")
			return nil, nil, nil, nil, nil, false
		}
		agentID = &parsed
	}
	var operation *string
	if strings.TrimSpace(operationRaw) != "" {
		trimmed := strings.TrimSpace(operationRaw)
		operation = &trimmed
	}
	return appID, functionID, connectorInstanceID, agentID, operation, true
}

func subscriptionResponse(result subscription.Result) map[string]any {
	response := map[string]any{"subscription": result.Subscription}
	if len(result.Warnings) > 0 {
		response["warnings"] = result.Warnings
	}
	return response
}

func isFilterValidationError(err error) bool {
	var filterErr subscriptionfilter.ValidationError
	return errors.As(err, &filterErr)
}

func writeSubscriptionFilterError(w http.ResponseWriter, err error) {
	var filterErr subscriptionfilter.ValidationError
	if !errors.As(err, &filterErr) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":         filterErr.Error(),
		"invalid_paths": filterErr.InvalidPaths,
		"invalid_ops":   filterErr.InvalidOps,
	})
}

func bearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (h *Handler) connectorInstances(w http.ResponseWriter, r *http.Request) {
	if h.connectorSvc == nil {
		writeError(w, http.StatusNotImplemented, "connector instance service unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectorName string          `json:"connector_name"`
			Scope         string          `json:"scope"`
			Config        json.RawMessage `json:"config"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var tenantID *tenant.ID
		switch strings.TrimSpace(req.Scope) {
		case "", connectorinstance.ScopeTenant:
			resolvedTenantID, ok := tenantIDFromContext(r.Context())
			if !ok {
				authenticated, resolvedTenantIDValue, authenticatedOK := h.authenticateTenantRequest(r)
				if authenticatedOK {
					r = authenticated
					resolvedTenantID = resolvedTenantIDValue
				}
				ok = authenticatedOK
			}
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tenantID = &resolvedTenantID
		case connectorinstance.ScopeGlobal:
			if !h.isSystemAuthorized(r) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		default:
			writeError(w, http.StatusBadRequest, connectorinstance.ErrInvalidScope.Error())
			return
		}

		instance, err := h.connectorSvc.Create(r.Context(), tenantID, req.ConnectorName, req.Scope, req.Config)
		if err != nil {
			switch {
			case errors.Is(err, connectorinstance.ErrUnsupportedConnector), errors.Is(err, connectorinstance.ErrInvalidConfig), errors.Is(err, connectorinstance.ErrMissingBotToken), errors.Is(err, connectorinstance.ErrMissingWebhookSecret), errors.Is(err, connectorinstance.ErrMissingStripeAccount), errors.Is(err, connectorinstance.ErrMissingNotionToken), errors.Is(err, connectorinstance.ErrMissingLLMProviders), errors.Is(err, connectorinstance.ErrInvalidLLMProvider), errors.Is(err, connectorinstance.ErrMissingLLMAPIKey), errors.Is(err, connectorinstance.ErrInvalidScope), errors.Is(err, connectorinstance.ErrGlobalNotAllowed), errors.Is(err, connectorinstance.ErrTenantOnlyConnector), errors.Is(err, connectorinstance.ErrGlobalOnlyConnector):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, connectorinstance.ErrDuplicateInstance):
				writeError(w, http.StatusConflict, err.Error())
			default:
				h.logger.Error("create connector instance", slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create connector instance")
			}
			return
		}
		h.audit("connector_instance.create", "connector_instance", &instance.ID, map[string]any{
			"connector_name": instance.ConnectorName,
			"scope":          instance.Scope,
		}, r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"id": instance.ID.String()})
	case http.MethodGet:
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
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

func (h *Handler) inboundRoutes(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.inboundRouteSvc == nil {
		writeError(w, http.StatusNotImplemented, "inbound route service unavailable")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			ConnectorName       string `json:"connector_name"`
			RouteKey            string `json:"route_key"`
			ConnectorInstanceID string `json:"connector_instance_id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
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
		route, err := h.inboundRouteSvc.Create(r.Context(), tenantID, req.ConnectorName, req.RouteKey, connectorInstanceID)
		if err != nil {
			switch {
			case errors.Is(err, inboundroute.ErrInvalidConnectorName), errors.Is(err, inboundroute.ErrInvalidRouteKey), errors.Is(err, inboundroute.ErrInvalidConnectorInstance):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, inboundroute.ErrConnectorInstanceNotFound):
				writeError(w, http.StatusNotFound, "connector instance not found")
			case errors.Is(err, inboundroute.ErrDuplicateRoute):
				writeError(w, http.StatusConflict, err.Error())
			default:
				h.logger.Error("create inbound route", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
				writeError(w, http.StatusInternalServerError, "failed to create inbound route")
			}
			return
		}
		h.logger.Info("inbound_route_created", slog.String("tenant_id", tenantID.String()), slog.String("connector_name", route.ConnectorName), slog.String("route_key", route.RouteKey))
		writeJSON(w, http.StatusOK, map[string]string{"id": route.ID.String()})
	case http.MethodGet:
		routes, err := h.inboundRouteSvc.List(r.Context(), tenantID)
		if err != nil {
			h.logger.Error("list inbound routes", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to list inbound routes")
			return
		}
		writeJSON(w, http.StatusOK, routes)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) systemInboundRoutes(w http.ResponseWriter, r *http.Request) {
	if h.inboundRouteSvc == nil {
		writeError(w, http.StatusNotImplemented, "inbound route service unavailable")
		return
	}
	routes, err := h.inboundRouteSvc.ListAll(r.Context())
	if err != nil {
		h.logger.Error("list system inbound routes", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to list inbound routes")
		return
	}
	writeJSON(w, http.StatusOK, routes)
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
	action := "subscription.resume"
	if strings.HasSuffix(r.URL.Path, "/pause") {
		action = "subscription.pause"
	}
	h.audit(action, "subscription", &sub.ID, map[string]any{"status": sub.Status}, r.Context())

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
	h.logger.Info("delivery_retry_requested", slog.String("tenant_id", tenantID.String()), slog.String("delivery_id", deliveryID.String()))
	writeJSON(w, http.StatusOK, map[string]string{"status": job.Status})
}

func (h *Handler) replayEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.replaySvc == nil {
		writeError(w, http.StatusNotImplemented, "replay service unavailable")
		return
	}
	eventID, err := uuid.Parse(r.PathValue("event_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event_id")
		return
	}
	h.logger.Info("event_replay_single_requested", slog.String("tenant_id", tenantID.String()), slog.String("event_id", eventID.String()))
	result, err := h.replaySvc.ReplayEvent(r.Context(), tenantID, eventID)
	if err != nil {
		switch {
		case errors.Is(err, replay.ErrEventNotFound):
			writeError(w, http.StatusNotFound, "event not found")
		default:
			h.logger.Error("replay event", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to replay event")
		}
		return
	}
	h.logger.Info("event_replay_completed", slog.String("tenant_id", tenantID.String()), slog.String("event_id", result.EventID.String()), slog.Int("jobs_created", result.JobsCreated))
	writeJSON(w, http.StatusOK, map[string]any{
		"event_id":              result.EventID.String(),
		"matched_subscriptions": result.MatchedSubscriptions,
		"jobs_created":          result.JobsCreated,
	})
}

func (h *Handler) replayEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.replaySvc == nil {
		writeError(w, http.StatusNotImplemented, "replay service unavailable")
		return
	}
	var req struct {
		From           string `json:"from"`
		To             string `json:"to"`
		Type           string `json:"type"`
		Source         string `json:"source"`
		SubscriptionID string `json:"subscription_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
	var subscriptionID *uuid.UUID
	if strings.TrimSpace(req.SubscriptionID) != "" {
		parsed, err := uuid.Parse(req.SubscriptionID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid subscription_id")
			return
		}
		subscriptionID = &parsed
	}
	h.logger.Info("event_replay_query_requested", slog.String("tenant_id", tenantID.String()), slog.String("from", from.Format(time.RFC3339)), slog.String("to", to.Format(time.RFC3339)))
	result, err := h.replaySvc.ReplayQuery(r.Context(), tenantID, replay.QueryRequest{
		From:           from,
		To:             to,
		Type:           strings.TrimSpace(req.Type),
		Source:         strings.TrimSpace(req.Source),
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		switch {
		case errors.Is(err, replay.ErrInvalidWindow), errors.Is(err, replay.ErrReplayLimitExceeded), errors.Is(err, replay.ErrSubscriptionInactive):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, replay.ErrSubscriptionNotFound):
			writeError(w, http.StatusNotFound, "subscription not found")
		default:
			h.logger.Error("replay events", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to replay events")
		}
		return
	}
	h.logger.Info("event_replay_completed", slog.String("tenant_id", tenantID.String()), slog.String("from", from.Format(time.RFC3339)), slog.String("to", to.Format(time.RFC3339)), slog.Int("events_scanned", result.EventsScanned), slog.Int("jobs_created", result.JobsCreated))
	writeJSON(w, http.StatusOK, map[string]any{
		"events_scanned": result.EventsScanned,
		"jobs_created":   result.JobsCreated,
	})
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

func (h *Handler) stripeEnable(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.stripeSvc == nil {
		writeError(w, http.StatusNotImplemented, "stripe service unavailable")
		return
	}
	var req struct {
		StripeAccountID string `json:"stripe_account_id"`
		WebhookSecret   string `json:"webhook_secret"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	instanceID, err := h.stripeSvc.Enable(r.Context(), tenantID, req.StripeAccountID, req.WebhookSecret)
	if err != nil {
		switch {
		case errors.Is(err, stripeconnector.ErrInvalidAccountID), errors.Is(err, stripeconnector.ErrInvalidSecret):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, stripeconnector.ErrRouteConflict):
			writeError(w, http.StatusConflict, err.Error())
		default:
			h.logger.Error("enable stripe connector", slog.String("tenant_id", tenantID.String()), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "failed to enable stripe connector")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"connector_instance_id": instanceID.String()})
}

func (h *Handler) slackWebhook(w http.ResponseWriter, r *http.Request) {
	if h.slackSvc == nil {
		writeError(w, http.StatusNotImplemented, "slack service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	result, err := h.slackSvc.HandleEvents(r.Context(), rawBody, r.Header.Clone())
	if err != nil {
		h.logger.Error("handle slack webhook", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	if result.IsChallenge {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": result.Challenge})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

func (h *Handler) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.stripeSvc == nil {
		writeError(w, http.StatusNotImplemented, "stripe service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()

	if err := h.stripeSvc.HandleWebhook(r.Context(), rawBody, r.Header.Clone()); err != nil {
		if errors.Is(err, stripeconnector.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.logger.Error("handle stripe webhook", slog.String("error", err.Error()))
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
	var replayOfEventID any
	if job.ReplayOfEventID != nil {
		replayOfEventID = job.ReplayOfEventID.String()
	}
	return map[string]any{
		"id":                 job.ID.String(),
		"subscription_id":    job.SubscriptionID.String(),
		"event_id":           job.EventID.String(),
		"is_replay":          job.IsReplay,
		"replay_of_event_id": replayOfEventID,
		"status":             job.Status,
		"attempts":           job.Attempts,
		"last_error":         job.LastError,
		"external_id":        job.ExternalID,
		"last_status_code":   job.LastStatusCode,
		"result_event_id":    optionalUUIDValue(job.ResultEventID),
		"created_at":         job.CreatedAt,
		"completed_at":       job.CompletedAt,
	}
}

func (h *Handler) audit(action, resourceType string, resourceID *uuid.UUID, metadata map[string]any, ctx context.Context) {
	if h.auditSvc == nil {
		return
	}
	if err := h.auditSvc.Audit(ctx, action, resourceType, resourceID, metadata); err != nil {
		h.logger.Error("audit_failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func (h *Handler) adminAudit(tenantID tenant.ID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any, r *http.Request) {
	if h.auditSvc == nil {
		return
	}
	if err := h.auditSvc.AuditForTenant(r.Context(), tenantID, action, resourceType, resourceID, metadata); err != nil {
		h.logger.Error("audit_failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func optionalUUIDValue(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}
