package connectorinstance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
	"groot/internal/tenant"
)

func init() {
	registerTestProvider(ConnectorNameSlack, true, true)
	registerTestProvider(ConnectorNameResend, false, true)
	registerTestProvider(ConnectorNameStripe, true, false)
	registerTestProvider(ConnectorNameNotion, true, false)
	registerTestProvider(ConnectorNameLLM, false, true)
}

type testProvider struct {
	spec       provider.ProviderSpec
	validateFn func(map[string]any) error
}

func (p testProvider) Spec() provider.ProviderSpec { return p.spec }
func (p testProvider) ValidateConfig(config map[string]any) error {
	if p.validateFn == nil {
		return nil
	}
	return p.validateFn(config)
}
func (p testProvider) ExecuteOperation(context.Context, provider.OperationRequest) (provider.OperationResult, error) {
	return provider.OperationResult{}, nil
}

func registerTestProvider(name string, tenantScope, globalScope bool) {
	if registry.GetProvider(name) != nil {
		return
	}
	validate := func(map[string]any) error { return nil }
	switch name {
	case ConnectorNameSlack:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["bot_token"])) == "" {
				return ErrMissingBotToken
			}
			return nil
		}
	case ConnectorNameStripe:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["stripe_account_id"])) == "" {
				return ErrMissingStripeAccount
			}
			if strings.TrimSpace(testAnyString(config["webhook_secret"])) == "" {
				return ErrMissingWebhookSecret
			}
			return nil
		}
	case ConnectorNameNotion:
		validate = func(config map[string]any) error {
			if strings.TrimSpace(testAnyString(config["integration_token"])) == "" {
				return ErrMissingNotionToken
			}
			return nil
		}
	case ConnectorNameLLM:
		validate = func(config map[string]any) error {
			providers, ok := config["providers"].(map[string]any)
			if !ok || len(providers) == 0 {
				return ErrMissingLLMProviders
			}
			defaultProvider := strings.TrimSpace(testAnyString(config["default_provider"]))
			if defaultProvider == "" {
				return ErrInvalidLLMProvider
			}
			if _, ok := providers[defaultProvider]; !ok {
				return ErrInvalidLLMProvider
			}
			return nil
		}
	}
	registry.RegisterProvider(testProvider{
		spec: provider.ProviderSpec{
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

func (s stubStore) CreateConnectorInstance(ctx context.Context, record Record) (Instance, error) {
	return s.createFn(ctx, record)
}
func (s stubStore) ListConnectorInstances(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubStore) ListAllConnectorInstances(ctx context.Context) ([]Instance, error) {
	return s.listAllFn(ctx)
}
func (s stubStore) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

func (s stubStore) GetConnectorInstanceByID(ctx context.Context, id uuid.UUID) (Instance, error) {
	if s.getByIDFn == nil {
		return Instance{}, ErrNotFound
	}
	return s.getByIDFn(ctx, id)
}

func (s stubStore) UpdateConnectorInstanceByID(ctx context.Context, id uuid.UUID, config json.RawMessage) (Instance, error) {
	if s.updateByIDFn == nil {
		return Instance{}, ErrNotFound
	}
	return s.updateByIDFn(ctx, id, config)
}

func (s stubStore) ListConnectorInstancesAdmin(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]Instance, error) {
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
	_, err := svc.Create(context.Background(), &tenantID, ConnectorNameSlack, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), ErrMissingBotToken.Error()) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsUnsupportedConnector(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, "unknown", ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrUnsupportedConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsTenantScopedResendInstance(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, ConnectorNameResend, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrGlobalOnlyConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsGlobalStripeInstance(t *testing.T) {
	svc := NewService(stubStore{}, true)
	_, err := svc.Create(context.Background(), nil, ConnectorNameStripe, ScopeGlobal, json.RawMessage(`{"stripe_account_id":"acct_123","webhook_secret":"whsec_123"}`))
	if !errors.Is(err, ErrTenantOnlyConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRequiresNotionToken(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, ConnectorNameNotion, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), ErrMissingNotionToken.Error()) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsTenantScopedLLMInstance(t *testing.T) {
	svc := NewService(stubStore{}, true, "openai")
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, ConnectorNameLLM, ScopeTenant, json.RawMessage(`{"providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`))
	if !errors.Is(err, ErrGlobalOnlyConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateLLMUsesConfiguredDefaultProvider(t *testing.T) {
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Instance, error) {
			var cfg LLMConfig
			if err := json.Unmarshal(record.Config, &cfg); err != nil {
				t.Fatalf("Unmarshal(config) error = %v", err)
			}
			if got, want := cfg.DefaultProvider, "openai"; got != want {
				t.Fatalf("DefaultProvider = %q, want %q", got, want)
			}
			return Instance{ID: record.ID, Scope: record.Scope}, nil
		},
	}, true, "openai")

	_, err := svc.Create(context.Background(), nil, ConnectorNameLLM, ScopeGlobal, json.RawMessage(`{"providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`))
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

	instance, err := svc.Create(context.Background(), nil, ConnectorNameSlack, ScopeGlobal, json.RawMessage(`{"bot_token":"xoxb-test"}`))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if instance.Scope != ScopeGlobal {
		t.Fatalf("instance.Scope = %q", instance.Scope)
	}
}
