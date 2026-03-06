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

var (
	ErrUnsupportedConnector = errors.New("connector_name must be slack")
	ErrDuplicateInstance    = errors.New("connector instance already exists")
	ErrNotFound             = errors.New("connector instance not found")
	ErrInvalidConfig        = errors.New("invalid connector config")
	ErrMissingBotToken      = errors.New("config.bot_token is required")
	ErrInvalidScope         = errors.New("scope must be tenant or global")
	ErrGlobalNotAllowed     = errors.New("global connector instances are disabled")
)

type Store interface {
	CreateConnectorInstance(context.Context, Record) (Instance, error)
	ListConnectorInstances(context.Context, tenant.ID) ([]Instance, error)
	ListAllConnectorInstances(context.Context) ([]Instance, error)
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (Instance, error)
}

type Service struct {
	store                Store
	allowGlobalInstances bool
	now                  func() time.Time
}

func NewService(store Store, allowGlobalInstances bool) *Service {
	return &Service{
		store:                store,
		allowGlobalInstances: allowGlobalInstances,
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID *tenant.ID, connectorName string, scope string, config json.RawMessage) (Instance, error) {
	normalizedName := strings.TrimSpace(connectorName)
	if normalizedName != ConnectorNameSlack {
		return Instance{}, ErrUnsupportedConnector
	}
	normalizedScope, tenantValue, ownerTenantID, err := normalizeScope(scope, tenantID)
	if err != nil {
		return Instance{}, err
	}
	if normalizedScope == ScopeGlobal && !s.allowGlobalInstances {
		return Instance{}, ErrGlobalNotAllowed
	}

	normalizedConfig, err := validateConfig(normalizedName, config)
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

func validateConfig(connectorName string, config json.RawMessage) (json.RawMessage, error) {
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
	default:
		return nil, ErrUnsupportedConnector
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
