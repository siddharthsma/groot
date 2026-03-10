package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connectedapp"
	"groot/internal/connection"
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
	if s.createFn == nil {
		return Subscription{}, nil
	}
	return s.createFn(ctx, record)
}

func (s stubStore) UpdateSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, record Record) (Subscription, error) {
	if s.updateFn == nil {
		return Subscription{}, ErrSubscriptionNotFound
	}
	return s.updateFn(ctx, tenantID, subscriptionID, record)
}

func (s stubStore) GetSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	if s.getFn == nil {
		return Subscription{ID: subscriptionID, TenantID: uuid.UUID(tenantID)}, nil
	}
	return s.getFn(ctx, tenantID, subscriptionID)
}

func (s stubStore) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]Subscription, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn(ctx, tenantID)
}

func (s stubStore) ListSubscriptionsAdmin(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]Subscription, error) {
	if s.adminListFn == nil {
		return nil, nil
	}
	return s.adminListFn(ctx, tenantID, eventType, destinationType)
}

func (s stubStore) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]Subscription, error) {
	if s.matchFn == nil {
		return nil, nil
	}
	return s.matchFn(ctx, tenantID, eventType, eventSource)
}

func (s stubStore) SetSubscriptionStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (Subscription, error) {
	if s.setFn == nil {
		return Subscription{}, ErrSubscriptionNotFound
	}
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

type stubConnections struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
}

func (s stubConnections) GetConnection(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (connection.Instance, error) {
	return s.getFn(ctx, tenantID, id)
}

type stubAgents struct {
	getFn func(context.Context, tenant.ID, uuid.UUID) (agent.Definition, error)
}

func (s stubAgents) Get(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (agent.Definition, error) {
	if s.getFn == nil {
		return agent.Definition{}, agent.ErrNotFound
	}
	return s.getFn(ctx, tenantID, id)
}

func TestCreateRequiresEventType(t *testing.T) {
	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnections{}, true)
	appID := uuid.New()
	_, err := svc.Create(context.Background(), tenant.ID{}, DestinationTypeWebhook, &appID, nil, nil, nil, nil, true, nil, nil, nil, " ", nil, false, false)
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
	}, stubApps{}, stubFunctions{}, stubConnections{}, true)

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
	}, stubConnections{}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeFunction, nil, &functionID, nil, nil, nil, true, nil, nil, nil, "example.event.v1", nil, false, false)
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
			if record.ConnectionID == nil || *record.ConnectionID != connectorID {
				t.Fatal("unexpected connection id")
			}
			if record.Operation == nil || *record.Operation != operation {
				t.Fatal("unexpected operation")
			}
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, ConnectionID: record.ConnectionID, Operation: record.Operation}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (connection.Instance, error) {
			if gotTenantID != tenantID || gotID != connectorID {
				t.Fatal("unexpected connection lookup")
			}
			return connection.Instance{
				ID:              connectorID,
				IntegrationName: connection.IntegrationNameSlack,
				Scope:           connection.ScopeTenant,
				OwnerTenantID:   ptrUUID(uuid.UUID(tenantID)),
				Config:          json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeConnection, nil, nil, &connectorID, nil, nil, true, &operation, params, nil, "resend.email.received.v1", nil, false, false)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	sub := result.Subscription
	if sub.DestinationType != DestinationTypeConnection {
		t.Fatalf("sub.DestinationType = %q", sub.DestinationType)
	}
}

