package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"groot/internal/agent"
	"groot/internal/apikey"
	iauth "groot/internal/auth"
	"groot/internal/connectedapp"
	"groot/internal/connection"
	"groot/internal/delivery"
	"groot/internal/edition"
	"groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/graph"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/integrations/catalog"
	"groot/internal/integrations/resend"
	"groot/internal/integrations/slack"
	"groot/internal/observability"
	"groot/internal/replay"
	"groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
	"groot/internal/tenant"
	"groot/internal/workflow"
	builderapi "groot/internal/workflow/builderapi"
	workflowpublish "groot/internal/workflow/publish"
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
	Ingest(context.Context, ingest.Request) (event.Event, error)
}

type EventQueryService interface {
	List(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]event.ListEvent, error)
	AdminList(context.Context, tenant.ID, string, *time.Time, *time.Time, int, bool) ([]event.AdminEvent, error)
	AdminGet(context.Context, uuid.UUID, bool) (event.AdminEvent, error)
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
	ListVersions(context.Context, tenant.ID, uuid.UUID) ([]agent.Version, error)
	Delete(context.Context, tenant.ID, uuid.UUID) error
	ListSessions(context.Context, tenant.ID, *uuid.UUID, string, int) ([]agent.Session, error)
	GetSession(context.Context, tenant.ID, uuid.UUID) (agent.Session, error)
	CloseSession(context.Context, tenant.ID, uuid.UUID) (agent.Session, error)
}

type AgentToolService interface {
	ExecuteTool(context.Context, agent.ToolExecutionRequest) (agent.ToolExecutionResult, error)
}

type WorkflowService interface {
	Create(context.Context, tenant.ID, string, string) (workflow.Workflow, error)
	List(context.Context, tenant.ID) ([]workflow.Workflow, error)
	Get(context.Context, tenant.ID, uuid.UUID) (workflow.Workflow, error)
	Update(context.Context, tenant.ID, uuid.UUID, string, string) (workflow.Workflow, error)
	CreateVersion(context.Context, tenant.ID, uuid.UUID, json.RawMessage) (workflow.Version, error)
	ListVersions(context.Context, tenant.ID, uuid.UUID) ([]workflow.Version, error)
	GetVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Version, error)
	UpdateVersion(context.Context, tenant.ID, uuid.UUID, json.RawMessage) (workflow.Version, error)
	ValidateVersion(context.Context, tenant.ID, uuid.UUID) (workflow.ValidateResult, error)
	CompileVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Version, error)
}

type WorkflowPublishService interface {
	Publish(context.Context, tenant.ID, uuid.UUID) (workflowpublish.PublishResult, error)
	Unpublish(context.Context, tenant.ID, uuid.UUID) (workflowpublish.UnpublishResult, error)
	ArtifactsByWorkflow(context.Context, tenant.ID, uuid.UUID) (workflow.Artifacts, error)
	ArtifactsByVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Artifacts, error)
}

type WorkflowRuntimeService interface {
	ListRuns(context.Context, tenant.ID, uuid.UUID, int) ([]workflow.Run, error)
	GetRun(context.Context, tenant.ID, uuid.UUID) (workflow.Run, error)
	ListRunSteps(context.Context, tenant.ID, uuid.UUID) ([]workflow.RunStep, error)
	ListRunWaits(context.Context, tenant.ID, uuid.UUID) ([]workflow.RunWait, error)
	CancelRun(context.Context, tenant.ID, uuid.UUID) (workflow.Run, error)
}

type WorkflowBuilderService interface {
	NodeTypes() builderapi.NodeTypesResponse
	TriggerIntegrations() builderapi.TriggerIntegrationsResponse
	ActionIntegrations() builderapi.ActionIntegrationsResponse
	ListConnections(context.Context, tenant.ID, string, string, string) (builderapi.ConnectionsResponse, error)
	ListAgents(context.Context, tenant.ID) (builderapi.AgentsResponse, error)
	ListAgentVersions(context.Context, tenant.ID, uuid.UUID) (builderapi.AgentVersionsResponse, error)
	WaitStrategies() builderapi.WaitStrategiesResponse
	ArtifactMap(context.Context, tenant.ID, uuid.UUID) (builderapi.ArtifactMapResponse, error)
}

