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
	createFn func(context.Context, Record) (Instance, error)
	listFn   func(context.Context, tenant.ID) ([]Instance, error)
	getFn    func(context.Context, tenant.ID, uuid.UUID) (Instance, error)
}

func (s stubStore) CreateConnectorInstance(ctx context.Context, record Record) (Instance, error) {
	return s.createFn(ctx, record)
}
func (s stubStore) ListConnectorInstances(ctx context.Context, tenantID tenant.ID) ([]Instance, error) {
	return s.listFn(ctx, tenantID)
}
func (s stubStore) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

func TestCreateRequiresSlackBotToken(t *testing.T) {
	svc := NewService(stubStore{})
	_, err := svc.Create(context.Background(), tenant.ID{}, ConnectorNameSlack, json.RawMessage(`{}`))
	if !errors.Is(err, ErrMissingBotToken) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsUnsupportedConnector(t *testing.T) {
	svc := NewService(stubStore{})
	_, err := svc.Create(context.Background(), tenant.ID{}, "resend", json.RawMessage(`{}`))
	if !errors.Is(err, ErrUnsupportedConnector) {
		t.Fatalf("Create() error = %v", err)
	}
}
