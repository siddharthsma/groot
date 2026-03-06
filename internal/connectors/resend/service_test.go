package resend

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	svix "github.com/svix/svix-webhooks/go"

	"groot/internal/config"
	"groot/internal/ingest"
	"groot/internal/stream"
	"groot/internal/tenant"
)

type stubStore struct {
	ensureConnectorInstanceFn func(context.Context, tenant.ID, string, time.Time) error
	getResendRouteByTenantFn  func(context.Context, tenant.ID) (string, error)
	createResendRouteFn       func(context.Context, tenant.ID, string, time.Time) error
	getResendRouteTenantFn    func(context.Context, string) (tenant.ID, error)
	getSystemSettingFn        func(context.Context, string) (string, error)
	upsertSystemSettingFn     func(context.Context, string, string) error
}

func (s stubStore) EnsureConnectorInstance(ctx context.Context, tenantID tenant.ID, connectorName string, createdAt time.Time) error {
	return s.ensureConnectorInstanceFn(ctx, tenantID, connectorName, createdAt)
}
func (s stubStore) GetResendRouteByTenant(ctx context.Context, tenantID tenant.ID) (string, error) {
	return s.getResendRouteByTenantFn(ctx, tenantID)
}
func (s stubStore) CreateResendRoute(ctx context.Context, tenantID tenant.ID, token string, createdAt time.Time) error {
	return s.createResendRouteFn(ctx, tenantID, token, createdAt)
}
func (s stubStore) GetResendRouteTenant(ctx context.Context, token string) (tenant.ID, error) {
	return s.getResendRouteTenantFn(ctx, token)
}
func (s stubStore) GetSystemSetting(ctx context.Context, key string) (string, error) {
	return s.getSystemSettingFn(ctx, key)
}
func (s stubStore) UpsertSystemSetting(ctx context.Context, key, value string) error {
	return s.upsertSystemSettingFn(ctx, key, value)
}

type stubIngestor struct {
	ingestFn func(context.Context, ingest.Request) (stream.Event, error)
}

func (s stubIngestor) Ingest(ctx context.Context, req ingest.Request) (stream.Event, error) {
	return s.ingestFn(ctx, req)
}

func TestEnableReturnsExistingRoute(t *testing.T) {
	var tenantID tenant.ID
	svc := NewService(config.ResendConfig{ReceivingDomain: "example.resend.app"}, stubStore{
		ensureConnectorInstanceFn: func(context.Context, tenant.ID, string, time.Time) error { return nil },
		getResendRouteByTenantFn:  func(context.Context, tenant.ID) (string, error) { return "abc123", nil },
		createResendRouteFn: func(context.Context, tenant.ID, string, time.Time) error {
			t.Fatal("should not create route")
			return nil
		},
	}, nil, slog.Default(), nil, nil)

	result, err := svc.Enable(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if got, want := result.Address, "inbound+abc123@example.resend.app"; got != want {
		t.Fatalf("address = %q, want %q", got, want)
	}
}

func TestHandleWebhookPublishesEvent(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"data":{"to":["inbound+token123@example.resend.app"]}}`)

	webhook, err := svix.NewWebhook(secret)
	if err != nil {
		t.Fatalf("NewWebhook() error = %v", err)
	}
	ts := time.Now().UTC()
	signature, err := webhook.Sign("msg_123", ts, payload)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	headers := http.Header{
		"Svix-Id":        []string{"msg_123"},
		"Svix-Timestamp": []string{fmt.Sprintf("%d", ts.Unix())},
		"Svix-Signature": []string{signature},
	}

	tenantID := tenant.ID{}
	called := false
	svc := NewService(config.ResendConfig{}, stubStore{
		getSystemSettingFn: func(_ context.Context, key string) (string, error) {
			if key != systemSettingSigningSecret {
				t.Fatalf("unexpected key %q", key)
			}
			return secret, nil
		},
		getResendRouteTenantFn: func(_ context.Context, token string) (tenant.ID, error) {
			if token != "token123" {
				t.Fatalf("token = %q", token)
			}
			return tenantID, nil
		},
	}, stubIngestor{
		ingestFn: func(_ context.Context, req ingest.Request) (stream.Event, error) {
			called = true
			if req.Type != EventTypeEmailReceived || req.Source != EventSourceResend {
				t.Fatal("unexpected event metadata")
			}
			var body map[string]any
			if err := json.Unmarshal(req.Payload, &body); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			return stream.Event{}, nil
		},
	}, slog.Default(), nil, nil)

	if err := svc.HandleWebhook(context.Background(), payload, headers); err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !called {
		t.Fatal("expected ingest call")
	}
}

func TestBootstrapUsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webhooks" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer re_test" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"wh_123","signing_secret":"whsec_123"}`))
	}))
	defer server.Close()

	var stored map[string]string = map[string]string{}
	svc := NewService(config.ResendConfig{
		APIKey:           "re_test",
		APIBaseURL:       server.URL,
		WebhookPublicURL: "https://example.com/webhooks/resend",
		ReceivingDomain:  "example.resend.app",
		WebhookEvents:    []string{"email.received"},
	}, stubStore{
		getSystemSettingFn: func(context.Context, string) (string, error) { return "", sql.ErrNoRows },
		upsertSystemSettingFn: func(_ context.Context, key, value string) error {
			stored[key] = value
			return nil
		},
	}, nil, slog.Default(), nil, nil)

	status, err := svc.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if status != "bootstrapped" {
		t.Fatalf("status = %q", status)
	}
	if stored[systemSettingWebhookID] != "wh_123" {
		t.Fatalf("stored webhook id = %q", stored[systemSettingWebhookID])
	}
}