type ConnectionService interface {
	Create(context.Context, *tenant.ID, string, string, json.RawMessage) (connection.Instance, error)
	List(context.Context, tenant.ID) ([]connection.Instance, error)
	ListAll(context.Context) ([]connection.Instance, error)
	AdminList(context.Context, *tenant.ID, string, string) ([]connection.Instance, error)
	AdminUpsert(context.Context, uuid.UUID, *tenant.ID, string, string, json.RawMessage) (connection.Instance, error)
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
	List(context.Context) ([]schema.Schema, error)
	Get(context.Context, string) (schema.Schema, error)
}

type IntegrationCatalogService interface {
	List(context.Context) ([]catalog.IntegrationSummary, error)
	Get(context.Context, string) (catalog.IntegrationDetail, error)
	ListOperations(context.Context, string) ([]catalog.OperationCatalog, error)
	ListSchemas(context.Context, string) ([]catalog.SchemaCatalog, error)
	GetConfig(context.Context, string) (catalog.ConfigCatalog, error)
}

type SlackService interface {
	HandleEvents(context.Context, []byte, http.Header) (slack.Result, error)
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
	Connections              ConnectionService
	InboundRoutes            InboundRouteService
	Deliveries               DeliveryService
	Replay                   ReplayService
	AdminReplay              ReplayService
	Schemas                  SchemaService
	IntegrationCatalog       IntegrationCatalogService
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
	Workflows                WorkflowService
	WorkflowPublish          WorkflowPublishService
	WorkflowRuntime          WorkflowRuntimeService
	WorkflowBuilder          WorkflowBuilderService
	SystemAPIKey             string
	AgentRuntimeSharedSecret string
	Edition                  edition.Runtime
	CommunityBootstrapTenant *uuid.UUID
	Metrics                  *observability.Metrics
}

type State struct {
	Logger                   *slog.Logger
	Checkers                 []NamedChecker
	RouterCheckers           []NamedChecker
	DeliveryCheckers         []NamedChecker
	TenantSvc                TenantService
	EventSvc                 EventService
	EventQuerySvc            EventQueryService
	AppSvc                   ConnectedAppService
	FunctionSvc              FunctionDestinationService
	SubSvc                   SubscriptionService
	ConnectionSvc            ConnectionService
	InboundRouteSvc          InboundRouteService
	DeliverySvc              DeliveryService
	ReplaySvc                ReplayService
	AdminReplaySvc           ReplayService
	SchemaSvc                SchemaService
	IntegrationCatalogSvc    IntegrationCatalogService
	ResendSvc                ResendService
	SlackSvc                 SlackService
	StripeSvc                StripeService
	Metrics                  *observability.Metrics
	AuthTenantFn             func(context.Context, string) (tenant.Tenant, error)
	AuthSvc                  Authenticator
	AdminAuthSvc             Authenticator
	AdminRateLimitRPS        int
	APIKeySvc                APIKeyService
	GraphSvc                 GraphService
	AuditSvc                 AuditService
	AgentSvc                 AgentService
	AgentToolSvc             AgentToolService
	WorkflowSvc              WorkflowService
	WorkflowPublishSvc       WorkflowPublishService
	WorkflowRuntimeSvc       WorkflowRuntimeService
	WorkflowBuilderSvc       WorkflowBuilderService
	SystemAPIKey             string
	AgentRuntimeSharedSecret string
	EditionRuntime           edition.Runtime
	CommunityBootstrapTenant *uuid.UUID
	AdminEnabled             bool
	AdminAllowViewPayloads   bool
	AdminReplayEnabled       bool
	AdminReplayMaxEvents     int
}

