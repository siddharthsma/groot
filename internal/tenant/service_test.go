package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubStore struct {
	createFn    func(context.Context, TenantRecord) (Tenant, error)
	listFn      func(context.Context) ([]Tenant, error)
	getFn       func(context.Context, ID) (Tenant, error)
	updateFn    func(context.Context, ID, string) (Tenant, error)
	getByHashFn func(context.Context, string) (Tenant, error)
}

func (s stubStore) CreateTenant(ctx context.Context, record TenantRecord) (Tenant, error) {
	return s.createFn(ctx, record)
}

func (s stubStore) ListTenants(ctx context.Context) ([]Tenant, error) {
	return s.listFn(ctx)
}

func (s stubStore) GetTenant(ctx context.Context, id ID) (Tenant, error) {
	return s.getFn(ctx, id)
}

func (s stubStore) UpdateTenantName(ctx context.Context, id ID, name string) (Tenant, error) {
	if s.updateFn == nil {
		return Tenant{}, nil
	}
	return s.updateFn(ctx, id, name)
}

func (s stubStore) GetTenantByAPIKeyHash(ctx context.Context, hash string) (Tenant, error) {
	return s.getByHashFn(ctx, hash)
}

func TestCreateTenant(t *testing.T) {
	now := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record TenantRecord) (Tenant, error) {
			if record.Name != "example" {
				t.Fatalf("record.Name = %q", record.Name)
			}
			if record.APIKeyHash == "" {
				t.Fatal("expected API key hash")
			}
			return Tenant{ID: record.ID, Name: record.Name, CreatedAt: record.CreatedAt}, nil
		},
	})
	svc.now = func() time.Time { return now }

	created, err := svc.CreateTenant(context.Background(), " example ")
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if created.APIKey == "" {
		t.Fatal("expected API key")
	}
	if created.Tenant.Name != "example" {
		t.Fatalf("Tenant.Name = %q", created.Tenant.Name)
	}
}

func TestCreateTenantRequiresName(t *testing.T) {
	svc := NewService(stubStore{})

	_, err := svc.CreateTenant(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidTenantName) {
		t.Fatalf("CreateTenant() error = %v, want %v", err, ErrInvalidTenantName)
	}
}

func TestAuthenticate(t *testing.T) {
	id := uuid.New()
	svc := NewService(stubStore{
		getByHashFn: func(_ context.Context, hash string) (Tenant, error) {
			if hash != HashAPIKey("secret") {
				t.Fatalf("unexpected hash %q", hash)
			}
			return Tenant{ID: id, Name: "example"}, nil
		},
	})

	record, err := svc.Authenticate(context.Background(), "secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if record.ID != id {
		t.Fatalf("record.ID = %s, want %s", record.ID, id)
	}
}

func TestAuthenticateUnauthorized(t *testing.T) {
	svc := NewService(stubStore{
		getByHashFn: func(context.Context, string) (Tenant, error) {
			return Tenant{}, ErrTenantNotFound
		},
	})

	_, err := svc.Authenticate(context.Background(), "secret")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v, want %v", err, ErrUnauthorized)
	}
}
