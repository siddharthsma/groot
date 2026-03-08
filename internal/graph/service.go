package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectorinstance"
	"groot/internal/delivery"
	"groot/internal/event"
	"groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type Store interface {
	ListConnectorInstancesAdmin(context.Context, *tenant.ID, string, string) ([]connectorinstance.Instance, error)
	ListSubscriptionsAdmin(context.Context, *tenant.ID, string, string) ([]subscription.Subscription, error)
	ListEventSchemas(context.Context) ([]schema.Schema, error)
	GetEvent(context.Context, uuid.UUID) (event.Event, error)
	ListDeliveryJobsForEvent(context.Context, tenant.ID, uuid.UUID, int) ([]delivery.Job, error)
}

type Metrics interface {
	IncGraphRequests()
	AddGraphNodes(int)
	AddGraphEdges(int)
	IncGraphLimitExceeded()
}

type Service struct {
	store   Store
	cfg     Config
	logger  *slog.Logger
	metrics Metrics
}

func NewService(store Store, cfg Config, logger *slog.Logger, metrics Metrics) *Service {
	if cfg.MaxNodes <= 0 {
		cfg.MaxNodes = 5000
	}
	if cfg.MaxEdges <= 0 {
		cfg.MaxEdges = 20000
	}
	if cfg.ExecutionTraversalMaxEvents <= 0 {
		cfg.ExecutionTraversalMaxEvents = 500
	}
	if cfg.ExecutionMaxDepth <= 0 {
		cfg.ExecutionMaxDepth = 25
	}
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 500
	}
	return &Service{store: store, cfg: cfg, logger: logger, metrics: metrics}
}

