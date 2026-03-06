package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

type stubStore struct {
	createFn    func(context.Context, Record) (Subscription, error)
	updateFn    func(context.Context, tenant.ID, uuid.UUID, Record) (Subscription, error)
	getFn       func(context.Context, tenant.ID, uuid.UUID) (Subscription, error)
	listFn      func(context.Context, tenant.ID) ([]Subscription, error)
	adminListFn func(context.Context, *tenant.ID, string, string) ([]Subscription, error)
	matchFn     func(context.Context, tenant.ID, string, string) ([]Subscription, error)
	setFn       func(context.Context, tenant.ID, uuid.UUID, string) (Subscription, error)
}

func (s stubStore) CreateSubscription(ctx context.Context, record Record) (Subscription, error) {
	return s.createFn(ctx, record)
}

func (s stubStore) UpdateSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, record Record) (Subscription, error) {
	return s.updateFn(ctx, tenantID, subscriptionID, record)
}

func (s stubStore) GetSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	return s.getFn(ctx, tenantID, subscriptionID)
}

func (s stubStore) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]Subscription, error) {
	return s.listFn(ctx, tenantID)
}

func (s stubStore) ListSubscriptionsAdmin(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]Subscription, error) {
	if s.adminListFn == nil {
		return nil, nil
	}
	return s.adminListFn(ctx, tenantID, eventType, destinationType)
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
	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnectorInstances{}, true)
	appID := uuid.New()
	_, err := svc.Create(context.Background(), tenant.ID{}, DestinationTypeWebhook, &appID, nil, nil, nil, nil, nil, " ", nil, false, false)
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
	}, stubApps{}, stubFunctions{}, stubConnectorInstances{}, true)

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
	}, stubConnectorInstances{}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeFunction, nil, &functionID, nil, nil, nil, nil, "example.event.v1", nil, false, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sub := result.Subscription
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
				Scope:         connectorinstance.ScopeTenant,
				OwnerTenantID: ptrUUID(uuid.UUID(tenantID)),
				Config:        json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, params, nil, "resend.email.received.v1", nil, false, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sub := result.Subscription
	if sub.DestinationType != DestinationTypeConnector {
		t.Fatalf("sub.DestinationType = %q", sub.DestinationType)
	}
}

func TestCreateRejectsGlobalConnectorWhenDisabled(t *testing.T) {
	connectorID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"

	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnectorInstances{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error) {
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameSlack,
				Scope:         connectorinstance.ScopeGlobal,
				Config:        json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, false)

	_, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, json.RawMessage(`{"text":"hello"}`), nil, "example.event.v1", nil, false, false)
	if !errors.Is(err, ErrGlobalConnectorNotAllowed) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateAllowsGlobalLLMConnectorWhenDisabled(t *testing.T) {
	connectorID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "generate"

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			return Subscription{ID: record.ID, DestinationType: record.DestinationType}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnectorInstances{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error) {
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameLLM,
				Scope:         connectorinstance.ScopeGlobal,
				Config:        json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false)

	_, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, json.RawMessage(`{"prompt":"Hello {{payload.text}}"}`), nil, "example.event.v1", nil, false, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreatePersistsEmitFlags(t *testing.T) {
	connectorID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			if !record.EmitSuccessEvent || !record.EmitFailureEvent {
				t.Fatalf("emit flags not persisted: success=%t failure=%t", record.EmitSuccessEvent, record.EmitFailureEvent)
			}
			return Subscription{
				ID:               record.ID,
				EmitSuccessEvent: record.EmitSuccessEvent,
				EmitFailureEvent: record.EmitFailureEvent,
			}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnectorInstances{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error) {
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameSlack,
				Scope:         connectorinstance.ScopeTenant,
				OwnerTenantID: ptrUUID(uuid.UUID(tenantID)),
				Config:        json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, json.RawMessage(`{"text":"hello"}`), nil, "example.event.v1", nil, true, true)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sub := result.Subscription
	if !sub.EmitSuccessEvent || !sub.EmitFailureEvent {
		t.Fatalf("sub = %+v", sub)
	}
}

func TestCreateLLMAgentSubscription(t *testing.T) {
	connectorID := uuid.New()
	functionID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "agent"
	params := json.RawMessage(`{
		"instructions":"Triage inbound support messages",
		"allowed_tools":["slack.post_message","notify_support"],
		"tool_bindings":{
			"notify_support":{
				"type":"function",
				"function_destination_id":"` + functionID.String() + `"
			}
		},
		"max_steps":4
	}`)

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			if record.Operation == nil || *record.Operation != operation {
				t.Fatalf("record.Operation = %v", record.Operation)
			}
			cfg, err := agent.ParseConfig(record.OperationParams)
			if err != nil {
				t.Fatalf("ParseConfig() error = %v", err)
			}
			if got, want := len(cfg.AllowedTools), 2; got != want {
				t.Fatalf("len(AllowedTools) = %d, want %d", got, want)
			}
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, Operation: record.Operation}, nil
		},
	}, stubApps{}, stubFunctions{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (functiondestination.Destination, error) {
			if gotTenantID != tenantID || gotID != functionID {
				t.Fatal("unexpected function destination lookup")
			}
			return functiondestination.Destination{ID: functionID}, nil
		},
	}, stubConnectorInstances{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (connectorinstance.Instance, error) {
			if gotTenantID != tenantID || gotID != connectorID {
				t.Fatal("unexpected connector lookup")
			}
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameLLM,
				Scope:         connectorinstance.ScopeGlobal,
				Config:        json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false)

	if _, err := svc.Create(context.Background(), tenantID, DestinationTypeConnector, nil, nil, &connectorID, &operation, params, nil, "resend.email.received.v1", nil, true, true); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsRawFunctionInvokeAgentTool(t *testing.T) {
	connectorID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "agent"

	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnectorInstances{
		getFn: func(_ context.Context, _ tenant.ID, _ uuid.UUID) (connectorinstance.Instance, error) {
			return connectorinstance.Instance{
				ID:            connectorID,
				ConnectorName: connectorinstance.ConnectorNameLLM,
				Scope:         connectorinstance.ScopeGlobal,
				Config:        json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false)

	_, err := svc.Create(
		context.Background(),
		tenantID,
		DestinationTypeConnector,
		nil,
		nil,
		&connectorID,
		&operation,
		json.RawMessage(`{"instructions":"Do work","allowed_tools":["function.invoke"]}`),
		nil,
		"example.event.v1",
		nil,
		false,
		false,
	)
	if !errors.Is(err, ErrInvalidOperationParams) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsUnversionedEventType(t *testing.T) {
	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnectorInstances{}, true)
	appID := uuid.New()
	_, err := svc.Create(context.Background(), tenant.ID{}, DestinationTypeWebhook, &appID, nil, nil, nil, nil, nil, "example.event", nil, false, false)
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("Create() error = %v", err)
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID {
	return &value
}
