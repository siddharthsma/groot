package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/tenant"
)

type Subscription struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"-"`
	ConnectedAppID uuid.UUID `json:"connected_app_id"`
	EventType      string    `json:"event_type"`
	EventSource    *string   `json:"event_source"`
	CreatedAt      time.Time `json:"-"`
}

type Record struct {
	ID             uuid.UUID
	TenantID       tenant.ID
	ConnectedAppID uuid.UUID
	EventType      string
	EventSource    *string
	CreatedAt      time.Time
}

var (
	ErrInvalidEventType     = errors.New("event_type is required")
	ErrConnectedAppNotFound = errors.New("connected app not found")
)

type Store interface {
	CreateSubscription(context.Context, Record) (Subscription, error)
	ListSubscriptions(context.Context, tenant.ID) ([]Subscription, error)
	ListMatchingSubscriptions(context.Context, tenant.ID, string, string) ([]Subscription, error)
}

type ConnectedAppStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type Service struct {
	store         Store
	connectedApps ConnectedAppStore
	now           func() time.Time
}

func NewService(store Store, connectedApps ConnectedAppStore) *Service {
	return &Service{
		store:         store,
		connectedApps: connectedApps,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, connectedAppID uuid.UUID, eventType string, eventSource *string) (Subscription, error) {
	trimmedType := strings.TrimSpace(eventType)
	if trimmedType == "" {
		return Subscription{}, ErrInvalidEventType
	}

	if _, err := s.connectedApps.Get(ctx, tenantID, connectedAppID); err != nil {
		if errors.Is(err, connectedapp.ErrNotFound) {
			return Subscription{}, ErrConnectedAppNotFound
		}
		return Subscription{}, fmt.Errorf("get connected app: %w", err)
	}

	record := Record{
		ID:             uuid.New(),
		TenantID:       tenantID,
		ConnectedAppID: connectedAppID,
		EventType:      trimmedType,
		EventSource:    normalizeSource(eventSource),
		CreatedAt:      s.now(),
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