func NewState(opts Options) *State {
	state := &State{
		Logger:                   opts.Logger,
		Checkers:                 opts.Checkers,
		RouterCheckers:           opts.RouterCheckers,
		DeliveryCheckers:         opts.DeliveryCheckers,
		TenantSvc:                opts.Tenants,
		EventSvc:                 opts.EventSvc,
		EventQuerySvc:            opts.EventQuerySvc,
		AppSvc:                   opts.Apps,
		FunctionSvc:              opts.Functions,
		SubSvc:                   opts.Subs,
		ConnectionSvc:            opts.Connections,
		InboundRouteSvc:          opts.InboundRoutes,
		DeliverySvc:              opts.Deliveries,
		ReplaySvc:                opts.Replay,
		AdminReplaySvc:           opts.AdminReplay,
		SchemaSvc:                opts.Schemas,
		IntegrationCatalogSvc:    opts.IntegrationCatalog,
		ResendSvc:                opts.Resend,
		SlackSvc:                 opts.Slack,
		StripeSvc:                opts.Stripe,
		Metrics:                  opts.Metrics,
		AuthSvc:                  opts.Auth,
		AdminAuthSvc:             opts.AdminAuth,
		AdminRateLimitRPS:        opts.AdminRateLimitRPS,
		APIKeySvc:                opts.APIKeys,
		GraphSvc:                 opts.Graph,
		AuditSvc:                 opts.Audit,
		AgentSvc:                 opts.Agents,
		AgentToolSvc:             opts.AgentTools,
		WorkflowSvc:              opts.Workflows,
		WorkflowPublishSvc:       opts.WorkflowPublish,
		WorkflowRuntimeSvc:       opts.WorkflowRuntime,
		WorkflowBuilderSvc:       opts.WorkflowBuilder,
		SystemAPIKey:             opts.SystemAPIKey,
		AgentRuntimeSharedSecret: opts.AgentRuntimeSharedSecret,
		EditionRuntime:           opts.Edition,
		CommunityBootstrapTenant: opts.CommunityBootstrapTenant,
		AdminEnabled:             opts.AdminEnabled,
		AdminAllowViewPayloads:   opts.AdminAllowViewPayloads,
		AdminReplayEnabled:       opts.AdminReplayEnabled,
		AdminReplayMaxEvents:     opts.AdminReplayMaxEvents,
	}
	if state.Logger == nil {
		state.Logger = slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	}
	if opts.Tenants != nil {
		state.AuthTenantFn = opts.Tenants.Authenticate
	}
	return state
}

func (s *State) CommunityEditionRestriction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.EditionRuntime.IsCommunity() {
			WriteError(w, http.StatusForbidden, "community_edition_restriction")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *State) RunChecks(w http.ResponseWriter, r *http.Request, checkers []NamedChecker) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	failures := make(map[string]string)
	for _, checker := range checkers {
		if err := checker.Checker.Check(ctx); err != nil {
			failures[checker.Name] = err.Error()
		}
	}

	if len(failures) > 0 {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"status": "error",
			"checks": failures,
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *State) Audit(action, resourceType string, resourceID *uuid.UUID, metadata map[string]any, ctx context.Context) {
	if s.AuditSvc == nil {
		return
	}
	if err := s.AuditSvc.Audit(ctx, action, resourceType, resourceID, metadata); err != nil {
		s.Logger.Error("audit_failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func (s *State) AdminAudit(tenantID tenant.ID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any, ctx context.Context) {
	if s.AuditSvc == nil {
		return
	}
	if err := s.AuditSvc.AuditForTenant(ctx, tenantID, action, resourceType, resourceID, metadata); err != nil {
		s.Logger.Error("audit_failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func WriteSubscriptionFilterError(w http.ResponseWriter, err error) {
	var filterErr subscriptionfilter.ValidationError
	if !errors.As(err, &filterErr) {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	WriteJSON(w, http.StatusBadRequest, map[string]any{
		"error":         filterErr.Error(),
		"invalid_paths": filterErr.InvalidPaths,
		"invalid_ops":   filterErr.InvalidOps,
	})
}