func (s *Service) BuildTopology(ctx context.Context, req TopologyRequest) (Topology, error) {
	if s.metrics != nil {
		s.metrics.IncGraphRequests()
	}
	limit := s.normalizeLimit(req.Limit)
	connectorName := strings.TrimSpace(req.ConnectorName)
	eventTypePrefix := strings.TrimSpace(req.EventTypePrefix)

	connectors, err := s.store.ListConnectorInstancesAdmin(ctx, nil, connectorName, "")
	if err != nil {
		return Topology{}, fmt.Errorf("list connector instances: %w", err)
	}
	schemasList, err := s.store.ListEventSchemas(ctx)
	if err != nil {
		return Topology{}, fmt.Errorf("list event schemas: %w", err)
	}
	subs, err := s.store.ListSubscriptionsAdmin(ctx, nil, "", "")
	if err != nil {
		return Topology{}, fmt.Errorf("list subscriptions: %w", err)
	}

	builder := newBuilder(s.cfg.MaxNodes, s.cfg.MaxEdges)
	schemaByFullName := make(map[string]schema.Schema)
	matchingSchemaSources := make(map[string]struct{})
	for _, schema := range schemasList {
		if eventTypePrefix != "" && !strings.HasPrefix(schema.FullName, eventTypePrefix) {
			continue
		}
		schemaByFullName[schema.FullName] = schema
		matchingSchemaSources[schema.Source] = struct{}{}
		node := Node{
			ID:    eventTypeNodeID(schema.FullName),
			Type:  "event_type",
			Label: schema.FullName,
			Data: map[string]any{
				"source":      schema.Source,
				"source_kind": schema.SourceKind,
				"version":     schema.Version,
			},
		}
		if err := builder.addNode(node); err != nil {
			s.recordLimitExceeded("graph_topology_built")
			return Topology{}, err
		}
	}

	for _, instance := range connectors {
		if req.TenantID != nil && instance.Scope == connectorinstance.ScopeTenant && instance.TenantID != *req.TenantID {
			continue
		}
		if !req.IncludeGlobal && instance.Scope == connectorinstance.ScopeGlobal {
			continue
		}
		if eventTypePrefix != "" {
			if _, ok := matchingSchemaSources[instance.ConnectorName]; !ok {
				continue
			}
		}
		node := connectorNode(instance)
		if err := builder.addNode(node); err != nil {
			s.recordLimitExceeded("graph_topology_built")
			return Topology{}, err
		}
		for _, schema := range schemaByFullName {
			if schema.Source != instance.ConnectorName {
				continue
			}
			if err := builder.addEdge(Edge{
				From: connectorNodeID(instance.ID),
				To:   eventTypeNodeID(schema.FullName),
				Type: "emits",
			}); err != nil {
				s.recordLimitExceeded("graph_topology_built")
				return Topology{}, err
			}
		}
	}

	for _, sub := range subs {
		if req.TenantID != nil && sub.TenantID != *req.TenantID {
			continue
		}
		if eventTypePrefix != "" && !strings.HasPrefix(sub.EventType, eventTypePrefix) {
			continue
		}
		node := subscriptionNode(sub)
		if err := builder.addNode(node); err != nil {
			s.recordLimitExceeded("graph_topology_built")
			return Topology{}, err
		}
		if _, ok := schemaByFullName[sub.EventType]; ok {
			if err := builder.addEdge(Edge{
				From: eventTypeNodeID(sub.EventType),
				To:   subscriptionNodeID(sub.ID),
				Type: "triggers",
			}); err != nil {
				s.recordLimitExceeded("graph_topology_built")
				return Topology{}, err
			}
		}
		if sub.DestinationType == subscription.DestinationTypeConnector && sub.ConnectorInstanceID != nil {
			targetID := connectorNodeID(*sub.ConnectorInstanceID)
			if builder.hasNode(targetID) {
				if err := builder.addEdge(Edge{
					From: subscriptionNodeID(sub.ID),
					To:   targetID,
					Type: "delivers_to",
				}); err != nil {
					s.recordLimitExceeded("graph_topology_built")
					return Topology{}, err
				}
			}
		}
	}

	if len(builder.nodes) > limit {
		s.recordLimitExceeded("graph_topology_built")
		return Topology{}, ErrGraphTooLarge
	}

	topology := Topology{
		Nodes: builder.nodesSlice(),
		Edges: builder.edgesSlice(),
		Summary: TopologySummary{
			Status:     "ok",
			NodesTotal: len(builder.nodes),
			EdgesTotal: len(builder.edges),
		},
	}
	s.recordBuilt("graph_topology_built", topology.Summary.NodesTotal, topology.Summary.EdgesTotal)
	return topology, nil
}

