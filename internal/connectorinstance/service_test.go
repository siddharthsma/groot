package connectorinstance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

type stubStore struct {
	createFn  func(context.Context, Record) (Instance, error)
	listFn    func(context.Context, tenant.ID) ([]Instance, error)
	listAllFn func(context.Context) ([]Instance, error)
	getFn     func(context.Context, tenant.ID, uuid.UUID) (Instance, error)
}

func (s stubStore) CreateConnectorInstance(ctx context.Context, record Record) (Instance, error) {
	return s.createFn(ctx, record)
}
func (s stubStore) ListConnectorInstances(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubStore) ListAllConnectorInstances(ctx context.Context) ([]Instance, error) {
	return s.listAllFn(ctx)
}
func (s stubStore) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

func TestCreateRequiresSlackBotToken(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, ConnectorNameSlack, ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrMissingBotToken) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsUnsupportedConnector(t *testing.T) {
	svc := NewService(stubStore{}, true)
	tenantID := tenant.ID(uuid.New())
	_, err := svc.Create(context.Background(), &tenantID, "resend", ScopeTenant, json.RawMessage(`{}`))
	if !errors.Is(err, ErrUnsupportedConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateGlobalInstanceUsesGlobalTenantID(t *testing.T) {
	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Instance, error) {
			if record.Scope != ScopeGlobal {
				t.Fatalf("Scope = %q", record.Scope)
			}
			if uuid.UUID(record.TenantID) != GlobalTenantID {
				t.Fatalf("TenantID = %s", record.TenantID)
			}
			if record.OwnerTenantID != nil {
				t.Fatal("OwnerTenantID should be nil")
			}
			return Instance{ID: record.ID, Scope: record.Scope, TenantID: uuid.UUID(record.TenantID)}, nil
		},
	}, true)

	instance, err := svc.Create(context.Background(), nil, ConnectorNameSlack, ScopeGlobal, json.RawMessage(`{"bot_token":"xoxb-test"}`))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if instance.Scope != ScopeGlobal {
		t.Fatalf("instance.Scope = %q", instance.Scope)
	}
}
