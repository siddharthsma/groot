package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

type Subscription struct {
	ID                    uuid.UUID       `json:"id"`
	TenantID              uuid.UUID       `json:"-"`
	ConnectedAppID        *uuid.UUID      `json:"connected_app_id,omitempty"`
	DestinationType       string          `json:"destination_type"`
	FunctionDestinationID *uuid.UUID      `json:"function_destination_id,omitempty"`
	ConnectorInstanceID   *uuid.UUID      `json:"connector_instance_id,omitempty"`
	Operation             *string         `json:"operation,omitempty"`
	OperationParams       json.RawMessage `json:"operation_params,omitempty"`
	EventType             string          `json:"event_type"`
	EventSource           *string         `json:"event_source"`
	Status                string          `json:"status"`
	CreatedAt             time.Time       `json:"-"`
}

type Record struct {
	ID                    uuid.UUID
	TenantID              tenant.ID
	ConnectedAppID        *uuid.UUID
	DestinationType       string
	FunctionDestinationID *uuid.UUID
	ConnectorInstanceID   *uuid.UUID
	Operation             *string
	OperationParams       json.RawMessage
	EventType             string
	EventSource           *string
	Status                string
	CreatedAt             time.Time
}

var (
	ErrInvalidEventType            = errors.New("event_type is required")
	ErrConnectedAppNotFound        = errors.New("connected app not found")
	ErrSubscriptionNotFound        = errors.New("subscription not found")
	ErrInvalidDestinationType      = errors.New("destination_type must be webhook, function, or connector")
	ErrFunctionDestinationNotFound = errors.New("function destination not found")
	ErrConnectorInstanceNotFound   = errors.New("connector instance not found")
	ErrInvalidOperation            = errors.New("operation is required for connector subscriptions")
	ErrInvalidOperationParams      = errors.New("operation_params are invalid")
)

const (
	StatusActive             = "active"
	StatusPaused             = "paused"
	DestinationTypeWebhook   = "webhook"
	DestinationTypeFunction  = "function"
	DestinationTypeConnector = "connector"
)

type Store interface {
	CreateSubscription(context.Context, Record) (Subscription, error)
	ListSubscriptions(context.Context, tenant.ID) ([]Subscription, error)
	ListMatchingSubscriptions(context.Context, tenant.ID, string, string) ([]Subscription, error)
	SetSubscriptionStatus(context.Context, tenant.ID, uuid.UUID, string) (Subscription, error)
}

type ConnectedAppStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type FunctionDestinationStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
}

type ConnectorInstanceStore interface {
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error)
}

type Service struct {
	store                Store
	connectedApps        ConnectedAppStore
	functionDestinations FunctionDestinationStore
	connectorInstances   ConnectorInstanceStore
	now                  func() time.Time
}

