package functiondestination

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

type stubStore struct {
	createFn func(context.Context, Record) (Destination, error)
	listFn   func(context.Context, tenant.ID) ([]Destination, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (Destination, error)
	deleteFn func(context.Context, tenant.ID, uuid.UUID) error
}

func (s stubStore) CreateFunctionDestination(ctx context.Context, record Record) (Destination, error) {
	return s.createFn(ctx, record)
}
func (s stubStore) ListFunctionDestinations(ctx context.Context, tenantID tenant.ID) ([]Destination, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubStore) GetFunctionDestination(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Destination, error) {
	return s.getFn(ctx, tenantID, id)
}
func (s stubStore) DeleteFunctionDestination(ctx context.Context, tenantID tenant.ID, id uuid.UUID) error {
	return s.deleteFn(ctx, tenantID, id)
}

func TestCreateValidatesURL(t *testing.T) {
	svc := NewService(stubStore{})
	if _, err := svc.Create(context.Background(), tenant.ID{}, "fn", "http://example.com"); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateAllowsLocalHTTP(t *testing.T) {
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Destination, error) {
			if record.URL != "http://127.0.0.1:8080/fn" {
				t.Fatalf("record.URL = %q", record.URL)
			}
			if record.Secret == "" {
				t.Fatal("expected secret")
			}
			return Destination{ID: record.ID, Name: record.Name, URL: record.URL, TimeoutSeconds: record.TimeoutSeconds}, nil
		},
	})

	created, err := svc.Create(context.Background(), tenant.ID{}, "fn", "http://127.0.0.1:8080/fn")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Secret == "" {
		t.Fatal("expected returned secret")
	}
}
