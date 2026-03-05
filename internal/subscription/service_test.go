package subscription

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/tenant"
)

type stubStore struct {
	createFn func(context.Context, Record) (Subscription, error)
	listFn   func(context.Context, tenant.ID) ([]Subscription, error)
	matchFn  func(context.Context, tenant.ID, string, string) ([]Subscription, error)
}

func (s stubStore) CreateSubscription(ctx context.Context, record Record) (Subscription, error) {
	return s.createFn(ctx, record)
}

func (s stubStore) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]Subscription, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubStore) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]Subscription, error) {
	return s.matchFn(ctx, tenantID, eventType, eventSource)
}

type stubApps struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

func (s stubApps) Get(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (connectedapp.App, error) {
	return s.getFn(ctx, tenantID, appID)
}

func TestCreateRequiresEventType(t *testing.T) {
	svc := NewService(stubStore{}, stubApps{})
	_, err := svc.Create(context.Background(), tenant.ID{}, uuid.New(), " ", nil)
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("Create() error = %v", err)
	}
}
