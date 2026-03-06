package connectorinstance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const (
	ConnectorNameSlack  = "slack"
	ConnectorNameResend = "resend"
	ConnectorNameStripe = "stripe"
	ConnectorNameNotion = "notion"
	ConnectorNameLLM    = "llm"
	ScopeTenant         = "tenant"
	ScopeGlobal         = "global"
)

var GlobalTenantID = uuid.Nil

type Instance struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"-"`
	OwnerTenantID *uuid.UUID      `json:"-"`
	ConnectorName string          `json:"connector_name"`
	Scope         string          `json:"scope"`
	Status        string          `json:"status"`
	Config        json.RawMessage `json:"-"`
	CreatedAt     time.Time       `json:"-"`
	UpdatedAt     time.Time       `json:"-"`
}

type Record struct {
	ID            uuid.UUID
	TenantID      tenant.ID
	OwnerTenantID *tenant.ID
	ConnectorName string
	Scope         string
	Status        string
	Config        json.RawMessage
	CreatedAt     time.Time
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
	DefaultProvider string                       `json:"default_provider"`
	Providers       map[string]LLMProviderConfig `json:"providers"`
}

type LLMProviderConfig struct {
	APIKey string `json:"api_key"`
}

var (
	ErrUnsupportedConnector = errors.New("connector_name must be slack, resend, stripe, notion, or llm")
	ErrDuplicateInstance    = errors.New("connector instance already exists")
	ErrNotFound             = errors.New("connector instance not found")
	ErrInvalidConfig        = errors.New("invalid connector config")
	ErrMissingBotToken      = errors.New("config.bot_token is required")
	ErrMissingWebhookSecret = errors.New("config.webhook_secret is required")
	ErrMissingStripeAccount = errors.New("config.stripe_account_id is required")
	ErrMissingNotionToken   = errors.New("config.integration_token is required")
	ErrInvalidScope         = errors.New("scope must be tenant or global")
	ErrGlobalNotAllowed     = errors.New("global connector instances are disabled")
	ErrTenantOnlyConnector  = errors.New("connector only supports tenant scope")
	ErrGlobalOnlyConnector  = errors.New("connector only supports global scope")
	ErrMissingLLMProviders  = errors.New("config.providers is required")
	ErrInvalidLLMProvider   = errors.New("config.default_provider must exist in providers")
	ErrMissingLLMAPIKey     = errors.New("config.providers.<provider>.api_key is required")
	ErrImmutableName        = errors.New("connector_name cannot change")
	ErrImmutableScope       = errors.New("scope cannot change")
)

type Store interface {
	CreateConnectorInstance(context.Context, Record) (Instance, error)
	ListConnectorInstances(context.Context, tenant.ID) ([]Instance, error)
	ListAllConnectorInstances(context.Context) ([]Instance, error)
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (Instance, error)
	GetConnectorInstanceByID(context.Context, uuid.UUID) (Instance, error)
	UpdateConnectorInstanceByID(context.Context, uuid.UUID, json.RawMessage) (Instance, error)
	ListConnectorInstancesAdmin(context.Context, *tenant.ID, string, string) ([]Instance, error)
}

type Service struct {
	store                Store
	allowGlobalInstances bool
	llmDefaultProvider   string
	now                  func() time.Time
}

func NewService(store Store, allowGlobalInstances bool, llmDefaultProvider ...string) *Service {
	defaultProvider := ""
	if len(llmDefaultProvider) > 0 {
		defaultProvider = strings.TrimSpace(llmDefaultProvider[0])
	}
	return &Service{
		store:                store,
		allowGlobalInstances: allowGlobalInstances,
		llmDefaultProvider:   defaultProvider,
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID *tenant.ID, connectorName string, scope string, config json.RawMessage) (Instance, error) {
	normalizedName := strings.TrimSpace(connectorName)
	if !isSupportedConnector(normalizedName) {
		return Instance{}, ErrUnsupportedConnector
	}
	normalizedScope, tenantValue, ownerTenantID, err := normalizeScope(scope, tenantID)
	if err != nil {
		return Instance{}, err
	}
	if requiresTenantScope(normalizedName) && normalizedScope != ScopeTenant {
		return Instance{}, ErrTenantOnlyConnector
	}
	if normalizedScope == ScopeGlobal && normalizedName != ConnectorNameLLM && !s.allowGlobalInstances {
		return Instance{}, ErrGlobalNotAllowed
	}
	if requiresGlobalScope(normalizedName) && normalizedScope != ScopeGlobal {
		return Instance{}, ErrGlobalOnlyConnector
	}

	normalizedConfig, err := s.validateConfig(normalizedName, config)
	if err != nil {
		return Instance{}, err
	}

	instance, err := s.store.CreateConnectorInstance(ctx, Record{
		ID:            uuid.New(),
		TenantID:      tenantValue,
		OwnerTenantID: ownerTenantID,
		ConnectorName: normalizedName,
		Scope:         normalizedScope,
		Status:        "enabled",
		Config:        normalizedConfig,
		CreatedAt:     s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateInstance) {
			return Instance{}, ErrDuplicateInstance
		}
		return Instance{}, fmt.Errorf("create connector instance: %w", err)
	}
	return instance, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	instances, err := s.store.ListConnectorInstances(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list connector instances: %w", err)
	}
	return instances, nil
}

func (s *Service) ListAll(ctx context.Context) ([]Instance, error) {
	instances, err := s.store.ListAllConnectorInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all connector instances: %w", err)
	}
	return instances, nil
}

func (s *Service) AdminList(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]Instance, error) {
	instances, err := s.store.ListConnectorInstancesAdmin(ctx, tenantID, strings.TrimSpace(connectorName), strings.TrimSpace(scope))
	if err != nil {
		return nil, fmt.Errorf("list admin connector instances: %w", err)
	}
	return instances, nil
}

func (s *Service) AdminUpsert(ctx context.Context, id uuid.UUID, tenantID *tenant.ID, connectorName, scope string, config json.RawMessage) (Instance, error) {
	current, err := s.store.GetConnectorInstanceByID(ctx, id)
	switch {
	case err == nil:
		if strings.TrimSpace(connectorName) != "" && strings.TrimSpace(connectorName) != current.ConnectorName {
			return Instance{}, ErrImmutableName
		}
		if strings.TrimSpace(scope) != "" && strings.TrimSpace(scope) != current.Scope {
			return Instance{}, ErrImmutableScope
		}
		normalizedConfig, err := s.validateConfig(current.ConnectorName, config)
		if err != nil {
			return Instance{}, err
		}
		updated, err := s.store.UpdateConnectorInstanceByID(ctx, id, normalizedConfig)
		if err != nil {
			return Instance{}, fmt.Errorf("update connector instance: %w", err)
		}
		return updated, nil
	case errors.Is(err, ErrNotFound):
	default:
		return Instance{}, fmt.Errorf("get connector instance: %w", err)
	}

	normalizedName := strings.TrimSpace(connectorName)
	if !isSupportedConnector(normalizedName) {
		return Instance{}, ErrUnsupportedConnector
	}
	normalizedScope, tenantValue, ownerTenantID, err := normalizeScope(scope, tenantID)
	if err != nil {
		return Instance{}, err
	}
	if requiresTenantScope(normalizedName) && normalizedScope != ScopeTenant {
		return Instance{}, ErrTenantOnlyConnector
	}
	if normalizedScope == ScopeGlobal && normalizedName != ConnectorNameLLM && !s.allowGlobalInstances {
		return Instance{}, ErrGlobalNotAllowed
	}
	if requiresGlobalScope(normalizedName) && normalizedScope != ScopeGlobal {
		return Instance{}, ErrGlobalOnlyConnector
	}
	normalizedConfig, err := s.validateConfig(normalizedName, config)
	if err != nil {
		return Instance{}, err
	}
	instance, err := s.store.CreateConnectorInstance(ctx, Record{
		ID:            id,
		TenantID:      tenantValue,
		OwnerTenantID: ownerTenantID,
		ConnectorName: normalizedName,
		Scope:         normalizedScope,
		Status:        "enabled",
		Config:        normalizedConfig,
		CreatedAt:     s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateInstance) {
			return Instance{}, ErrDuplicateInstance
		}
		return Instance{}, fmt.Errorf("create connector instance: %w", err)
	}
	return instance, nil
}

func (s *Service) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	instance, err := s.store.GetConnectorInstance(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Instance{}, ErrNotFound
		}
		return Instance{}, fmt.Errorf("get connector instance: %w", err)
	}
	return instance, nil
}

func (s *Service) validateConfig(connectorName string, config json.RawMessage) (json.RawMessage, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	switch connectorName {
	case ConnectorNameSlack:
		var slackCfg SlackConfig
		if err := json.Unmarshal(config, &slackCfg); err != nil {
			return nil, ErrInvalidConfig
		}
		if strings.TrimSpace(slackCfg.BotToken) == "" {
			return nil, ErrMissingBotToken
		}
		normalized, err := json.Marshal(slackCfg)
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return normalized, nil
	case ConnectorNameStripe:
		var stripeCfg StripeConfig
		if err := json.Unmarshal(config, &stripeCfg); err != nil {
			return nil, ErrInvalidConfig
		}
		stripeCfg.StripeAccountID = strings.TrimSpace(stripeCfg.StripeAccountID)
		stripeCfg.WebhookSecret = strings.TrimSpace(stripeCfg.WebhookSecret)
		if stripeCfg.StripeAccountID == "" {
			return nil, ErrMissingStripeAccount
		}
		if stripeCfg.WebhookSecret == "" {
			return nil, ErrMissingWebhookSecret
		}
		normalized, err := json.Marshal(stripeCfg)
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return normalized, nil
	case ConnectorNameNotion:
		var notionCfg NotionConfig
		if err := json.Unmarshal(config, &notionCfg); err != nil {
			return nil, ErrInvalidConfig
		}
		notionCfg.IntegrationToken = strings.TrimSpace(notionCfg.IntegrationToken)
		if notionCfg.IntegrationToken == "" {
			return nil, ErrMissingNotionToken
		}
		normalized, err := json.Marshal(notionCfg)
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return normalized, nil
	case ConnectorNameResend:
		var cfg map[string]any
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, ErrInvalidConfig
		}
		normalized, err := json.Marshal(map[string]any{})
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return normalized, nil
	case ConnectorNameLLM:
		var llmCfg LLMConfig
		if err := json.Unmarshal(config, &llmCfg); err != nil {
			return nil, ErrInvalidConfig
		}
		if llmCfg.Providers == nil {
			llmCfg.Providers = map[string]LLMProviderConfig{}
		}
		if strings.TrimSpace(llmCfg.DefaultProvider) == "" {
			llmCfg.DefaultProvider = s.llmDefaultProvider
		}
		llmCfg.DefaultProvider = strings.TrimSpace(llmCfg.DefaultProvider)
		if len(llmCfg.Providers) == 0 {
			return nil, ErrMissingLLMProviders
		}
		for name, provider := range llmCfg.Providers {
			if !isSupportedLLMProvider(name) {
				return nil, ErrInvalidConfig
			}
			provider.APIKey = strings.TrimSpace(provider.APIKey)
			if provider.APIKey == "" {
				return nil, ErrMissingLLMAPIKey
			}
			llmCfg.Providers[name] = provider
		}
		if _, ok := llmCfg.Providers[llmCfg.DefaultProvider]; !ok {
			return nil, ErrInvalidLLMProvider
		}
		normalized, err := json.Marshal(llmCfg)
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return normalized, nil
	default:
		return nil, ErrUnsupportedConnector
	}
}

func isSupportedConnector(name string) bool {
	switch name {
	case ConnectorNameSlack, ConnectorNameResend, ConnectorNameStripe, ConnectorNameNotion, ConnectorNameLLM:
		return true
	default:
		return false
	}
}

func requiresTenantScope(name string) bool {
	switch name {
	case ConnectorNameStripe, ConnectorNameNotion:
		return true
	default:
		return false
	}
}

func requiresGlobalScope(name string) bool {
	return name == ConnectorNameLLM || name == ConnectorNameResend
}

func isSupportedLLMProvider(name string) bool {
	switch strings.TrimSpace(name) {
	case "openai", "anthropic":
		return true
	default:
		return false
	}
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
