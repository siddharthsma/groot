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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/apikey"
	iauth "groot/internal/auth"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/catalog"
	"groot/internal/connectors/providers/resend"
	slackconnector "groot/internal/connectors/providers/slack"
	stripeconnector "groot/internal/connectors/providers/stripe"
	"groot/internal/delivery"
	"groot/internal/edition"
	eventpkg "groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/graph"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/replay"
	schemapkg "groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type stubChecker struct {
	err error
}

func (s stubChecker) Check(context.Context) error {
	return s.err
}

type stubTenantService struct {
	createFn       func(context.Context, string) (tenant.CreatedTenant, error)
	listFn         func(context.Context) ([]tenant.Tenant, error)
	getFn          func(context.Context, uuid.UUID) (tenant.Tenant, error)
	updateFn       func(context.Context, uuid.UUID, string) (tenant.Tenant, error)
	authenticateFn func(context.Context, string) (tenant.Tenant, error)
}

func (s stubTenantService) CreateTenant(ctx context.Context, name string) (tenant.CreatedTenant, error) {
	return s.createFn(ctx, name)
}

func (s stubTenantService) ListTenants(ctx context.Context) ([]tenant.Tenant, error) {
	return s.listFn(ctx)
}

func (s stubTenantService) GetTenant(ctx context.Context, id uuid.UUID) (tenant.Tenant, error) {
	return s.getFn(ctx, id)
}

func (s stubTenantService) UpdateTenantName(ctx context.Context, id uuid.UUID, name string) (tenant.Tenant, error) {
	return s.updateFn(ctx, id, name)
}

func (s stubTenantService) Authenticate(ctx context.Context, apiKey string) (tenant.Tenant, error) {
	return s.authenticateFn(ctx, apiKey)
}

type stubAuthService struct {
	principal iauth.Principal
	err       error
}

func (s stubAuthService) AuthenticateRequest(*http.Request) (iauth.Principal, error) {
	if s.err != nil {
		return iauth.Principal{}, s.err
	}
	return s.principal, nil
}

type stubAPIKeyService struct {
	createFn func(context.Context, tenant.ID, string) (apikey.CreatedAPIKey, error)
	listFn   func(context.Context, tenant.ID) ([]apikey.APIKey, error)
	revokeFn func(context.Context, tenant.ID, uuid.UUID) (apikey.APIKey, error)
}

func (s stubAPIKeyService) Create(ctx context.Context, tenantID tenant.ID, name string) (apikey.CreatedAPIKey, error) {
	return s.createFn(ctx, tenantID, name)
}

func (s stubAPIKeyService) List(ctx context.Context, tenantID tenant.ID) ([]apikey.APIKey, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubAPIKeyService) Revoke(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (apikey.APIKey, error) {
	return s.revokeFn(ctx, tenantID, id)
}

type stubAuditService struct {
	auditFn          func(context.Context, string, string, *uuid.UUID, map[string]any) error
	auditForTenantFn func(context.Context, tenant.ID, string, string, *uuid.UUID, map[string]any) error
}

func (s stubAuditService) Audit(ctx context.Context, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	if s.auditFn == nil {
		return nil
	}
	return s.auditFn(ctx, action, resourceType, resourceID, metadata)
}

func (s stubAuditService) AuditForTenant(ctx context.Context, tenantID tenant.ID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	if s.auditForTenantFn == nil {
		return nil
	}
	return s.auditForTenantFn(ctx, tenantID, action, resourceType, resourceID, metadata)
}

type stubEventService struct {
	ingestFn func(context.Context, ingest.Request) (eventpkg.Event, error)
}

func (s stubEventService) Ingest(ctx context.Context, req ingest.Request) (eventpkg.Event, error) {
	return s.ingestFn(ctx, req)
}

type stubEventQueryService struct {
	listFn      func(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]eventpkg.ListEvent, error)
	adminListFn func(context.Context, tenant.ID, string, *time.Time, *time.Time, int, bool) ([]eventpkg.AdminEvent, error)
	adminGetFn  func(context.Context, uuid.UUID, bool) (eventpkg.AdminEvent, error)
}

func (s stubEventQueryService) List(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]eventpkg.ListEvent, error) {
	return s.listFn(ctx, tenantID, eventType, source, from, to, limit)
}

func (s stubEventQueryService) AdminList(ctx context.Context, tenantID tenant.ID, eventType string, from, to *time.Time, limit int, includePayload bool) ([]eventpkg.AdminEvent, error) {
	return s.adminListFn(ctx, tenantID, eventType, from, to, limit, includePayload)
}

func (s stubEventQueryService) AdminGet(ctx context.Context, eventID uuid.UUID, includePayload bool) (eventpkg.AdminEvent, error) {
	return s.adminGetFn(ctx, eventID, includePayload)
}

