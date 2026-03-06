package graph

import (
	"time"

	"github.com/google/uuid"
)

type Node struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Label    string         `json:"label"`
	TenantID *uuid.UUID     `json:"tenant_id"`
	Data     map[string]any `json:"data,omitempty"`
}

type Edge struct {
	From string         `json:"from"`
	To   string         `json:"to"`
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

type TopologyRequest struct {
	TenantID        *uuid.UUID
	ConnectorName   string
	EventTypePrefix string
	IncludeGlobal   bool
	Limit           int
}

type TopologySummary struct {
	Status     string `json:"status"`
	NodesTotal int    `json:"nodes_total"`
	EdgesTotal int    `json:"edges_total"`
}

type Topology struct {
	Nodes   []Node          `json:"nodes"`
	Edges   []Edge          `json:"edges"`
	Summary TopologySummary `json:"summary"`
}

type ExecutionRequest struct {
	MaxDepth  int
	MaxEvents int
}

type ExecutionSummary struct {
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	JobsTotal  int    `json:"jobs_total"`
	JobsFailed int    `json:"jobs_failed"`
}

type ExecutionGraph struct {
	EventID string           `json:"event_id"`
	Nodes   []Node           `json:"nodes"`
	Edges   []Edge           `json:"edges"`
	Summary ExecutionSummary `json:"summary"`
}

type Config struct {
	MaxNodes                    int
	MaxEdges                    int
	ExecutionTraversalMaxEvents int
	ExecutionMaxDepth           int
	DefaultLimit                int
}

type graphTooLargeError struct{}

func (graphTooLargeError) Error() string {
	return "graph_too_large"
}

var ErrGraphTooLarge error = graphTooLargeError{}

func connectorNodeID(id uuid.UUID) string {
	return "conninst:" + id.String()
}

func eventTypeNodeID(fullName string) string {
	return "eventtype:" + fullName
}

func subscriptionNodeID(id uuid.UUID) string {
	return "sub:" + id.String()
}

func eventNodeID(id uuid.UUID) string {
	return "evt:" + id.String()
}

func jobNodeID(id uuid.UUID) string {
	return "job:" + id.String()
}

func durationMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
