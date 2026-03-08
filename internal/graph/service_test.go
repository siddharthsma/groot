package graph

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectorinstance"
	"groot/internal/delivery"
	eventpkg "groot/internal/event"
	"groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type stubStore struct {
	connectors  []connectorinstance.Instance
	schemas     []schema.Schema
	subs        []subscription.Subscription
	events      map[uuid.UUID]eventpkg.Event
	jobsByEvent map[uuid.UUID][]delivery.Job
}

func (s stubStore) ListConnectorInstancesAdmin(_ context.Context, tenantID *tenant.ID, connectorName, scope string) ([]connectorinstance.Instance, error) {
	var out []connectorinstance.Instance
	for _, instance := range s.connectors {
		if tenantID != nil && instance.TenantID != uuid.UUID(*tenantID) {
			continue
		}
		if connectorName != "" && instance.ConnectorName != connectorName {
			continue
		}
		if scope != "" && instance.Scope != scope {
			continue
		}
		out = append(out, instance)
	}
	return out, nil
}

func (s stubStore) ListSubscriptionsAdmin(_ context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]subscription.Subscription, error) {
	var out []subscription.Subscription
	for _, sub := range s.subs {
		if tenantID != nil && sub.TenantID != uuid.UUID(*tenantID) {
			continue
		}
		if eventType != "" && sub.EventType != eventType {
			continue
		}
		if destinationType != "" && sub.DestinationType != destinationType {
			continue
		}
		out = append(out, sub)
	}
	return out, nil
}

func (s stubStore) ListEventSchemas(context.Context) ([]schema.Schema, error) {
	return s.schemas, nil
}

func (s stubStore) GetEvent(_ context.Context, id uuid.UUID) (eventpkg.Event, error) {
	evt, ok := s.events[id]
	if !ok {
		return eventpkg.Event{}, errors.New("not found")
	}
	return evt, nil
}

func (s stubStore) ListDeliveryJobsForEvent(_ context.Context, _ tenant.ID, eventID uuid.UUID, _ int) ([]delivery.Job, error) {
	return s.jobsByEvent[eventID], nil
}

type stubMetrics struct {
	requests      int
	nodesObserved int
	edgesObserved int
	limitExceeded int
}

func (s *stubMetrics) IncGraphRequests()      { s.requests++ }
func (s *stubMetrics) AddGraphNodes(n int)    { s.nodesObserved += n }
func (s *stubMetrics) AddGraphEdges(n int)    { s.edgesObserved += n }
func (s *stubMetrics) IncGraphLimitExceeded() { s.limitExceeded++ }