type stubConnectedAppService struct {
	createFn func(context.Context, tenant.ID, string, string) (connectedapp.App, error)
	listFn   func(context.Context, tenant.ID) ([]connectedapp.App, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type stubFunctionDestinationService struct {
	createFn func(context.Context, tenant.ID, string, string) (functiondestination.CreatedDestination, error)
	listFn   func(context.Context, tenant.ID) ([]functiondestination.Destination, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
	deleteFn func(context.Context, tenant.ID, uuid.UUID) error
}

func (s stubFunctionDestinationService) Create(ctx context.Context, tenantID tenant.ID, name, rawURL string) (functiondestination.CreatedDestination, error) {
	return s.createFn(ctx, tenantID, name, rawURL)
}
func (s stubFunctionDestinationService) List(ctx context.Context, tenantID tenant.ID) ([]functiondestination.Destination, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubFunctionDestinationService) Get(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (functiondestination.Destination, error) {
	return s.getFn(ctx, tenantID, id)
}
func (s stubFunctionDestinationService) Delete(ctx context.Context, tenantID tenant.ID, id uuid.UUID) error {
	return s.deleteFn(ctx, tenantID, id)
}

func (s stubConnectedAppService) Create(ctx context.Context, tenantID tenant.ID, name, destinationURL string) (connectedapp.App, error) {
	return s.createFn(ctx, tenantID, name, destinationURL)
}

func (s stubConnectedAppService) List(ctx context.Context, tenantID tenant.ID) ([]connectedapp.App, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubConnectedAppService) Get(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (connectedapp.App, error) {
	return s.getFn(ctx, tenantID, appID)
}

type stubSubscriptionService struct {
	createFn    func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error)
	updateFn    func(context.Context, tenant.ID, uuid.UUID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error)
	listFn      func(context.Context, tenant.ID) ([]subscription.Subscription, error)
	adminListFn func(context.Context, *tenant.ID, string, string) ([]subscription.Subscription, error)
	pauseFn     func(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
	resumeFn    func(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
}

func (s stubSubscriptionService) Create(ctx context.Context, tenantID tenant.ID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectorInstanceID *uuid.UUID, agentID *uuid.UUID, sessionKeyTemplate *string, sessionCreateIfMissing bool, operation *string, operationParams json.RawMessage, filter json.RawMessage, eventType string, eventSource *string, emitSuccessEvent bool, emitFailureEvent bool) (subscription.Result, error) {
	return s.createFn(ctx, tenantID, destinationType, connectedAppID, functionDestinationID, connectorInstanceID, agentID, sessionKeyTemplate, sessionCreateIfMissing, operation, operationParams, filter, eventType, eventSource, emitSuccessEvent, emitFailureEvent)
}

func (s stubSubscriptionService) Update(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectorInstanceID *uuid.UUID, agentID *uuid.UUID, sessionKeyTemplate *string, sessionCreateIfMissing bool, operation *string, operationParams json.RawMessage, filter json.RawMessage, eventType string, eventSource *string, emitSuccessEvent bool, emitFailureEvent bool) (subscription.Result, error) {
	return s.updateFn(ctx, tenantID, subscriptionID, destinationType, connectedAppID, functionDestinationID, connectorInstanceID, agentID, sessionKeyTemplate, sessionCreateIfMissing, operation, operationParams, filter, eventType, eventSource, emitSuccessEvent, emitFailureEvent)
}

func (s stubSubscriptionService) List(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubSubscriptionService) AdminList(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]subscription.Subscription, error) {
	return s.adminListFn(ctx, tenantID, eventType, destinationType)
}

func (s stubSubscriptionService) Pause(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	return s.pauseFn(ctx, tenantID, subscriptionID)
}

func (s stubSubscriptionService) Resume(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	return s.resumeFn(ctx, tenantID, subscriptionID)
}

type stubDeliveryService struct {
	listFn      func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]delivery.Job, error)
	adminListFn func(context.Context, tenant.ID, string, *time.Time, *time.Time, int) ([]delivery.Job, error)
	getFn       func(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
	retryFn     func(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
}

func (s stubDeliveryService) AdminList(ctx context.Context, tenantID tenant.ID, status string, from, to *time.Time, limit int) ([]delivery.Job, error) {
	return s.adminListFn(ctx, tenantID, status, from, to, limit)
}

type stubReplayService struct {
	replayEventFn func(context.Context, tenant.ID, uuid.UUID) (replay.SingleResult, error)
	replayQueryFn func(context.Context, tenant.ID, replay.QueryRequest) (replay.QueryResult, error)
}

func (s stubReplayService) ReplayEvent(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (replay.SingleResult, error) {
	return s.replayEventFn(ctx, tenantID, eventID)
}

func (s stubReplayService) ReplayQuery(ctx context.Context, tenantID tenant.ID, req replay.QueryRequest) (replay.QueryResult, error) {
	return s.replayQueryFn(ctx, tenantID, req)
}

type stubConnectorInstanceService struct {
	createFn      func(context.Context, *tenant.ID, string, string, json.RawMessage) (connectorinstance.Instance, error)
	listFn        func(context.Context, tenant.ID) ([]connectorinstance.Instance, error)
	listAllFn     func(context.Context) ([]connectorinstance.Instance, error)
	adminListFn   func(context.Context, *tenant.ID, string, string) ([]connectorinstance.Instance, error)
	adminUpsertFn func(context.Context, uuid.UUID, *tenant.ID, string, string, json.RawMessage) (connectorinstance.Instance, error)
}

func (s stubConnectorInstanceService) Create(ctx context.Context, tenantID *tenant.ID, connectorName string, scope string, config json.RawMessage) (connectorinstance.Instance, error) {
	return s.createFn(ctx, tenantID, connectorName, scope, config)
}

func (s stubConnectorInstanceService) List(ctx context.Context, tenantID tenant.ID) ([]connectorinstance.Instance, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubConnectorInstanceService) ListAll(ctx context.Context) ([]connectorinstance.Instance, error) {
	return s.listAllFn(ctx)
}

func (s stubConnectorInstanceService) AdminList(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]connectorinstance.Instance, error) {
	return s.adminListFn(ctx, tenantID, connectorName, scope)
}

func (s stubConnectorInstanceService) AdminUpsert(ctx context.Context, id uuid.UUID, tenantID *tenant.ID, connectorName, scope string, config json.RawMessage) (connectorinstance.Instance, error) {
	return s.adminUpsertFn(ctx, id, tenantID, connectorName, scope, config)
}

type stubInboundRouteService struct {
	createFn  func(context.Context, tenant.ID, string, string, *uuid.UUID) (inboundroute.Route, error)
	listFn    func(context.Context, tenant.ID) ([]inboundroute.Route, error)
	listAllFn func(context.Context) ([]inboundroute.Route, error)
}

func (s stubInboundRouteService) Create(ctx context.Context, tenantID tenant.ID, connectorName, routeKey string, connectorInstanceID *uuid.UUID) (inboundroute.Route, error) {
	return s.createFn(ctx, tenantID, connectorName, routeKey, connectorInstanceID)
}

func (s stubInboundRouteService) List(ctx context.Context, tenantID tenant.ID) ([]inboundroute.Route, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubInboundRouteService) ListAll(ctx context.Context) ([]inboundroute.Route, error) {
	return s.listAllFn(ctx)
}

type stubResendService struct {
	bootstrapFn func(context.Context) (string, error)
	enableFn    func(context.Context, tenant.ID) (resend.EnableResult, error)
	webhookFn   func(context.Context, []byte, http.Header) error
}

func (s stubResendService) Bootstrap(ctx context.Context) (string, error) {
	return s.bootstrapFn(ctx)
}
func (s stubResendService) Enable(ctx context.Context, tenantID tenant.ID) (resend.EnableResult, error) {
	return s.enableFn(ctx, tenantID)
}
func (s stubResendService) HandleWebhook(ctx context.Context, rawBody []byte, headers http.Header) error {
	return s.webhookFn(ctx, rawBody, headers)
}

type stubStripeService struct {
	enableFn  func(context.Context, tenant.ID, string, string) (uuid.UUID, error)
	webhookFn func(context.Context, []byte, http.Header) error
}

type stubSlackService struct {
	handleEventsFn func(context.Context, []byte, http.Header) (slackconnector.Result, error)
}

type stubSchemaService struct {
	listFn func(context.Context) ([]schemapkg.Schema, error)
	getFn  func(context.Context, string) (schemapkg.Schema, error)
}

type stubProviderCatalogService struct {
	listFn           func(context.Context) ([]catalog.ProviderSummary, error)
	getFn            func(context.Context, string) (catalog.ProviderDetail, error)
	listOperationsFn func(context.Context, string) ([]catalog.OperationCatalog, error)
	listSchemasFn    func(context.Context, string) ([]catalog.SchemaCatalog, error)
	getConfigFn      func(context.Context, string) (catalog.ConfigCatalog, error)
}

type stubGraphService struct {
	topologyFn  func(context.Context, graph.TopologyRequest) (graph.Topology, error)
	executionFn func(context.Context, uuid.UUID, graph.ExecutionRequest) (graph.ExecutionGraph, error)
}

type stubAgentToolService struct {
	executeFn func(context.Context, agent.ToolExecutionRequest) (agent.ToolExecutionResult, error)
}

func (s stubStripeService) Enable(ctx context.Context, tenantID tenant.ID, stripeAccountID string, webhookSecret string) (uuid.UUID, error) {
	return s.enableFn(ctx, tenantID, stripeAccountID, webhookSecret)
}

func (s stubStripeService) HandleWebhook(ctx context.Context, rawBody []byte, headers http.Header) error {
	return s.webhookFn(ctx, rawBody, headers)
}

func (s stubSlackService) HandleEvents(ctx context.Context, rawBody []byte, headers http.Header) (slackconnector.Result, error) {
	return s.handleEventsFn(ctx, rawBody, headers)
}

func (s stubSchemaService) List(ctx context.Context) ([]schemapkg.Schema, error) {
	return s.listFn(ctx)
}

func (s stubSchemaService) Get(ctx context.Context, fullName string) (schemapkg.Schema, error) {
	return s.getFn(ctx, fullName)
}

func (s stubProviderCatalogService) List(ctx context.Context) ([]catalog.ProviderSummary, error) {
	return s.listFn(ctx)
}

func (s stubProviderCatalogService) Get(ctx context.Context, name string) (catalog.ProviderDetail, error) {
	return s.getFn(ctx, name)
}

func (s stubProviderCatalogService) ListOperations(ctx context.Context, name string) ([]catalog.OperationCatalog, error) {
	return s.listOperationsFn(ctx, name)
}

func (s stubProviderCatalogService) ListSchemas(ctx context.Context, name string) ([]catalog.SchemaCatalog, error) {
	return s.listSchemasFn(ctx, name)
}

func (s stubProviderCatalogService) GetConfig(ctx context.Context, name string) (catalog.ConfigCatalog, error) {
	return s.getConfigFn(ctx, name)
}

func (s stubGraphService) BuildTopology(ctx context.Context, req graph.TopologyRequest) (graph.Topology, error) {
	return s.topologyFn(ctx, req)
}

func (s stubGraphService) BuildExecution(ctx context.Context, eventID uuid.UUID, req graph.ExecutionRequest) (graph.ExecutionGraph, error) {
	return s.executionFn(ctx, eventID, req)
}

func (s stubAgentToolService) ExecuteTool(ctx context.Context, req agent.ToolExecutionRequest) (agent.ToolExecutionResult, error) {
	return s.executeFn(ctx, req)
}

func (s stubDeliveryService) List(ctx context.Context, tenantID tenant.ID, status string, subscriptionID, eventID *uuid.UUID, limit int) ([]delivery.Job, error) {
	return s.listFn(ctx, tenantID, status, subscriptionID, eventID, limit)
}

func (s stubDeliveryService) Get(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	return s.getFn(ctx, tenantID, jobID)
}

func (s stubDeliveryService) Retry(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	return s.retryFn(ctx, tenantID, jobID)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{Logger: testLogger()}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "{\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestReadyzSuccess(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{
		Logger: testLogger(),
		Checkers: []NamedChecker{
			{Name: "postgres", Checker: stubChecker{}},
			{Name: "kafka", Checker: stubChecker{}},
		},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestReadyzFailure(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{
		Logger: testLogger(),
		Checkers: []NamedChecker{
			{Name: "postgres", Checker: stubChecker{}},
			{Name: "kafka", Checker: stubChecker{err: errors.New("unreachable")}},
		},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusInternalServerError; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateTenant(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"example"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Edition: edition.Runtime{
			BuildEdition:     edition.EditionInternal,
			EffectiveEdition: edition.EditionInternal,
			TenancyMode:      edition.TenancyMulti,
			Capabilities: edition.Capabilities{
				MultiTenant:           true,
				CrossTenantAdmin:      true,
				TenantCreationAllowed: true,
			},
		},
		Tenants: stubTenantService{
			createFn: func(context.Context, string) (tenant.CreatedTenant, error) {
				return tenant.CreatedTenant{
					Tenant: tenant.Tenant{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")},
					APIKey: "secret",
				}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "{\"api_key\":\"secret\",\"tenant_id\":\"11111111-1111-1111-1111-111111111111\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestListTenants(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tenants", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			listFn: func(context.Context) ([]tenant.Tenant, error) {
				return []tenant.Tenant{{
					ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					Name:      "example",
					CreatedAt: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
				}}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestGetTenantNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tenants/11111111-1111-1111-1111-111111111111", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			getFn: func(context.Context, uuid.UUID) (tenant.Tenant, error) {
				return tenant.Tenant{}, tenant.ErrTenantNotFound
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCommunityEditionRestrictsTenants(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"example"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:  testLogger(),
		Edition: edition.Runtime{BuildEdition: edition.EditionCommunity, EffectiveEdition: edition.EditionCommunity, TenancyMode: edition.TenancySingle},
		Tenants: stubTenantService{
			createFn: func(context.Context, string) (tenant.CreatedTenant, error) {
				t.Fatal("unexpected create tenant call")
				return tenant.CreatedTenant{}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusForbidden; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), "community_edition_restriction") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestSystemEdition(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/system/edition", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Edition: edition.Runtime{
			BuildEdition:     edition.EditionCommunity,
			EffectiveEdition: edition.EditionCommunity,
			TenancyMode:      edition.TenancySingle,
			Capabilities: edition.Capabilities{
				MultiTenant:           false,
				CrossTenantAdmin:      false,
				TenantCreationAllowed: false,
			},
			License: edition.LicenseState{
				Present:    true,
				Valid:      true,
				Licensee:   "Acme Ltd",
				MaxTenants: 1,
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"build_edition":"community"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"effective_edition":"community"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"tenancy_mode":"single"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"license":{"present":true,"valid":true,"licensee":"Acme Ltd","max_tenants":1}`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestCreateEventUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{"type":"example","source":"manual","payload":{}}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) {
				return tenant.Tenant{}, tenant.ErrUnauthorized
			},
		},
		EventSvc: stubEventService{},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateEvent(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	eventID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{"type":"example.event.v1","source":"manual","payload":{"ok":true}}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(_ context.Context, apiKey string) (tenant.Tenant, error) {
				if apiKey != "secret" {
					t.Fatalf("apiKey = %q", apiKey)
				}
				return tenant.Tenant{ID: tenantID, Name: "example"}, nil
			},
		},
		EventSvc: stubEventService{
			ingestFn: func(_ context.Context, req ingest.Request) (eventpkg.Event, error) {
				if req.TenantID != tenantID {
					t.Fatalf("req.TenantID = %s", req.TenantID)
				}
				if string(req.Payload) != `{"ok":true}` {
					t.Fatalf("req.Payload = %s", req.Payload)
				}
				return eventpkg.Event{EventID: eventID, TenantID: tenantID, Type: req.Type, Source: req.Source, Payload: json.RawMessage(`{"ok":true}`)}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "{\"event_id\":\"11111111-1111-1111-1111-111111111111\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestCreateConnectedApp(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/connected-apps", bytes.NewBufferString(`{"name":"app","destination_url":"https://example.com/hook"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Apps: stubConnectedAppService{
			createFn: func(_ context.Context, gotTenantID tenant.ID, name, destinationURL string) (connectedapp.App, error) {
				if gotTenantID != tenantID || name != "app" || destinationURL != "https://example.com/hook" {
					t.Fatal("unexpected connected app args")
				}
				return connectedapp.App{ID: uuid.MustParse("33333333-3333-3333-3333-333333333333"), Name: name, DestinationURL: destinationURL}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateSubscriptionAppNotFound(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBufferString(`{"connected_app_id":"33333333-3333-3333-3333-333333333333","event_type":"example.event.v1"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Subs: stubSubscriptionService{
			createFn: func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error) {
				return subscription.Result{}, subscription.ErrConnectedAppNotFound
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateSubscriptionReturnsWarnings(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBufferString(`{"destination_type":"connector","connector_instance_id":"33333333-3333-3333-3333-333333333333","operation":"post_message","operation_params":{"text":"hello"},"event_type":"example.event.v1","filter":{"path":"payload.currency","op":"==","value":"usd"}}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Subs: stubSubscriptionService{
			createFn: func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, bool, *string, json.RawMessage, json.RawMessage, string, *string, bool, bool) (subscription.Result, error) {
				return subscription.Result{
					Subscription: subscription.Subscription{ID: uuid.New()},
					Warnings:     []string{"schema_missing_for_event_type"},
				}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), "schema_missing_for_event_type") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestReplaceSubscription(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	subscriptionID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	req := httptest.NewRequest(http.MethodPut, "/subscriptions/"+subscriptionID.String(), bytes.NewBufferString(`{"destination_type":"connector","connector_instance_id":"44444444-4444-4444-4444-444444444444","operation":"post_message","operation_params":{"text":"hello"},"event_type":"example.event.v1","filter":{"path":"payload.currency","op":"==","value":"usd"}}`))
	req.SetPathValue("subscription_id", subscriptionID.String())
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Subs: stubSubscriptionService{
			updateFn: func(_ context.Context, gotTenantID tenant.ID, gotSubscriptionID uuid.UUID, _ string, _ *uuid.UUID, _ *uuid.UUID, _ *uuid.UUID, _ *uuid.UUID, _ *string, _ bool, _ *string, _ json.RawMessage, filter json.RawMessage, _ string, _ *string, _ bool, _ bool) (subscription.Result, error) {
				if gotTenantID != tenantID || gotSubscriptionID != subscriptionID {
					t.Fatal("unexpected update args")
				}
				if len(filter) == 0 {
					t.Fatal("expected filter")
				}
				return subscription.Result{Subscription: subscription.Subscription{ID: subscriptionID, Filter: filter}}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"subscription"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestMetricsEndpoint(t *testing.T) {
	metrics := observability.NewMetrics()
	metrics.IncEventsReceived()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{Logger: testLogger(), Metrics: metrics}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), "groot_events_received_total 1") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestCreateFunctionDestination(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/functions", bytes.NewBufferString(`{"name":"order_processor","url":"https://example.com/fn"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Functions: stubFunctionDestinationService{
			createFn: func(_ context.Context, gotTenantID tenant.ID, name, rawURL string) (functiondestination.CreatedDestination, error) {
				if gotTenantID != tenantID || name != "order_processor" || rawURL != "https://example.com/fn" {
					t.Fatal("unexpected function args")
				}
				return functiondestination.CreatedDestination{
					Destination: functiondestination.Destination{ID: uuid.MustParse("33333333-3333-3333-3333-333333333333"), Name: name, URL: rawURL},
					Secret:      "generated",
				}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateConnectorInstance(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/connector-instances", bytes.NewBufferString(`{"connector_name":"slack","config":{"bot_token":"xoxb-test"}}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		ConnectorInstances: stubConnectorInstanceService{
			createFn: func(_ context.Context, gotTenantID *tenant.ID, connectorName string, scope string, config json.RawMessage) (connectorinstance.Instance, error) {
				if gotTenantID == nil || *gotTenantID != tenantID || connectorName != "slack" || scope != "" || !strings.Contains(string(config), "xoxb-test") {
					t.Fatal("unexpected connector instance args")
				}
				return connectorinstance.Instance{ID: uuid.MustParse("44444444-4444-4444-4444-444444444444"), ConnectorName: connectorName, Scope: connectorinstance.ScopeTenant}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestDeliveryIncludesConnectorFields(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	deliveryID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	statusCode := 200
	externalID := "12345.6789"
	req := httptest.NewRequest(http.MethodGet, "/deliveries/"+deliveryID.String(), nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Deliveries: stubDeliveryService{
			getFn: func(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error) {
				return delivery.Job{
					ID:             deliveryID,
					SubscriptionID: uuid.MustParse("66666666-6666-6666-6666-666666666666"),
					EventID:        uuid.MustParse("77777777-7777-7777-7777-777777777777"),
					Status:         delivery.StatusSucceeded,
					ExternalID:     &externalID,
					LastStatusCode: &statusCode,
					CreatedAt:      time.Now().UTC(),
				}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"external_id":"12345.6789"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"last_status_code":200`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestReplaySingleEvent(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	eventID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	req := httptest.NewRequest(http.MethodPost, "/events/"+eventID.String()+"/replay", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Replay: stubReplayService{
			replayEventFn: func(context.Context, tenant.ID, uuid.UUID) (replay.SingleResult, error) {
				return replay.SingleResult{EventID: eventID, MatchedSubscriptions: 2, JobsCreated: 2}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"jobs_created":2`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestReplayEventsByQueryBadSubscription(t *testing.T) {
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := httptest.NewRequest(http.MethodPost, "/events/replay", bytes.NewBufferString(`{"from":"2026-03-06T10:00:00Z","to":"2026-03-06T11:00:00Z"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Replay: stubReplayService{
			replayQueryFn: func(context.Context, tenant.ID, replay.QueryRequest) (replay.QueryResult, error) {
				return replay.QueryResult{}, replay.ErrReplayLimitExceeded
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestResendBootstrapRequiresSystemAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/system/resend/bootstrap", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:       testLogger(),
		SystemAPIKey: "system-secret",
		Resend: stubResendService{
			bootstrapFn: func(context.Context) (string, error) { return "bootstrapped", nil },
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestStripeEnableConflict(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/connectors/stripe/enable", strings.NewReader(`{"stripe_account_id":"acct_123","webhook_secret":"whsec_123"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) {
				return tenant.Tenant{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")}, nil
			},
		},
		Stripe: stubStripeService{
			enableFn: func(context.Context, tenant.ID, string, string) (uuid.UUID, error) {
				return uuid.Nil, stripeconnector.ErrRouteConflict
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusConflict; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestStripeWebhookUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{"type":"payment_intent.succeeded","account":"acct_123"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Stripe: stubStripeService{
			webhookFn: func(context.Context, []byte, http.Header) error {
				return stripeconnector.ErrUnauthorized
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestSlackWebhookChallenge(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack/events", strings.NewReader(`{"type":"url_verification","challenge":"abc123"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Slack: stubSlackService{
			handleEventsFn: func(_ context.Context, rawBody []byte, _ http.Header) (slackconnector.Result, error) {
				if !strings.Contains(string(rawBody), `"challenge":"abc123"`) {
					t.Fatalf("rawBody = %s", rawBody)
				}
				return slackconnector.Result{IsChallenge: true, Challenge: "abc123"}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "{\"challenge\":\"abc123\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestSlackWebhookOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack/events", strings.NewReader(`{"team_id":"T123","event":{"type":"app_mention"}}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Slack: stubSlackService{
			handleEventsFn: func(_ context.Context, rawBody []byte, _ http.Header) (slackconnector.Result, error) {
				if !strings.Contains(string(rawBody), `"team_id":"T123"`) {
					t.Fatalf("rawBody = %s", rawBody)
				}
				return slackconnector.Result{}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "{\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestListSchemas(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/schemas", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Schemas: stubSchemaService{
			listFn: func(context.Context) ([]schemapkg.Schema, error) {
				return []schemapkg.Schema{{FullName: "resend.email.received.v1", EventType: "resend.email.received", Version: 1, Source: "resend"}}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"full_name":"resend.email.received.v1"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGetSchema(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/schemas/resend.email.received.v1", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Schemas: stubSchemaService{
			getFn: func(context.Context, string) (schemapkg.Schema, error) {
				return schemapkg.Schema{FullName: "resend.email.received.v1", SchemaJSON: json.RawMessage(`{"type":"object"}`)}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"schema":{"type":"object"}`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestListProviders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{
		Logger: testLogger(),
		ProviderCatalog: stubProviderCatalogService{
			listFn: func(context.Context) ([]catalog.ProviderSummary, error) {
				return []catalog.ProviderSummary{{
					Name:                "slack",
					SupportsTenantScope: true,
					SupportsGlobalScope: true,
					HasInbound:          true,
					OperationCount:      2,
					SchemaCount:         7,
				}}, nil
			},
		},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"name":"slack"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGetProviderConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/providers/slack/config", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{
		Logger: testLogger(),
		ProviderCatalog: stubProviderCatalogService{
			getConfigFn: func(context.Context, string) (catalog.ConfigCatalog, error) {
				return catalog.ConfigCatalog{
					Fields: []catalog.ConfigFieldCatalog{{
						Name:     "bot_token",
						Required: true,
						Secret:   true,
					}},
				}, nil
			},
		},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if strings.Contains(rec.Body.String(), `"config"`) {
		t.Fatalf("unexpected wrapper body = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"secret":true`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGetProviderNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/providers/nope", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{
		Logger: testLogger(),
		ProviderCatalog: stubProviderCatalogService{
			getFn: func(context.Context, string) (catalog.ProviderDetail, error) {
				return catalog.ProviderDetail{}, sql.ErrNoRows
			},
		},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestCreateAPIKey(t *testing.T) {
	tenantID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api-keys", strings.NewReader(`{"name":"ci-bot"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Auth: stubAuthService{
			principal: iauth.Principal{TenantID: tenantID},
		},
		APIKeys: stubAPIKeyService{
			createFn: func(_ context.Context, gotTenantID tenant.ID, name string) (apikey.CreatedAPIKey, error) {
				if gotTenantID != tenantID {
					t.Fatalf("tenantID = %s, want %s", gotTenantID, tenantID)
				}
				if name != "ci-bot" {
					t.Fatalf("name = %q, want ci-bot", name)
				}
				return apikey.CreatedAPIKey{
					APIKey: apikey.APIKey{
						ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
						Name:      "ci-bot",
						KeyPrefix: "ab12cd34",
					},
					Secret: "groot_ab12cd34_secret",
				}, nil
			},
			listFn: func(context.Context, tenant.ID) ([]apikey.APIKey, error) { return nil, nil },
			revokeFn: func(context.Context, tenant.ID, uuid.UUID) (apikey.APIKey, error) {
				return apikey.APIKey{}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"api_key":"groot_ab12cd34_secret"`) {
		t.Fatalf("body = %q", body)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	tenantID := uuid.New()
	keyID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	req := httptest.NewRequest(http.MethodPost, "/api-keys/"+keyID.String()+"/revoke", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Auth: stubAuthService{
			principal: iauth.Principal{TenantID: tenantID},
		},
		APIKeys: stubAPIKeyService{
			createFn: func(context.Context, tenant.ID, string) (apikey.CreatedAPIKey, error) {
				return apikey.CreatedAPIKey{}, nil
			},
			listFn: func(context.Context, tenant.ID) ([]apikey.APIKey, error) { return nil, nil },
			revokeFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (apikey.APIKey, error) {
				if gotTenantID != tenantID {
					t.Fatalf("tenantID = %s, want %s", gotTenantID, tenantID)
				}
				if gotID != keyID {
					t.Fatalf("keyID = %s, want %s", gotID, keyID)
				}
				return apikey.APIKey{ID: keyID, Name: "ci-bot", KeyPrefix: "ab12cd34", IsActive: false}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"is_active":false`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAdminRoutesDisabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{Logger: testLogger()})
	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestAdminAuthRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:       testLogger(),
		AdminEnabled: true,
		AdminAuth: stubAuthService{
			err: errors.New("unauthorized"),
		},
	})
	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestAdminCreateTenantAudits(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", strings.NewReader(`{"name":"Acme"}`))
	rec := httptest.NewRecorder()
	tenantID := uuid.New()
	audited := false

	handler := NewHandler(Options{
		Logger:       testLogger(),
		AdminEnabled: true,
		AdminAuth:    stubAuthService{principal: iauth.Principal{Actor: iauth.Actor{Type: "operator", ID: "admin"}}},
		Tenants: stubTenantService{
			createFn: func(context.Context, string) (tenant.CreatedTenant, error) {
				return tenant.CreatedTenant{
					Tenant: tenant.Tenant{ID: tenantID, Name: "Acme"},
					APIKey: "legacy-key",
				}, nil
			},
		},
		Audit: stubAuditService{
			auditForTenantFn: func(_ context.Context, gotTenantID tenant.ID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
				audited = true
				if gotTenantID != tenantID || action != "admin.tenant.create" || resourceType != "tenant" || resourceID == nil || *resourceID != tenantID {
					t.Fatal("unexpected audit payload")
				}
				if metadata["name"] != "Acme" {
					t.Fatalf("metadata[name] = %v", metadata["name"])
				}
				return nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !audited {
		t.Fatal("expected audit call")
	}
}

func TestAdminCreateTenantAPIKey(t *testing.T) {
	tenantID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants/"+tenantID.String()+"/api-keys", strings.NewReader(`{"name":"backend"}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:       testLogger(),
		AdminEnabled: true,
		AdminAuth:    stubAuthService{principal: iauth.Principal{Actor: iauth.Actor{Type: "operator", ID: "admin"}}},
		APIKeys: stubAPIKeyService{
			createFn: func(_ context.Context, gotTenantID tenant.ID, name string) (apikey.CreatedAPIKey, error) {
				if gotTenantID != tenantID || name != "backend" {
					t.Fatal("unexpected create args")
				}
				return apikey.CreatedAPIKey{
					APIKey: apikey.APIKey{ID: uuid.New(), Name: "backend", KeyPrefix: "ab12cd34"},
					Secret: "groot_ab12cd34_secret",
				}, nil
			},
			listFn: func(context.Context, tenant.ID) ([]apikey.APIKey, error) { return nil, nil },
			revokeFn: func(context.Context, tenant.ID, uuid.UUID) (apikey.APIKey, error) {
				return apikey.APIKey{}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"api_key":"groot_ab12cd34_secret"`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAdminReplayLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", strings.NewReader(`{"tenant_id":"11111111-1111-1111-1111-111111111111","from":"2026-03-01T00:00:00Z","to":"2026-03-02T00:00:00Z","limit":101}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:               testLogger(),
		AdminEnabled:         true,
		AdminAuth:            stubAuthService{principal: iauth.Principal{Actor: iauth.Actor{Type: "operator", ID: "admin"}}},
		AdminReplayEnabled:   true,
		AdminReplayMaxEvents: 100,
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestAdminTopology(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/topology?include_global=false&event_type_prefix=llm.", nil)
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:       testLogger(),
		AdminEnabled: true,
		AdminAuth:    stubAuthService{principal: iauth.Principal{Actor: iauth.Actor{Type: "operator", ID: "admin"}}},
		Graph: stubGraphService{
			topologyFn: func(_ context.Context, req graph.TopologyRequest) (graph.Topology, error) {
				if req.IncludeGlobal {
					t.Fatal("expected include_global=false")
				}
				if req.EventTypePrefix != "llm." {
					t.Fatalf("event_type_prefix = %q", req.EventTypePrefix)
				}
				return graph.Topology{
					Nodes: []graph.Node{{ID: "eventtype:llm.generate.completed.v1", Type: "event_type", Label: "llm.generate.completed.v1"}},
					Edges: []graph.Edge{},
					Summary: graph.TopologySummary{
						Status:     "ok",
						NodesTotal: 1,
						EdgesTotal: 0,
					},
				}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), `"nodes_total":1`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAdminExecutionGraphTooLarge(t *testing.T) {
	eventID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	req := httptest.NewRequest(http.MethodGet, "/admin/events/"+eventID.String()+"/execution-graph", nil)
	req.SetPathValue("event_id", eventID.String())
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:       testLogger(),
		AdminEnabled: true,
		AdminAuth:    stubAuthService{principal: iauth.Principal{Actor: iauth.Actor{Type: "operator", ID: "admin"}}},
		Graph: stubGraphService{
			executionFn: func(context.Context, uuid.UUID, graph.ExecutionRequest) (graph.ExecutionGraph, error) {
				return graph.ExecutionGraph{}, graph.ErrGraphTooLarge
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if !strings.Contains(rec.Body.String(), "graph_too_large") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestInternalAgentRuntimeRouteRequiresAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/internal/agent-runtime/tool-calls", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger:                   testLogger(),
		AgentRuntimeSharedSecret: "runtime-secret",
		AgentTools: stubAgentToolService{
			executeFn: func(context.Context, agent.ToolExecutionRequest) (agent.ToolExecutionResult, error) {
				t.Fatal("unexpected tool execution")
				return agent.ToolExecutionResult{}, nil
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()

	NewHandler(Options{Logger: testLogger()}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