func (s *Service) BuildExecution(ctx context.Context, eventID uuid.UUID, req ExecutionRequest) (ExecutionGraph, error) {
	if s.metrics != nil {
		s.metrics.IncGraphRequests()
	}
	root, err := s.store.GetEvent(ctx, eventID)
	if err != nil {
		return ExecutionGraph{}, fmt.Errorf("get root event: %w", err)
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 || maxDepth > s.cfg.ExecutionMaxDepth {
		maxDepth = s.cfg.ExecutionMaxDepth
	}
	maxEvents := req.MaxEvents
	if maxEvents <= 0 || maxEvents > s.cfg.ExecutionTraversalMaxEvents {
		maxEvents = s.cfg.ExecutionTraversalMaxEvents
	}

	builder := newBuilder(s.cfg.MaxNodes, s.cfg.MaxEdges)
	queue := []executionCursor{{event: root, depth: 0}}
	visited := map[uuid.UUID]struct{}{root.EventID: {}}
	partial := false
	jobsTotal := 0
	jobsFailed := 0
	startedAt := root.Timestamp
	endedAt := root.Timestamp

	for len(queue) > 0 {
		cursor := queue[0]
		queue = queue[1:]

		if err := builder.addNode(eventNode(cursor.event)); err != nil {
			s.recordLimitExceeded("graph_execution_built")
			return ExecutionGraph{}, err
		}
		if cursor.event.Timestamp.After(endedAt) {
			endedAt = cursor.event.Timestamp
		}

		jobs, err := s.store.ListDeliveryJobsForEvent(ctx, tenant.ID(cursor.event.TenantID), cursor.event.EventID, s.cfg.MaxNodes)
		if err != nil {
			return ExecutionGraph{}, fmt.Errorf("list delivery jobs for event %s: %w", cursor.event.EventID, err)
		}
		for _, job := range jobs {
			jobsTotal++
			if job.Status == delivery.StatusFailed || job.Status == delivery.StatusDeadLetter {
				jobsFailed++
			}
			if err := builder.addNode(jobNode(job)); err != nil {
				s.recordLimitExceeded("graph_execution_built")
				return ExecutionGraph{}, err
			}
			if err := builder.addEdge(Edge{
				From: eventNodeID(cursor.event.EventID),
				To:   jobNodeID(job.ID),
				Type: "triggered",
			}); err != nil {
				s.recordLimitExceeded("graph_execution_built")
				return ExecutionGraph{}, err
			}
			endedAt = maxTime(endedAt, job.CreatedAt)
			if job.CompletedAt != nil {
				endedAt = maxTime(endedAt, *job.CompletedAt)
			}
			if job.ResultEventID == nil {
				continue
			}
			resultEvent, err := s.store.GetEvent(ctx, *job.ResultEventID)
			if err != nil {
				continue
			}
			if err := builder.addNode(eventNode(resultEvent)); err != nil {
				s.recordLimitExceeded("graph_execution_built")
				return ExecutionGraph{}, err
			}
			if err := builder.addEdge(Edge{
				From: jobNodeID(job.ID),
				To:   eventNodeID(resultEvent.EventID),
				Type: "emitted",
			}); err != nil {
				s.recordLimitExceeded("graph_execution_built")
				return ExecutionGraph{}, err
			}
			endedAt = maxTime(endedAt, resultEvent.Timestamp)

			if _, ok := visited[resultEvent.EventID]; ok {
				continue
			}
			if len(visited) >= maxEvents {
				partial = true
				continue
			}
			if cursor.depth+1 > maxDepth {
				partial = true
				continue
			}
			visited[resultEvent.EventID] = struct{}{}
			queue = append(queue, executionCursor{event: resultEvent, depth: cursor.depth + 1})
		}
	}

	status := "succeeded"
	if partial {
		status = "partial"
	} else if jobsFailed > 0 {
		status = "failed"
	}
	result := ExecutionGraph{
		EventID: root.EventID.String(),
		Nodes:   builder.nodesSlice(),
		Edges:   builder.edgesSlice(),
		Summary: ExecutionSummary{
			Status:     status,
			DurationMS: durationMillis(startedAt, endedAt),
			JobsTotal:  jobsTotal,
			JobsFailed: jobsFailed,
		},
	}
	s.recordBuilt("graph_execution_built", len(builder.nodes), len(builder.edges))
	return result, nil
}

type executionCursor struct {
	event event.Event
	depth int
}

type builder struct {
	maxNodes int
	maxEdges int
	nodes    map[string]Node
	edges    map[string]Edge
}

func newBuilder(maxNodes, maxEdges int) *builder {
	return &builder{
		maxNodes: maxNodes,
		maxEdges: maxEdges,
		nodes:    make(map[string]Node),
		edges:    make(map[string]Edge),
	}
}

func (b *builder) addNode(node Node) error {
	if _, ok := b.nodes[node.ID]; ok {
		return nil
	}
	if len(b.nodes) >= b.maxNodes {
		return ErrGraphTooLarge
	}
	b.nodes[node.ID] = node
	return nil
}

func (b *builder) hasNode(id string) bool {
	_, ok := b.nodes[id]
	return ok
}

func (b *builder) addEdge(edge Edge) error {
	key := edge.From + "|" + edge.Type + "|" + edge.To
	if _, ok := b.edges[key]; ok {
		return nil
	}
	if len(b.edges) >= b.maxEdges {
		return ErrGraphTooLarge
	}
	b.edges[key] = edge
	return nil
}

func (b *builder) nodesSlice() []Node {
	out := make([]Node, 0, len(b.nodes))
	for _, node := range b.nodes {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (b *builder) edgesSlice() []Edge {
	out := make([]Edge, 0, len(b.edges))
	for _, edge := range b.edges {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From == out[j].From {
			if out[i].Type == out[j].Type {
				return out[i].To < out[j].To
			}
			return out[i].Type < out[j].Type
		}
		return out[i].From < out[j].From
	})
	return out
}

func connectorNode(instance connectorinstance.Instance) Node {
	var tenantID *uuid.UUID
	if instance.Scope == connectorinstance.ScopeTenant {
		id := instance.TenantID
		tenantID = &id
	}
	return Node{
		ID:       connectorNodeID(instance.ID),
		Type:     "connector_instance",
		Label:    instance.ConnectorName,
		TenantID: tenantID,
		Data: map[string]any{
			"connector_name": instance.ConnectorName,
			"scope":          instance.Scope,
			"status":         instance.Status,
		},
	}
}

func subscriptionNode(sub subscription.Subscription) Node {
	tenantID := uuid.UUID(sub.TenantID)
	data := map[string]any{
		"destination_type":   sub.DestinationType,
		"event_type":         sub.EventType,
		"status":             sub.Status,
		"emit_success_event": sub.EmitSuccessEvent,
		"emit_failure_event": sub.EmitFailureEvent,
	}
	if sub.Operation != nil {
		data["operation"] = *sub.Operation
	}
	if sub.EventSource != nil {
		data["event_source"] = *sub.EventSource
	}
	return Node{
		ID:       subscriptionNodeID(sub.ID),
		Type:     "subscription",
		Label:    sub.EventType,
		TenantID: &tenantID,
		Data:     data,
	}
}

func eventNode(event event.Event) Node {
	tenantID := event.TenantID
	return Node{
		ID:       eventNodeID(event.EventID),
		Type:     "event",
		Label:    event.Type,
		TenantID: &tenantID,
		Data: map[string]any{
			"type":           event.Type,
			"source":         event.Source,
			"source_kind":    event.SourceKind,
			"occurred_at":    event.Timestamp,
			"chain_depth":    event.ChainDepth,
			"schema":         event.SchemaFullName,
			"schema_version": event.SchemaVersion,
		},
	}
}

func jobNode(job delivery.Job) Node {
	tenantID := job.TenantID
	data := map[string]any{
		"status":          job.Status,
		"subscription_id": job.SubscriptionID.String(),
		"attempts":        job.Attempts,
		"is_replay":       job.IsReplay,
		"created_at":      job.CreatedAt,
	}
	if job.ResultEventID != nil {
		data["result_event_id"] = job.ResultEventID.String()
	}
	if job.ExternalID != nil {
		data["external_id"] = *job.ExternalID
	}
	if job.LastStatusCode != nil {
		data["last_status_code"] = *job.LastStatusCode
	}
	return Node{
		ID:       jobNodeID(job.ID),
		Type:     "delivery_job",
		Label:    job.Status,
		TenantID: &tenantID,
		Data:     data,
	}
}

func (s *Service) normalizeLimit(value int) int {
	if value <= 0 {
		return s.cfg.DefaultLimit
	}
	if value > s.cfg.MaxNodes {
		return s.cfg.MaxNodes
	}
	return value
}

func (s *Service) recordBuilt(msg string, nodes, edges int) {
	if s.metrics != nil {
		s.metrics.AddGraphNodes(nodes)
		s.metrics.AddGraphEdges(edges)
	}
	if s.logger != nil {
		s.logger.Info(msg, slog.Int("nodes", nodes), slog.Int("edges", edges))
	}
}

func (s *Service) recordLimitExceeded(msg string) {
	if s.metrics != nil {
		s.metrics.IncGraphLimitExceeded()
	}
	if s.logger != nil {
		s.logger.Info("graph_limit_exceeded", slog.String("graph", msg))
	}
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func IsGraphTooLarge(err error) bool {
	return errors.Is(err, ErrGraphTooLarge)
}
