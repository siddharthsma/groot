package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/integrations/registry"
	"groot/internal/tenant"
)

const (
	IntegrationNameSlack  = "slack"
	IntegrationNameResend = "resend"
	IntegrationNameStripe = "stripe"
	IntegrationNameNotion = "notion"
	IntegrationNameLLM    = "llm"
	ScopeTenant           = "tenant"
	ScopeGlobal           = "global"
)

var GlobalTenantID = uuid.Nil

type Instance struct {
	ID              uuid.UUID       `json:"id"`
	TenantID        uuid.UUID       `json:"-"`
	OwnerTenantID   *uuid.UUID      `json:"-"`
	IntegrationName string          `json:"integration_name"`
	Scope           string          `json:"scope"`
	Status          string          `json:"status"`
	Config          json.RawMessage `json:"-"`
	CreatedAt       time.Time       `json:"-"`
	UpdatedAt       time.Time       `json:"-"`
}

type Record struct {
	ID              uuid.UUID
	TenantID        tenant.ID
	OwnerTenantID   *tenant.ID
	IntegrationName string
	Scope           string
	Status          string
	Config          json.RawMessage
	CreatedAt       time.Time
}

type SlackConfig struct {
	BotToken       string `json:"bot_token"`
	DefaultChannel string `json:"default_channel,omitempty"`
}

type StripeConfig struct {
	StripeAccountID string `json:"stripe_account_id"`
	WebhookSecret   string `json:"webhook_secret"`
}

type NotionConfig struct {
	IntegrationToken string `json:"integration_token"`
}

type LLMConfig struct {
	DefaultIntegration string                          `json:"default_integration"`
	Integrations       map[string]LLMIntegrationConfig `json:"integrations"`
}

type LLMIntegrationConfig struct {
	APIKey string `json:"api_key"`
}

var (
	ErrUnsupportedConnection  = errors.New("integration_name must be a registered integration")
	ErrDuplicateInstance      = errors.New("connection already exists")
	ErrNotFound               = errors.New("connection not found")
	ErrInvalidConfig          = errors.New("invalid connection config")
	ErrMissingBotToken        = errors.New("config.bot_token is required")
	ErrMissingWebhookSecret   = errors.New("config.webhook_secret is required")
	ErrMissingStripeAccount   = errors.New("config.stripe_account_id is required")
	ErrMissingNotionToken     = errors.New("config.integration_token is required")
	ErrInvalidScope           = errors.New("scope must be tenant or global")
	ErrGlobalNotAllowed       = errors.New("global connections are disabled")
	ErrTenantOnlyConnection   = errors.New("connection only supports tenant scope")
	ErrGlobalOnlyConnection   = errors.New("connection only supports global scope")
	ErrMissingLLMIntegrations = errors.New("config.integrations is required")
	ErrInvalidLLMIntegration  = errors.New("config.default_integration must exist in integrations")
	ErrMissingLLMAPIKey       = errors.New("config.integrations.<integration>.api_key is required")
	ErrImmutableName          = errors.New("integration_name cannot change")
	ErrImmutableScope         = errors.New("scope cannot change")
)

type Store interface {
	CreateConnection(context.Context, Record) (Instance, error)
	ListConnections(context.Context, tenant.ID) ([]Instance, error)
	ListAllConnections(context.Context) ([]Instance, error)
	GetConnection(context.Context, tenant.ID, uuid.UUID) (Instance, error)
	GetConnectionByID(context.Context, uuid.UUID) (Instance, error)
	UpdateConnectionByID(context.Context, uuid.UUID, json.RawMessage) (Instance, error)
	ListConnectionsAdmin(context.Context, *tenant.ID, string, string) ([]Instance, error)
}

type Service struct {
	store                 Store
	allowGlobalInstances  bool
	llmDefaultIntegration string
	now                   func() time.Time
}

func NewService(store Store, allowGlobalInstances bool, llmDefaultIntegration ...string) *Service {
	defaultIntegration := ""
	if len(llmDefaultIntegration) > 0 {
		defaultIntegration = strings.TrimSpace(llmDefaultIntegration[0])
	}
	return &Service{
		store:                 store,
		allowGlobalInstances:  allowGlobalInstances,
		llmDefaultIntegration: defaultIntegration,
		now:                   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID *tenant.ID, connectorName string, scope string, config json.RawMessage) (Instance, error) {
	normalizedName := strings.TrimSpace(connectorName)
	integration := registry.GetIntegration(normalizedName)
	if integration == nil {
		return Instance{}, ErrUnsupportedConnection
	}
	normalizedScope, tenantValue, ownerTenantID, err := normalizeScope(scope, tenantID)
	if err != nil {
		return Instance{}, err
	}
	spec := integration.Spec()
	if !spec.SupportsGlobalScope && normalizedScope == ScopeGlobal {
		return Instance{}, ErrTenantOnlyConnection
	}
	if !spec.SupportsTenantScope && normalizedScope == ScopeTenant {
		return Instance{}, ErrGlobalOnlyConnection
	}
	if normalizedScope == ScopeGlobal && normalizedName != IntegrationNameLLM && !s.allowGlobalInstances {
		return Instance{}, ErrGlobalNotAllowed
	}
	if normalizedName == IntegrationNameLLM && normalizedScope != ScopeGlobal {
		return Instance{}, ErrGlobalOnlyConnection
	}

	normalizedConfig, err := s.validateConfig(normalizedName, config)
	if err != nil {
		return Instance{}, err
	}

	instance, err := s.store.CreateConnection(ctx, Record{
		ID:              uuid.New(),
		TenantID:        tenantValue,
		OwnerTenantID:   ownerTenantID,
		IntegrationName: normalizedName,
		Scope:           normalizedScope,
		Status:          "enabled",
		Config:          normalizedConfig,
		CreatedAt:       s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateInstance) {
			return Instance{}, ErrDuplicateInstance
		}
		return Instance{}, fmt.Errorf("create connection: %w", err)
	}
	return instance, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	instances, err := s.store.ListConnections(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return instances, nil
}

func (s *Service) ListAll(ctx context.Context) ([]Instance, error) {
	instances, err := s.store.ListAllConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all connections: %w", err)
	}
	return instances, nil
}

func (s *Service) AdminList(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]Instance, error) {
	instances, err := s.store.ListConnectionsAdmin(ctx, tenantID, strings.TrimSpace(connectorName), strings.TrimSpace(scope))
	if err != nil {
		return nil, fmt.Errorf("list admin connections: %w", err)
	}
	return instances, nil
}