func TestCreateRejectsGlobalConnectionWhenDisabled(t *testing.T) {
	connectionID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"

	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error) {
			return connection.Instance{
				ID:              connectionID,
				IntegrationName: connection.IntegrationNameSlack,
				Scope:           connection.ScopeGlobal,
				Config:          json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, false)

	_, err := svc.Create(context.Background(), tenantID, DestinationTypeConnection, nil, nil, &connectionID, nil, nil, true, &operation, json.RawMessage(`{"text":"hello"}`), nil, "example.event.v1", nil, false, false)
	if !errors.Is(err, ErrGlobalConnectionNotAllowed) {
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
	}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error) {
			return connection.Instance{
				ID:              connectorID,
				IntegrationName: connection.IntegrationNameLLM,
				Scope:           connection.ScopeGlobal,
				Config:          json.RawMessage(`{"default_integration":"openai","integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false)

	_, err := svc.Create(context.Background(), tenantID, DestinationTypeConnection, nil, nil, &connectorID, nil, nil, true, &operation, json.RawMessage(`{"prompt":"Hello {{payload.text}}"}`), nil, "example.event.v1", nil, false, false)
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
	}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error) {
			return connection.Instance{
				ID:              connectorID,
				IntegrationName: connection.IntegrationNameSlack,
				Scope:           connection.ScopeTenant,
				OwnerTenantID:   ptrUUID(uuid.UUID(tenantID)),
				Config:          json.RawMessage(`{"bot_token":"xoxb-123","default_channel":"#alerts"}`),
			}, nil
		},
	}, true)

	result, err := svc.Create(context.Background(), tenantID, DestinationTypeConnection, nil, nil, &connectorID, nil, nil, true, &operation, json.RawMessage(`{"text":"hello"}`), nil, "example.event.v1", nil, true, true)
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
	agentID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "agent"
	template := "resend:thread:{{payload.thread_id}}"

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			if record.Operation == nil || *record.Operation != operation {
				t.Fatalf("record.Operation = %v", record.Operation)
			}
			if record.AgentID == nil || *record.AgentID != agentID {
				t.Fatalf("record.AgentID = %v", record.AgentID)
			}
			if record.SessionKeyTemplate == nil || *record.SessionKeyTemplate != template {
				t.Fatalf("record.SessionKeyTemplate = %v", record.SessionKeyTemplate)
			}
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, Operation: record.Operation}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (connection.Instance, error) {
			if gotTenantID != tenantID || gotID != connectorID {
				t.Fatal("unexpected connection lookup")
			}
			return connection.Instance{
				ID:              connectorID,
				IntegrationName: connection.IntegrationNameLLM,
				Scope:           connection.ScopeGlobal,
				Config:          json.RawMessage(`{"default_integration":"openai","integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false, WithAgentStore(stubAgents{
		getFn: func(_ context.Context, gotTenantID tenant.ID, gotID uuid.UUID) (agent.Definition, error) {
			if gotTenantID != tenantID || gotID != agentID {
				t.Fatal("unexpected agent lookup")
			}
			return agent.Definition{ID: agentID}, nil
		},
	}))

	if _, err := svc.Create(context.Background(), tenantID, DestinationTypeConnection, nil, nil, &connectorID, &agentID, &template, true, &operation, json.RawMessage(`{}`), nil, "resend.email.received.v1", nil, true, true); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateRejectsSubscriptionLevelAgentParams(t *testing.T) {
	connectorID := uuid.New()
	agentID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	operation := "agent"
	template := "example:{{payload.id}}"

	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnections{
		getFn: func(_ context.Context, _ tenant.ID, _ uuid.UUID) (connection.Instance, error) {
			return connection.Instance{
				ID:              connectorID,
				IntegrationName: connection.IntegrationNameLLM,
				Scope:           connection.ScopeGlobal,
				Config:          json.RawMessage(`{"default_integration":"openai","integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
			}, nil
		},
	}, false, WithAgentStore(stubAgents{
		getFn: func(_ context.Context, _ tenant.ID, _ uuid.UUID) (agent.Definition, error) {
			return agent.Definition{ID: agentID}, nil
		},
	}))

	_, err := svc.Create(
		context.Background(),
		tenantID,
		DestinationTypeConnection,
		nil,
		nil,
		&connectorID,
		&agentID,
		&template,
		true,
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
	svc := NewService(stubStore{}, stubApps{}, stubFunctions{}, stubConnections{}, true)
	appID := uuid.New()
	_, err := svc.Create(context.Background(), tenant.ID{}, DestinationTypeWebhook, &appID, nil, nil, nil, nil, true, nil, nil, nil, "example.event", nil, false, false)
	if !errors.Is(err, ErrInvalidEventType) {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateAllowsSourceAwareConnectionTemplatesWithoutExplicitConnectionID(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, OperationParams: record.OperationParams}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnections{}, true)

	_, err := svc.Create(
		context.Background(),
		tenantID,
		DestinationTypeConnection,
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		&operation,
		json.RawMessage(`{"channel":"#alerts","text":"From {{source.connection_id}} via {{lineage.connection_id}}"}`),
		nil,
		"example.event.v1",
		nil,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestCreateAllowsPayloadTemplatesForImplicitSlackConnectionOnLLMEvents(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	operation := "post_message"

	svc := NewService(stubStore{
		createFn: func(_ context.Context, record Record) (Subscription, error) {
			return Subscription{ID: record.ID, DestinationType: record.DestinationType, OperationParams: record.OperationParams}, nil
		},
	}, stubApps{}, stubFunctions{}, stubConnections{}, true)

	_, err := svc.Create(
		context.Background(),
		tenantID,
		DestinationTypeConnection,
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		&operation,
		json.RawMessage(`{"channel":"#alerts","text":"Summary {{payload.text}}"}`),
		nil,
		"llm.generate.completed.v1",
		nil,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID {
	return &value
}
