package connectedapp

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

type stubStore struct {
	createFn func(context.Context, Record) (App, error)
	listFn   func(context.Context, tenant.ID) ([]App, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (App, error)
}

func (s stubStore) CreateConnectedApp(ctx context.Context, record Record) (App, error) {
	return s.createFn(ctx, record)
}

func (s stubStore) ListConnectedApps(ctx context.Context, tenantID tenant.ID) ([]App, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubStore) GetConnectedApp(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (App, error) {
	return s.getFn(ctx, tenantID, appID)
}

func TestCreateValidatesURL(t *testing.T) {
	svc := NewService(stubStore{})
	if _, err := svc.Create(context.Background(), tenant.ID{}, "app", "not-a-url"); !errors.Is(err, ErrInvalidDestinationURL) {
		t.Fatalf("Create() error = %v", err)
	}
}
