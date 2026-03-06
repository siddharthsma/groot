package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

type stubStore struct {
	createFn func(context.Context, Record) (Subscription, error)
	listFn   func(context.Context, tenant.ID) ([]Subscription, error)
	matchFn  func(context.Context, tenant.ID, string, string) ([]Subscription, error)
	setFn    func(context.Context, tenant.ID, uuid.UUID, string) (Subscription, error)
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

func (s stubStore) SetSubscriptionStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (Subscription, error) {
	return s.setFn(ctx, tenantID, subscriptionID, status)
}

type stubApps struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

func (s stubApps) Get(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (connectedapp.App, error) {
	return s.getFn(ctx, tenantID, appID)
}

type stubFunctions struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
}

func (s stubFunctions) Get(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (functiondestination.Destination, error) {
	return s.getFn(ctx, tenantID, id)
}

type stubConnectorInstances struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error)
}

func (s stubConnectorInstances) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (connectorinstance.Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

func TestCreateRequiresEventType(t *testing.T) {
	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnectorInstances{})
	appID := uuid.New()
	_, err := svc.Create(context.Background(), tenant.ID{}, DestinationTypeWebhook, &appID, nil, nil, nil, nil, " ", nil)
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestPause(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	subscriptionID := uuid.New()

	svc := NewService(stubStore{
		setFn: func(_ context.Context, gotTenantID tenant.ID, gotSubscriptionID uuid.UUID, gotStatus string) (Subscription, error) {
			if gotTenantID != tenantID || gotSubscriptionID != subscriptionID || gotStatus != StatusPaused {
				t.Fatal("unexpected pause args")
			}
			return Subscription{ID: subscriptionID, Status: StatusPaused}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnectorInstances{})

	sub, err := svc.Pause(context.Background(), tenantID, subscriptionID)
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if sub.Status != StatusPaused {
		t.Fatalf("Pause() status = %q", sub.Status)
	}
}

func TestCreateFunctionSubscription(t *testing.T) {
	functionID := uuid.New()
	tenantID := tenant.ID(uuid.New())

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			if record.DestinationType != DestinationTypeFunction {
				t.Fatalf("record.DestinationType = %q", record.DestinationType)
			}
			if record.FunctionDestinationID == nil || *record.FunctionDestinationID != functionID {
				t.Fatal("unexpected function destination id")
			}
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, FunctionDestinationID: record.FunctionDestinationID}, nil
		},
	}, stubApps{}, stubFunctions{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (functiondestination.Destination, error) {
			if gotTenantID != tenantID || gotID != functionID {
				t.Fatal("unexpected function lookup")
			}
			return functiondestination.Destination{ID: functionID}, nil
		},
	}, stubConnectorInstances{})

	sub, err := svc.Create(context.Background(), tenantID, DestinationTypeFunction, nil, &functionID, nil, nil, nil, "example.event", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sub.DestinationType != DestinationTypeFunction {
		t.Fatalf("sub.DestinationType = %q", sub.DestinationType)
	}
}

func TestCreateConnectorSubscription(t *testing.T) {
	connectorID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"
	params := json.RawMessage(`{"text":"New inbound {{event_id}}"}`)

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			if record.ConnectorInstanceID == nil || *record.ConnectorInstanceID != connectorID {
				t.Fatal("unexpected connector instance id")
			}
			if record.Operation == nil || *record.Operation != operation {
				t.Fatal("unexpected operation")
			}
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, ConnectorInstanceID: record.ConnectorInstanceID, Operation: record.Operation}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnectorInstances{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (connectorinstance.Instance, error) {
			if gotTenantID != tenantID || gotID != connectorID {
				t.Fatal("unexpected connector instance lookup")
			}
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameSlack,
				Config:        json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	})

	sub, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, params, "resend.email.received", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sub.DestinationType != DestinationTypeConnector {
		t.Fatalf("sub.DestinationType = %q", sub.DestinationType)
	}
}