func (s *Service) AdminUpsert(ctx context.Context, id uuid.UUID, tenantID *tenant.ID, connectorName, scope string, config json.RawMessage) (Instance, error) {
	current, err := s.store.GetConnectionByID(ctx, id)
	switch {
	case err == nil:
		if strings.TrimSpace(connectorName) != "" && strings.TrimSpace(connectorName) != current.IntegrationName {
			return Instance{}, ErrImmutableName
		}
		if strings.TrimSpace(scope) != "" && strings.TrimSpace(scope) != current.Scope {
			return Instance{}, ErrImmutableScope
		}
		normalizedConfig, err := s.validateConfig(current.IntegrationName, config)
		if err != nil {
			return Instance{}, err
		}
		updated, err := s.store.UpdateConnectionByID(ctx, id, normalizedConfig)
		if err != nil {
			return Instance{}, fmt.Errorf("update connection: %w", err)
		}
		return updated, nil
	case errors.Is(err, ErrNotFound):
	default:
		return Instance{}, fmt.Errorf("get connection: %w", err)
	}

	normalizedName := strings.TrimSpace(connectorName)
	p := registry.GetIntegration(normalizedName)
	if p == nil {
		return Instance{}, ErrUnsupportedConnection
	}
	normalizedScope, tenantValue, ownerTenantID, err := normalizeScope(scope, tenantID)
	if err != nil {
		return Instance{}, err
	}
	spec := p.Spec()
	if !spec.SupportsGlobalScope && normalizedScope == ScopeGlobal {
		return Instance{}, ErrTenantOnlyConnection
	}
	if !spec.SupportsTenantScope && normalizedScope == ScopeTenant {
		return Instance{}, ErrGlobalOnlyConnection
	}
	if normalizedScope == ScopeGlobal && normalizedName != IntegrationNameLLM && !s.allowGlobalInstances {
		return Instance{}, ErrGlobalNotAllowed
	}
	normalizedConfig, err := s.validateConfig(normalizedName, config)
	if err != nil {
		return Instance{}, err
	}
	instance, err := s.store.CreateConnection(ctx, Record{
		ID:              id,
		TenantID:        tenantValue,
		OwnerTenantID:   ownerTenantID,
		IntegrationName: normalizedName,
		Scope:           normalizedScope,
		Status:          "enabled",
		Config:          normalizedConfig,
		CreatedAt:       s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateInstance) {
			return Instance{}, ErrDuplicateInstance
		}
		return Instance{}, fmt.Errorf("create connection: %w", err)
	}
	return instance, nil
}

func (s *Service) GetConnection(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	instance, err := s.store.GetConnection(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Instance{}, ErrNotFound
		}
		return Instance{}, fmt.Errorf("get connection: %w", err)
	}
	return instance, nil
}

func (s *Service) validateConfig(connectorName string, config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	var decoded map[string]any
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, ErrInvalidConfig
	}
	if connectorName == IntegrationNameLLM {
		if strings.TrimSpace(anyString(decoded["default_integration"])) == "" && strings.TrimSpace(s.llmDefaultIntegration) != "" {
			decoded["default_integration"] = s.llmDefaultIntegration
		}
	}
	integration := registry.GetIntegration(connectorName)
	if integration == nil {
		return nil, ErrUnsupportedConnection
	}
	if err := integration.ValidateConfig(decoded); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return nil, ErrInvalidConfig
	}
	return normalized, nil
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeScope(scope string, tenantID *tenant.ID) (string, tenant.ID, *tenant.ID, error) {
	normalizedScope := strings.TrimSpace(scope)
	if normalizedScope == "" {
		normalizedScope = ScopeTenant
	}
	switch normalizedScope {
	case ScopeTenant:
		if tenantID == nil {
			return "", tenant.ID{}, nil, ErrInvalidScope
		}
		tid := *tenantID
		return normalizedScope, tid, &tid, nil
	case ScopeGlobal:
		globalTenant := tenant.ID(GlobalTenantID)
		return normalizedScope, globalTenant, nil, nil
	default:
		return "", tenant.ID{}, nil, ErrInvalidScope
	}
}
