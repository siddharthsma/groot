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
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/ingest"
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

type stubConnectedAppService struct {
	createFn func(context.Context, tenant.ID, string, string) (connectedapp.App, error)
	listFn   func(context.Context, tenant.ID) ([]connectedapp.App, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
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
	createFn func(context.Context, tenant.ID, uuid.UUID, string, *string) (subscription.Subscription, error)
	listFn   func(context.Context, tenant.ID) ([]subscription.Subscription, error)
}

func (s stubSubscriptionService) Create(ctx context.Context, tenantID tenant.ID, connectedAppID uuid.UUID, eventType string, eventSource *string) (subscription.Subscription, error) {
	return s.createFn(ctx, tenantID, connectedAppID, eventType, eventSource)
}

func (s stubSubscriptionService) List(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	return s.listFn(ctx, tenantID)
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
			createFn: func(context.Context, tenant.ID, uuid.UUID, string, *string) (subscription.Subscription, error) {
				return subscription.Subscription{}, subscription.ErrConnectedAppNotFound
			},
		},
	})

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}
