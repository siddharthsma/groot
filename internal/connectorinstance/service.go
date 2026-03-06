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
)

type Instance struct {
	ID            uuid.UUID       `json:"id"`
	TenantID      uuid.UUID       `json:"-"`
	ConnectorName string          `json:"connector_name"`
	Status        string          `json:"status"`
	Config        json.RawMessage `json:"-"`
	CreatedAt     time.Time       `json:"-"`
}

type Record struct {
	ID            uuid.UUID
	TenantID      tenant.ID
	ConnectorName string
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
)

type Store interface {
	CreateConnectorInstance(context.Context, Record) (Instance, error)
	ListConnectorInstances(context.Context, tenant.ID) ([]Instance, error)
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (Instance, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, connectorName string, config json.RawMessage) (Instance, error) {
	normalizedName := strings.TrimSpace(connectorName)
	if normalizedName != ConnectorNameSlack {
		return Instance{}, ErrUnsupportedConnector
	}
	normalizedConfig, err := validateConfig(normalizedName, config)
	if err != nil {
		return Instance{}, err
	}

	instance, err := s.store.CreateConnectorInstance(ctx, Record{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ConnectorName: normalizedName,
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