func NewService(store Store, connectedApps ConnectedAppStore, functionDestinations FunctionDestinationStore, connectorInstances ConnectorInstanceStore) *Service {
	return &Service{
		store:                store,
		connectedApps:        connectedApps,
		functionDestinations: functionDestinations,
		connectorInstances:   connectorInstances,
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectorInstanceID *uuid.UUID, operation *string, operationParams json.RawMessage, eventType string, eventSource *string) (Subscription, error) {
	trimmedType := strings.TrimSpace(eventType)
	if trimmedType == "" {
		return Subscription{}, ErrInvalidEventType
	}

	normalizedDestinationType := normalizeDestinationType(destinationType)
	switch normalizedDestinationType {
	case DestinationTypeWebhook:
		if connectedAppID == nil {
			return Subscription{}, ErrConnectedAppNotFound
		}
		if _, err := s.connectedApps.Get(ctx, tenantID, *connectedAppID); err != nil {
			if errors.Is(err, connectedapp.ErrNotFound) {
				return Subscription{}, ErrConnectedAppNotFound
			}
			return Subscription{}, fmt.Errorf("get connected app: %w", err)
		}
	case DestinationTypeFunction:
		if functionDestinationID == nil {
			return Subscription{}, ErrFunctionDestinationNotFound
		}
		if _, err := s.functionDestinations.Get(ctx, tenantID, *functionDestinationID); err != nil {
			if errors.Is(err, functiondestination.ErrNotFound) {
				return Subscription{}, ErrFunctionDestinationNotFound
			}
			return Subscription{}, fmt.Errorf("get function destination: %w", err)
		}
	case DestinationTypeConnector:
		if connectorInstanceID == nil {
			return Subscription{}, ErrConnectorInstanceNotFound
		}
		instance, err := s.connectorInstances.GetConnectorInstance(ctx, tenantID, *connectorInstanceID)
		if err != nil {
			if errors.Is(err, connectorinstance.ErrNotFound) {
				return Subscription{}, ErrConnectorInstanceNotFound
			}
			return Subscription{}, fmt.Errorf("get connector instance: %w", err)
		}
		normalizedOperation := normalizeOperation(operation)
		if normalizedOperation == nil {
			return Subscription{}, ErrInvalidOperation
		}
		normalizedParams, err := validateConnectorOperation(instance, *normalizedOperation, operationParams)
		if err != nil {
			return Subscription{}, err
		}
		operation = normalizedOperation
		operationParams = normalizedParams
	default:
		return Subscription{}, ErrInvalidDestinationType
	}

	record := Record{
		ID:                    uuid.New(),
		TenantID:              tenantID,
		ConnectedAppID:        connectedAppID,
		DestinationType:       normalizedDestinationType,
		FunctionDestinationID: functionDestinationID,
		ConnectorInstanceID:   connectorInstanceID,
		Operation:             normalizeOperation(operation),
		OperationParams:       operationParams,
		EventType:             trimmedType,
		EventSource:           normalizeSource(eventSource),
		Status:                StatusActive,
		CreatedAt:             s.now(),
	}

	sub, err := s.store.CreateSubscription(ctx, record)
	if err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return sub, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Subscription, error) {
	subs, err := s.store.ListSubscriptions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}

func (s *Service) Pause(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	return s.setStatus(ctx, tenantID, subscriptionID, StatusPaused)
}

func (s *Service) Resume(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	return s.setStatus(ctx, tenantID, subscriptionID, StatusActive)
}

func (s *Service) setStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (Subscription, error) {
	sub, err := s.store.SetSubscriptionStatus(ctx, tenantID, subscriptionID, status)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return Subscription{}, ErrSubscriptionNotFound
		}
		return Subscription{}, fmt.Errorf("set subscription status: %w", err)
	}
	return sub, nil
}

func normalizeSource(source *string) *string {
	if source == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*source)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeDestinationType(destinationType string) string {
	trimmed := strings.TrimSpace(destinationType)
	if trimmed == "" {
		return DestinationTypeWebhook
	}
	return trimmed
}

func normalizeOperation(operation *string) *string {
	if operation == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*operation)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

type slackOperationParams struct {
	Channel  string          `json:"channel,omitempty"`
	Text     string          `json:"text,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	ThreadTS string          `json:"thread_ts,omitempty"`
}

var placeholderPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func validateConnectorOperation(instance connectorinstance.Instance, operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	if instance.ConnectorName != connectorinstance.ConnectorNameSlack {
		return nil, ErrInvalidOperation
	}
	if operation != "post_message" {
		return nil, ErrInvalidOperation
	}
	if len(operationParams) == 0 {
		operationParams = json.RawMessage(`{}`)
	}
	var params slackOperationParams
	if err := json.Unmarshal(operationParams, &params); err != nil {
		return nil, ErrInvalidOperationParams
	}

	var cfg connectorinstance.SlackConfig
	if err := json.Unmarshal(instance.Config, &cfg); err != nil {
		return nil, ErrInvalidOperationParams
	}
	if strings.TrimSpace(params.Channel) == "" && strings.TrimSpace(cfg.DefaultChannel) == "" {
		return nil, ErrInvalidOperationParams
	}
	if strings.TrimSpace(params.Text) == "" && len(params.Blocks) == 0 {
		return nil, ErrInvalidOperationParams
	}
	if err := validateTemplate(params.Text); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(params)
	if err != nil {
		return nil, ErrInvalidOperationParams
	}
	return normalized, nil
}

func validateTemplate(text string) error {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	allowed := map[string]struct{}{
		"event_id":  {},
		"tenant_id": {},
		"type":      {},
		"source":    {},
		"timestamp": {},
	}
	for _, match := range matches {
		key := strings.TrimSpace(match[1])
		if _, ok := allowed[key]; !ok {
			return ErrInvalidOperationParams
		}
	}
	return nil
}
