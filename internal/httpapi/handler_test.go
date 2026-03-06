package httpapi

import (
	"bytes"
	"context"
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

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/resend"
	"groot/internal/delivery"
	"groot/internal/eventquery"
	"groot/internal/functiondestination"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/stream"
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

func (s stubTenantService) Authenticate(ctx context.Context, apiKey string) (tenant.Tenant, error) {
	return s.authenticateFn(ctx, apiKey)
}

type stubEventService struct {
	ingestFn func(context.Context, ingest.Request) (stream.Event, error)
}

func (s stubEventService) Ingest(ctx context.Context, req ingest.Request) (stream.Event, error) {
	return s.ingestFn(ctx, req)
}

type stubEventQueryService struct {
	listFn func(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]eventquery.Event, error)
}

func (s stubEventQueryService) List(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]eventquery.Event, error) {
	return s.listFn(ctx, tenantID, eventType, source, from, to, limit)
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
	createFn func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, json.RawMessage, string, *string) (subscription.Subscription, error)
	listFn   func(context.Context, tenant.ID) ([]subscription.Subscription, error)
	pauseFn  func(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
	resumeFn func(context.Context, tenant.ID, uuid.UUID) (subscription.Subscription, error)
}

func (s stubSubscriptionService) Create(ctx context.Context, tenantID tenant.ID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectorInstanceID *uuid.UUID, operation *string, operationParams json.RawMessage, eventType string, eventSource *string) (subscription.Subscription, error) {
	return s.createFn(ctx, tenantID, destinationType, connectedAppID, functionDestinationID, connectorInstanceID, operation, operationParams, eventType, eventSource)
}

func (s stubSubscriptionService) List(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubSubscriptionService) Pause(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	return s.pauseFn(ctx, tenantID, subscriptionID)
}

func (s stubSubscriptionService) Resume(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	return s.resumeFn(ctx, tenantID, subscriptionID)
}

type stubDeliveryService struct {
	listFn  func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, int) ([]delivery.Job, error)
	getFn   func(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
	retryFn func(context.Context, tenant.ID, uuid.UUID) (delivery.Job, error)
}

type stubConnectorInstanceService struct {
	createFn  func(context.Context, *tenant.ID, string, string, json.RawMessage) (connectorinstance.Instance, error)
	listFn    func(context.Context, tenant.ID) ([]connectorinstance.Instance, error)
	listAllFn func(context.Context) ([]connectorinstance.Instance, error)
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

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{"type":"example.event","source":"manual","payload":{"ok":true}}`))
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
			ingestFn: func(_ context.Context, req ingest.Request) (stream.Event, error) {
				if req.TenantID != tenantID {
					t.Fatalf("req.TenantID = %s", req.TenantID)
				}
				if string(req.Payload) != `{"ok":true}` {
					t.Fatalf("req.Payload = %s", req.Payload)
				}
				return stream.Event{EventID: eventID, TenantID: tenantID, Type: req.Type, Source: req.Source, Payload: json.RawMessage(`{"ok":true}`)}, nil
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
	req := httptest.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBufferString(`{"connected_app_id":"33333333-3333-3333-3333-333333333333","event_type":"example.event"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler := NewHandler(Options{
		Logger: testLogger(),
		Tenants: stubTenantService{
			authenticateFn: func(context.Context, string) (tenant.Tenant, error) { return tenant.Tenant{ID: tenantID}, nil },
		},
		Subs: stubSubscriptionService{
			createFn: func(context.Context, tenant.ID, string, *uuid.UUID, *uuid.UUID, *uuid.UUID, *string, json.RawMessage, string, *string) (subscription.Subscription, error) {
				return subscription.Subscription{}, subscription.ErrConnectedAppNotFound
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
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