func TestBuildTopologyEdges(t *testing.T) {
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	connectorID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	subID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	metrics := &stubMetrics{}
	service := NewService(stubStore{
		connectors: []connectorinstance.Instance{{
			ID:            connectorID,
			TenantID:      tenantID,
			ConnectorName: connectorinstance.ConnectorNameSlack,
			Scope:         connectorinstance.ScopeTenant,
			Status:        "enabled",
		}},
		schemas: []schema.Schema{{
			FullName:   "slack.message.posted.v1",
			Source:     connectorinstance.ConnectorNameSlack,
			SourceKind: eventpkg.SourceKindExternal,
			Version:    1,
		}},
		subs: []subscription.Subscription{{
			ID:                  subID,
			TenantID:            tenantID,
			DestinationType:     subscription.DestinationTypeConnector,
			ConnectorInstanceID: &connectorID,
			EventType:           "slack.message.posted.v1",
			Status:              subscription.StatusActive,
		}},
	}, Config{MaxNodes: 10, MaxEdges: 10, DefaultLimit: 10, ExecutionTraversalMaxEvents: 10, ExecutionMaxDepth: 5}, nil, metrics)

	result, err := service.BuildTopology(context.Background(), TopologyRequest{IncludeGlobal: true})
	if err != nil {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	if result.Summary.NodesTotal != 3 {
		t.Fatalf("nodes_total = %d, want 3", result.Summary.NodesTotal)
	}
	assertEdge(t, result.Edges, "conninst:"+connectorID.String(), "eventtype:slack.message.posted.v1", "emits")
	assertEdge(t, result.Edges, "eventtype:slack.message.posted.v1", "sub:"+subID.String(), "triggers")
	assertEdge(t, result.Edges, "sub:"+subID.String(), "conninst:"+connectorID.String(), "delivers_to")
	if metrics.requests != 1 || metrics.nodesObserved != 3 || metrics.edgesObserved != 3 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestBuildTopologyTenantFilterExcludesOtherTenant(t *testing.T) {
	tenantA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	connectorA := connectorinstance.Instance{ID: uuid.New(), TenantID: tenantA, ConnectorName: "slack", Scope: connectorinstance.ScopeTenant, Status: "enabled"}
	connectorB := connectorinstance.Instance{ID: uuid.New(), TenantID: tenantB, ConnectorName: "slack", Scope: connectorinstance.ScopeTenant, Status: "enabled"}
	subA := subscription.Subscription{ID: uuid.New(), TenantID: tenantA, DestinationType: subscription.DestinationTypeConnector, ConnectorInstanceID: &connectorA.ID, EventType: "slack.message.posted.v1", Status: subscription.StatusActive}
	subB := subscription.Subscription{ID: uuid.New(), TenantID: tenantB, DestinationType: subscription.DestinationTypeConnector, ConnectorInstanceID: &connectorB.ID, EventType: "slack.message.posted.v1", Status: subscription.StatusActive}
	service := NewService(stubStore{
		connectors: []connectorinstance.Instance{connectorA, connectorB},
		schemas:    []schema.Schema{{FullName: "slack.message.posted.v1", Source: "slack", Version: 1}},
		subs:       []subscription.Subscription{subA, subB},
	}, Config{MaxNodes: 10, MaxEdges: 10, DefaultLimit: 10, ExecutionTraversalMaxEvents: 10, ExecutionMaxDepth: 5}, nil, nil)

	result, err := service.BuildTopology(context.Background(), TopologyRequest{TenantID: &tenantA, IncludeGlobal: false})
	if err != nil {
		t.Fatalf("BuildTopology() error = %v", err)
	}
	for _, node := range result.Nodes {
		if node.Type == "connector_instance" && node.TenantID != nil && *node.TenantID == tenantB {
			t.Fatalf("unexpected tenant B node: %+v", node)
		}
		if node.Type == "subscription" && node.TenantID != nil && *node.TenantID == tenantB {
			t.Fatalf("unexpected tenant B subscription: %+v", node)
		}
	}
}

func TestBuildExecutionChain(t *testing.T) {
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	rootEventID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	resultEventID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	job1ID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	job2ID := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	service := NewService(stubStore{
		events: map[uuid.UUID]eventpkg.Event{
			rootEventID:   {EventID: rootEventID, TenantID: tenantID, Type: "example.root.v1", Source: "manual", SourceKind: eventpkg.SourceKindExternal, Timestamp: now, Payload: json.RawMessage(`{}`)},
			resultEventID: {EventID: resultEventID, TenantID: tenantID, Type: "llm.generate.completed.v1", Source: "llm", SourceKind: eventpkg.SourceKindInternal, Timestamp: now.Add(2 * time.Second), Payload: json.RawMessage(`{}`)},
		},
		jobsByEvent: map[uuid.UUID][]delivery.Job{
			rootEventID: {{
				ID:             job1ID,
				TenantID:       tenantID,
				SubscriptionID: uuid.New(),
				EventID:        rootEventID,
				Status:         delivery.StatusSucceeded,
				ResultEventID:  &resultEventID,
				CreatedAt:      now.Add(500 * time.Millisecond),
			}},
			resultEventID: {{
				ID:             job2ID,
				TenantID:       tenantID,
				SubscriptionID: uuid.New(),
				EventID:        resultEventID,
				Status:         delivery.StatusSucceeded,
				CreatedAt:      now.Add(3 * time.Second),
			}},
		},
	}, Config{MaxNodes: 10, MaxEdges: 10, DefaultLimit: 10, ExecutionTraversalMaxEvents: 10, ExecutionMaxDepth: 5}, nil, nil)

	result, err := service.BuildExecution(context.Background(), rootEventID, ExecutionRequest{})
	if err != nil {
		t.Fatalf("BuildExecution() error = %v", err)
	}
	if result.Summary.JobsTotal != 2 {
		t.Fatalf("jobs_total = %d, want 2", result.Summary.JobsTotal)
	}
	if result.Summary.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Summary.Status)
	}
	assertEdge(t, result.Edges, "evt:"+rootEventID.String(), "job:"+job1ID.String(), "triggered")
	assertEdge(t, result.Edges, "job:"+job1ID.String(), "evt:"+resultEventID.String(), "emitted")
	assertEdge(t, result.Edges, "evt:"+resultEventID.String(), "job:"+job2ID.String(), "triggered")
}

func TestBuildExecutionReturnsPartialWhenDepthExceeded(t *testing.T) {
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	rootEventID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	resultEventID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	thirdEventID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	service := NewService(stubStore{
		events: map[uuid.UUID]eventpkg.Event{
			rootEventID:   {EventID: rootEventID, TenantID: tenantID, Type: "example.root.v1", Source: "manual", SourceKind: eventpkg.SourceKindExternal, Timestamp: time.Now().UTC()},
			resultEventID: {EventID: resultEventID, TenantID: tenantID, Type: "llm.generate.completed.v1", Source: "llm", SourceKind: eventpkg.SourceKindInternal, Timestamp: time.Now().UTC()},
			thirdEventID:  {EventID: thirdEventID, TenantID: tenantID, Type: "slack.posted.v1", Source: "slack", SourceKind: eventpkg.SourceKindInternal, Timestamp: time.Now().UTC()},
		},
		jobsByEvent: map[uuid.UUID][]delivery.Job{
			rootEventID: {{
				ID:             uuid.New(),
				TenantID:       tenantID,
				SubscriptionID: uuid.New(),
				EventID:        rootEventID,
				Status:         delivery.StatusSucceeded,
				ResultEventID:  &resultEventID,
				CreatedAt:      time.Now().UTC(),
			}},
			resultEventID: {{
				ID:             uuid.New(),
				TenantID:       tenantID,
				SubscriptionID: uuid.New(),
				EventID:        resultEventID,
				Status:         delivery.StatusSucceeded,
				ResultEventID:  &thirdEventID,
				CreatedAt:      time.Now().UTC(),
			}},
		},
	}, Config{MaxNodes: 10, MaxEdges: 10, DefaultLimit: 10, ExecutionTraversalMaxEvents: 10, ExecutionMaxDepth: 1}, nil, nil)

	result, err := service.BuildExecution(context.Background(), rootEventID, ExecutionRequest{MaxDepth: 1})
	if err != nil {
		t.Fatalf("BuildExecution() error = %v", err)
	}
	if result.Summary.Status != "partial" {
		t.Fatalf("status = %q, want partial", result.Summary.Status)
	}
}

func assertEdge(t *testing.T, edges []Edge, from, to, edgeType string) {
	t.Helper()
	for _, edge := range edges {
		if edge.From == from && edge.To == to && edge.Type == edgeType {
			return
		}
	}
	t.Fatalf("missing edge %s --%s--> %s", from, edgeType, to)
}
