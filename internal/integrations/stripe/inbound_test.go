package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/config"
	"groot/internal/connection"
	"groot/internal/event"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/tenant"
)

type stubStore struct {
	getByNameFn           func(context.Context, tenant.ID, string) (connection.Instance, error)
	createConnectorFn     func(context.Context, connection.Record) (connection.Instance, error)
	updateConfigFn        func(context.Context, tenant.ID, string, json.RawMessage) (connection.Instance, error)
	getRouteByTenantFn    func(context.Context, string, tenant.ID) (inboundroute.Route, error)
	createRouteFn         func(context.Context, inboundroute.Record) (inboundroute.Route, error)
	updateRouteByTenantFn func(context.Context, string, tenant.ID, string, *uuid.UUID) (inboundroute.Route, error)
	getRouteFn            func(context.Context, string, string) (inboundroute.Route, error)
	getConnectorFn        func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
}

func (s stubStore) GetTenantConnectionByName(ctx context.Context, tenantID tenant.ID, connectorName string) (connection.Instance, error) {
	return s.getByNameFn(ctx, tenantID, connectorName)
}
func (s stubStore) CreateConnection(ctx context.Context, record connection.Record) (connection.Instance, error) {
	return s.createConnectorFn(ctx, record)
}
func (s stubStore) UpdateConnectionConfig(ctx context.Context, tenantID tenant.ID, connectorName string, config json.RawMessage) (connection.Instance, error) {
	return s.updateConfigFn(ctx, tenantID, connectorName, config)
}
func (s stubStore) GetInboundRouteByTenant(ctx context.Context, connectorName string, tenantID tenant.ID) (inboundroute.Route, error) {
	return s.getRouteByTenantFn(ctx, connectorName, tenantID)
}
func (s stubStore) CreateInboundRoute(ctx context.Context, record inboundroute.Record) (inboundroute.Route, error) {
	return s.createRouteFn(ctx, record)
}
func (s stubStore) UpdateInboundRouteByTenant(ctx context.Context, connectorName string, tenantID tenant.ID, routeKey string, connectorInstanceID *uuid.UUID) (inboundroute.Route, error) {
	return s.updateRouteByTenantFn(ctx, connectorName, tenantID, routeKey, connectorInstanceID)
}
func (s stubStore) GetInboundRoute(ctx context.Context, connectorName, routeKey string) (inboundroute.Route, error) {
	return s.getRouteFn(ctx, connectorName, routeKey)
}
func (s stubStore) GetConnection(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (connection.Instance, error) {
	return s.getConnectorFn(ctx, tenantID, id)
}

type stubIngestor struct {
	ingestFn func(context.Context, ingest.Request) (event.Event, error)
}

func (s stubIngestor) Ingest(ctx context.Context, req ingest.Request) (event.Event, error) {
	return s.ingestFn(ctx, req)
}

func TestEnableCreatesOrUpdatesInstance(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	instanceID := uuid.New()
	routeID := uuid.New()
	svc := NewService(config.StripeConfig{WebhookToleranceSeconds: 300}, stubStore{
		getByNameFn: func(context.Context, tenant.ID, string) (connection.Instance, error) {
			return connection.Instance{}, connection.ErrNotFound
		},
		createConnectorFn: func(_ context.Context, record connection.Record) (connection.Instance, error) {
			if record.IntegrationName != IntegrationName {
				t.Fatalf("IntegrationName = %q", record.IntegrationName)
			}
			return connection.Instance{ID: instanceID}, nil
		},
		getRouteByTenantFn: func(context.Context, string, tenant.ID) (inboundroute.Route, error) {
			return inboundroute.Route{}, sql.ErrNoRows
		},
		createRouteFn: func(_ context.Context, record inboundroute.Record) (inboundroute.Route, error) {
			if record.RouteKey != "acct_123" {
				t.Fatalf("RouteKey = %q", record.RouteKey)
			}
			return inboundroute.Route{ID: routeID}, nil
		},
		updateConfigFn: func(context.Context, tenant.ID, string, json.RawMessage) (connection.Instance, error) {
			t.Fatal("unexpected update")
			return connection.Instance{}, nil
		},
		updateRouteByTenantFn: func(context.Context, string, tenant.ID, string, *uuid.UUID) (inboundroute.Route, error) {
			t.Fatal("unexpected route update")
			return inboundroute.Route{}, nil
		},
	}, stubIngestor{}, nil, nil)
	id, err := svc.Enable(context.Background(), tenantID, "acct_123", "whsec_123")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if id != instanceID {
		t.Fatalf("Enable() id = %s, want %s", id, instanceID)
	}
}

func TestHandleWebhookUnauthorized(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	instanceID := uuid.New()
	body := []byte(`{"type":"payment_intent.succeeded","account":"acct_123"}`)
	svc := NewService(config.StripeConfig{WebhookToleranceSeconds: 300}, stubStore{
		getRouteFn: func(context.Context, string, string) (inboundroute.Route, error) {
			return inboundroute.Route{TenantID: uuid.UUID(tenantID), ConnectionID: &instanceID}, nil
		},
		getConnectorFn: func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error) {
			return connection.Instance{Config: json.RawMessage(`{"stripe_account_id":"acct_123","webhook_secret":"whsec_test"}`)}, nil
		},
	}, stubIngestor{}, nil, nil)
	headers := http.Header{}
	headers.Set("Stripe-Signature", "t=1700000000,v1=bad")
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	err := svc.HandleWebhook(context.Background(), body, headers)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
}

func TestHandleWebhookPublishesEvent(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	instanceID := uuid.New()
	body := []byte(`{"type":"payment_intent.succeeded","account":"acct_123","data":{"object":{"id":"pi_123"}}}`)
	headers := http.Header{}
	signature := computeTestSignature("whsec_test", "1700000000", body)
	headers.Set("Stripe-Signature", "t=1700000000,v1="+signature)

	svc := NewService(config.StripeConfig{WebhookToleranceSeconds: 300}, stubStore{
		getRouteFn: func(context.Context, string, string) (inboundroute.Route, error) {
			return inboundroute.Route{TenantID: uuid.UUID(tenantID), ConnectionID: &instanceID}, nil
		},
		getConnectorFn: func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error) {
			return connection.Instance{Config: json.RawMessage(`{"stripe_account_id":"acct_123","webhook_secret":"whsec_test"}`)}, nil
		},
	}, stubIngestor{
		ingestFn: func(_ context.Context, req ingest.Request) (event.Event, error) {
			if got, want := req.Type, "stripe.payment_intent.succeeded.v1"; got != want {
				t.Fatalf("Type = %q, want %q", got, want)
			}
			if got, want := req.SourceInfo.Integration, EventSource; got != want {
				t.Fatalf("SourceInfo.Integration = %q, want %q", got, want)
			}
			if req.TenantID != tenantID {
				t.Fatalf("TenantID = %s, want %s", req.TenantID, tenantID)
			}
			return event.Event{EventID: uuid.New()}, nil
		},
	}, nil, nil)
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	if err := svc.HandleWebhook(context.Background(), body, headers); err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
}

func computeTestSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
