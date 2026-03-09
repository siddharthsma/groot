package connection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"groot/internal/integrations"
	"groot/internal/integrations/registry"
	"groot/internal/tenant"
)

func init() {
	registerTestIntegration(IntegrationNameSlack, true, true)
	registerTestIntegration(IntegrationNameResend, false, true)
	registerTestIntegration(IntegrationNameStripe, true, false)
	registerTestIntegration(IntegrationNameNotion, true, false)
	registerTestIntegration(IntegrationNameLLM, false, true)
}

type testIntegration struct {
	spec       integration.IntegrationSpec
	validateFn func(map[string]any) error
}

func (p testIntegration) Spec() integration.IntegrationSpec { return p.spec }
func (p testIntegration) ValidateConfig(config map[string]any) error {
	if p.validateFn == nil {
		return nil
	}
	return p.validateFn(config)
}
func (p testIntegration) ExecuteOperation(context.Context, integration.OperationRequest) (integration.OperationResult, error) {
	return integration.OperationResult{}, nil
}

func registerTestIntegration(name string, tenantScope, globalScope bool) {
	if registry.GetIntegration(name) != nil {
		return
	}
	validate := func(map[string]any) error { return nil }
	switch name {
	case IntegrationNameSlack:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["bot_token"])) == "" {
				return ErrMissingBotToken
			}
			return nil
		}
	case IntegrationNameStripe:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["stripe_account_id"])) == "" {
				return ErrMissingStripeAccount
			}
			if strings.TrimSpace(testAnyString(config["webhook_secret"])) == "" {
				return ErrMissingWebhookSecret
			}
			return nil
		}
	case IntegrationNameNotion:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["integration_token"])) == "" {
				return ErrMissingNotionToken
			}
			return nil
		}
	case IntegrationNameLLM:
		validate = func(config map[string]any) error {
			integrations, ok := config["integrations"].(map[string]any)
			if !ok || len(integrations) == 0 {
				return ErrMissingLLMIntegrations
			}
			defaultIntegration := strings.TrimSpace(testAnyString(config["default_integration"]))
			if defaultIntegration == "" {
				return ErrInvalidLLMIntegration
			}
			if _, ok := integrations[defaultIntegration]; !ok {
				return ErrInvalidLLMIntegration
			}
			return nil
		}
	}
	registry.RegisterIntegration(testIntegration{
		spec: integration.IntegrationSpec{
			Name:                name,
			SupportsTenantScope: tenantScope,
			SupportsGlobalScope: globalScope,
		},
		validateFn: validate,
	})
}

type stubStore struct {
	createFn     func(context.Context, Record) (Instance, error)
	listFn       func(context.Context, tenant.ID) ([]Instance, error)
	listAllFn    func(context.Context) ([]Instance, error)
	getFn        func(context.Context, tenant.ID, uuid.UUID) (Instance, error)
	getByIDFn    func(context.Context, uuid.UUID) (Instance, error)
	updateByIDFn func(context.Context, uuid.UUID, json.RawMessage) (Instance, error)
	adminListFn  func(context.Context, *tenant.ID, string, string) ([]Instance, error)
}

func (s stubStore) CreateConnection(ctx context.Context, record Record) (Instance, error) {
	return s.createFn(ctx, record)
}
func (s stubStore) ListConnections(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubStore) ListAllConnections(ctx context.Context) ([]Instance, error) {
	return s.listAllFn(ctx)
}
func (s stubStore) GetConnection(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

func (s stubStore) GetConnectionByID(ctx context.Context, id uuid.UUID) (Instance, error) {
	if s.getByIDFn == nil {
		return Instance{}, ErrNotFound
	}
	return s.getByIDFn(ctx, id)
}

func (s stubStore) UpdateConnectionByID(ctx context.Context, id uuid.UUID, config json.RawMessage) (Instance, error) {
	if s.updateByIDFn == nil {
		return Instance{}, ErrNotFound
	}
	return s.updateByIDFn(ctx, id, config)
}

func (s stubStore) ListConnectionsAdmin(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]Instance, error) {
	if s.adminListFn == nil {
		return nil, nil
	}
	return s.adminListFn(ctx, tenantID, connectorName, scope)
}

func testAnyString(value any) string {
	text, _ := value.(string)
	return text
}

func TestCreateRequiresSlackBotToken(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, IntegrationNameSlack, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), ErrMissingBotToken.Error()) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsUnsupportedConnection(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, "unknown", ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrUnsupportedConnection) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsTenantScopedResendInstance(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, IntegrationNameResend, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrGlobalOnlyConnection) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsGlobalStripeInstance(t *testing.T) {
	svc := NewService(stubStore{}, true)
	_, err := svc.Create(context.Background(), nil, IntegrationNameStripe, ScopeGlobal, json.RawMessage(`{"stripe_account_id":"acct_123","webhook_secret":"whsec_123"}`))
	if !errors.Is(err, ErrTenantOnlyConnection) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRequiresNotionToken(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, IntegrationNameNotion, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), ErrMissingNotionToken.Error()) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsTenantScopedLLMInstance(t *testing.T) {
	svc := NewService(stubStore{}, true, "openai")
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, IntegrationNameLLM, ScopeTenant, json.RawMessage(`{"integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`))
	if !errors.Is(err, ErrGlobalOnlyConnection) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateLLMUsesConfiguredDefaultIntegration(t *testing.T) {
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Instance, error) {
			var cfg LLMConfig
			if err := json.Unmarshal(record.Config, &cfg); err != nil {
				t.Fatalf("Unmarshal(config) error = %v", err)
			}
			if got, want := cfg.DefaultIntegration, "openai"; got != want {
				t.Fatalf("DefaultIntegration = %q, want %q", got, want)
			}
			return Instance{ID: record.ID, Scope: record.Scope}, nil
		},
	}, true, "openai")

	_, err := svc.Create(context.Background(), nil, IntegrationNameLLM, ScopeGlobal, json.RawMessage(`{"integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateGlobalInstanceUsesGlobalTenantID(t *testing.T) {
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Instance, error) {
			if record.Scope != ScopeGlobal {
				t.Fatalf("Scope = %q", record.Scope)
			}
			if uuid.UUID(record.TenantID) != GlobalTenantID {
				t.Fatalf("TenantID = %s", record.TenantID)
			}
			if record.OwnerTenantID != nil {
				t.Fatal("OwnerTenantID should be nil")
			}
			return Instance{ID: record.ID, Scope: record.Scope, TenantID: uuid.UUID(record.TenantID)}, nil
		},
	}, true)

	instance, err := svc.Create(context.Background(), nil, IntegrationNameSlack, ScopeGlobal, json.RawMessage(`{"bot_token":"xoxb-test"}`))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if instance.Scope != ScopeGlobal {
		t.Fatalf("instance.Scope = %q", instance.Scope)
	}
}
