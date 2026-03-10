package validation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connection"
	"groot/internal/integrations/registry"
	"groot/internal/schema"
	"groot/internal/tenant"
	specpkg "groot/internal/workflow/spec"
)

type Resolver interface {
	GetConnection(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
	GetAgentVersion(context.Context, tenant.ID, uuid.UUID) (agent.Version, error)
	GetEventSchema(context.Context, string) (schema.Schema, error)
}

type Validator struct {
	resolver Resolver
}

func New(resolver Resolver) *Validator {
	return &Validator{resolver: resolver}
}

func (v *Validator) Validate(ctx context.Context, tenantID tenant.ID, definition specpkg.Definition) []specpkg.ValidationIssue {
	nodeByID := make(map[string]specpkg.Node, len(definition.Nodes))
	inbound := make(map[string]int, len(definition.Nodes))
	outbound := make(map[string]int, len(definition.Nodes))
	issues := make([]specpkg.ValidationIssue, 0)

	triggerCount := 0
	for i, node := range definition.Nodes {
		path := fmt.Sprintf("nodes[%d]", i)
		node.ID = strings.TrimSpace(node.ID)
		node.Type = strings.TrimSpace(node.Type)
		if node.ID == "" {
			issues = append(issues, issue("missing_node_id", path+".id", "node id is required"))
			continue
		}
		if _, exists := nodeByID[node.ID]; exists {
			issues = append(issues, issue("duplicate_node_id", path+".id", "node id must be unique"))
			continue
		}
		if !isValidNodeType(node.Type) {
			issues = append(issues, issue("invalid_node_type", path+".type", "node type is invalid"))
			nodeByID[node.ID] = node
			continue
		}
		if node.Type == specpkg.NodeTypeTrigger {
			triggerCount++
		}
		nodeByID[node.ID] = node
	}
	if triggerCount == 0 {
		issues = append(issues, issue("missing_trigger", "nodes", "at least one trigger node is required"))
	}

	for i, edge := range definition.Edges {
		path := fmt.Sprintf("edges[%d]", i)
		edge.ID = strings.TrimSpace(edge.ID)
		edge.Source = strings.TrimSpace(edge.Source)
		edge.Target = strings.TrimSpace(edge.Target)
		if edge.ID == "" {
			issues = append(issues, issue("missing_edge_id", path+".id", "edge id is required"))
		}
		if edge.Source == "" || edge.Target == "" {
			issues = append(issues, issue("invalid_edge", path, "edge source and target are required"))
			continue
		}
		if _, ok := nodeByID[edge.Source]; !ok {
			issues = append(issues, issue("unknown_edge_source", path+".source", "edge source node does not exist"))
		}
		if _, ok := nodeByID[edge.Target]; !ok {
			issues = append(issues, issue("unknown_edge_target", path+".target", "edge target node does not exist"))
		}
		if edge.Source == edge.Target {
			issues = append(issues, issue("self_edge", path, "self-referential edges are not supported"))
		}
		outbound[edge.Source]++
		inbound[edge.Target]++
	}

	for _, node := range definition.Nodes {
		switch node.Type {
		case specpkg.NodeTypeTrigger:
			if inbound[node.ID] > 0 {
				issues = append(issues, issue("trigger_inbound", "nodes."+node.ID, "trigger nodes cannot have inbound edges"))
			}
			cfg, err := decodeTrigger(node.Config)
			if err != nil {
				issues = append(issues, issue("invalid_trigger_config", "nodes."+node.ID+".config", err.Error()))
				continue
			}
			if strings.TrimSpace(cfg.Integration) == "" {
				issues = append(issues, issue("missing_trigger_integration", "nodes."+node.ID+".config.integration", "trigger integration is required"))
			} else if registry.GetIntegration(strings.TrimSpace(cfg.Integration)) == nil {
				issues = append(issues, issue("unknown_integration", "nodes."+node.ID+".config.integration", "integration is not registered"))
			}
			if strings.TrimSpace(cfg.EventType) == "" {
				issues = append(issues, issue("missing_event_type", "nodes."+node.ID+".config.event_type", "event_type is required"))
			} else if _, _, ok := schema.ParseFullName(cfg.EventType); !ok {
				issues = append(issues, issue("invalid_event_type", "nodes."+node.ID+".config.event_type", "event_type must be versioned"))
			} else if v.resolver != nil {
				if _, err := v.resolver.GetEventSchema(ctx, cfg.EventType); err != nil {
					issues = append(issues, issue("unknown_event_type", "nodes."+node.ID+".config.event_type", "event type does not exist"))
				}
			}
			if cfg.ConnectionID != nil && v.resolver != nil {
				instance, err := v.resolver.GetConnection(ctx, tenantID, *cfg.ConnectionID)
				if err != nil {
					issues = append(issues, issue("unknown_connection", "nodes."+node.ID+".config.connection_id", "connection does not exist"))
				} else if instance.IntegrationName != strings.TrimSpace(cfg.Integration) {
					issues = append(issues, issue("integration_connection_mismatch", "nodes."+node.ID+".config.connection_id", "connection integration does not match trigger integration"))
				}
			}
		case specpkg.NodeTypeAction:
			if outbound[node.ID] > 1 {
				issues = append(issues, issue("unsupported_branching", "nodes."+node.ID, "action nodes may have at most one outbound edge"))
			}
			cfg, err := decodeAction(node.Config)
			if err != nil {
				issues = append(issues, issue("invalid_action_config", "nodes."+node.ID+".config", err.Error()))
				continue
			}
			if strings.TrimSpace(cfg.Integration) == "" {
				issues = append(issues, issue("missing_action_integration", "nodes."+node.ID+".config.integration", "action integration is required"))
			} else if registry.GetIntegration(strings.TrimSpace(cfg.Integration)) == nil {
				issues = append(issues, issue("unknown_integration", "nodes."+node.ID+".config.integration", "integration is not registered"))
			}
			if cfg.ConnectionID == uuid.Nil {
				issues = append(issues, issue("missing_action_connection", "nodes."+node.ID+".config.connection_id", "action connection_id is required"))
			} else if v.resolver != nil {
				instance, err := v.resolver.GetConnection(ctx, tenantID, cfg.ConnectionID)
				if err != nil {
					issues = append(issues, issue("unknown_connection", "nodes."+node.ID+".config.connection_id", "connection does not exist"))
				} else if instance.IntegrationName != strings.TrimSpace(cfg.Integration) {
					issues = append(issues, issue("integration_connection_mismatch", "nodes."+node.ID+".config.connection_id", "connection integration does not match action integration"))
				}
			}
			if strings.TrimSpace(cfg.Operation) == "" {
				issues = append(issues, issue("missing_action_operation", "nodes."+node.ID+".config.operation", "action operation is required"))
			}
		case specpkg.NodeTypeCondition:
			if outbound[node.ID] == 0 {
				issues = append(issues, issue("condition_without_branch", "nodes."+node.ID, "condition nodes must have outbound edges"))
			}
			cfg, err := decodeCondition(node.Config)
			if err != nil {
				issues = append(issues, issue("invalid_condition_config", "nodes."+node.ID+".config", err.Error()))
				continue
			}
			if strings.TrimSpace(cfg.Expression) == "" {
				issues = append(issues, issue("missing_condition_expression", "nodes."+node.ID+".config.expression", "condition expression is required"))
			}
		case specpkg.NodeTypeAgent:
			if outbound[node.ID] > 1 {
				issues = append(issues, issue("unsupported_branching", "nodes."+node.ID, "agent nodes may have at most one outbound edge"))
			}
			cfg, err := decodeAgent(node.Config)
			if err != nil {
				issues = append(issues, issue("invalid_agent_config", "nodes."+node.ID+".config", err.Error()))
				continue
			}
			if cfg.AgentID == uuid.Nil {
				issues = append(issues, issue("missing_agent_id", "nodes."+node.ID+".config.agent_id", "agent_id is required"))
			}
			if cfg.AgentVersionID == uuid.Nil {
				issues = append(issues, issue("missing_agent_version_id", "nodes."+node.ID+".config.agent_version_id", "agent_version_id is required"))
			} else if v.resolver != nil {
				version, err := v.resolver.GetAgentVersion(ctx, tenantID, cfg.AgentVersionID)
				if err != nil {
					issues = append(issues, issue("unknown_agent_version", "nodes."+node.ID+".config.agent_version_id", "agent version does not exist"))
				} else if cfg.AgentID != uuid.Nil && version.AgentID != cfg.AgentID {
					issues = append(issues, issue("agent_version_mismatch", "nodes."+node.ID+".config.agent_version_id", "agent_version_id does not belong to agent_id"))
				}
			}
		case specpkg.NodeTypeWait:
			if outbound[node.ID] > 1 {
				issues = append(issues, issue("unsupported_branching", "nodes."+node.ID, "wait nodes may have at most one outbound edge"))
			}
			cfg, err := decodeWait(node.Config)
			if err != nil {
				issues = append(issues, issue("invalid_wait_config", "nodes."+node.ID+".config", err.Error()))
				continue
			}
			if strings.TrimSpace(cfg.ExpectedIntegration) == "" {
				issues = append(issues, issue("missing_expected_integration", "nodes."+node.ID+".config.expected_integration", "expected_integration is required"))
			} else if registry.GetIntegration(strings.TrimSpace(cfg.ExpectedIntegration)) == nil {
				issues = append(issues, issue("unknown_integration", "nodes."+node.ID+".config.expected_integration", "integration is not registered"))
			}
			if strings.TrimSpace(cfg.ExpectedEventType) == "" {
				issues = append(issues, issue("missing_expected_event_type", "nodes."+node.ID+".config.expected_event_type", "expected_event_type is required"))
			} else if _, _, ok := schema.ParseFullName(cfg.ExpectedEventType); !ok {
				issues = append(issues, issue("invalid_event_type", "nodes."+node.ID+".config.expected_event_type", "expected_event_type must be versioned"))
			} else if v.resolver != nil {
				if _, err := v.resolver.GetEventSchema(ctx, cfg.ExpectedEventType); err != nil {
					issues = append(issues, issue("unknown_event_type", "nodes."+node.ID+".config.expected_event_type", "event type does not exist"))
				}
			}
			if strings.TrimSpace(cfg.CorrelationStrategy) == "" {
				issues = append(issues, issue("missing_correlation_strategy", "nodes."+node.ID+".config.correlation_strategy", "correlation_strategy is required"))
			}
		case specpkg.NodeTypeEnd:
			if outbound[node.ID] > 0 {
				issues = append(issues, issue("end_has_outbound", "nodes."+node.ID, "end nodes cannot have outbound edges"))
			}
			if _, err := decodeEnd(node.Config); err != nil {
				issues = append(issues, issue("invalid_end_config", "nodes."+node.ID+".config", err.Error()))
			}
		}
	}

	if hasCycle(definition) {
		issues = append(issues, issue("cycle_detected", "edges", "workflow graph must be acyclic"))
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			if issues[i].Code == issues[j].Code {
				return issues[i].Message < issues[j].Message
			}
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Path < issues[j].Path
	})
	return issues
}

func issue(code, path, message string) specpkg.ValidationIssue {
	return specpkg.ValidationIssue{Code: code, Path: path, Message: message}
}

func isValidNodeType(nodeType string) bool {
	switch nodeType {
	case specpkg.NodeTypeTrigger, specpkg.NodeTypeAction, specpkg.NodeTypeCondition, specpkg.NodeTypeAgent, specpkg.NodeTypeWait, specpkg.NodeTypeEnd:
		return true
	default:
		return false
	}
}

func decodeTrigger(raw json.RawMessage) (specpkg.TriggerConfig, error) {
	return decodeConfig[specpkg.TriggerConfig](raw)
}

func decodeAction(raw json.RawMessage) (specpkg.ActionConfig, error) {
	return decodeConfig[specpkg.ActionConfig](raw)
}

func decodeCondition(raw json.RawMessage) (specpkg.ConditionConfig, error) {
	return decodeConfig[specpkg.ConditionConfig](raw)
}

func decodeAgent(raw json.RawMessage) (specpkg.AgentConfig, error) {
	return decodeConfig[specpkg.AgentConfig](raw)
}

func decodeWait(raw json.RawMessage) (specpkg.WaitConfig, error) {
	return decodeConfig[specpkg.WaitConfig](raw)
}

func decodeEnd(raw json.RawMessage) (specpkg.EndConfig, error) {
	return decodeConfig[specpkg.EndConfig](raw)
}

func decodeConfig[T any](raw json.RawMessage) (T, error) {
	var zero T
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(raw, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

func hasCycle(definition specpkg.Definition) bool {
	graph := make(map[string][]string, len(definition.Nodes))
	for _, node := range definition.Nodes {
		graph[node.ID] = nil
	}
	for _, edge := range definition.Edges {
		graph[edge.Source] = append(graph[edge.Source], edge.Target)
	}
	visiting := make(map[string]bool, len(graph))
	visited := make(map[string]bool, len(graph))
	var visit func(string) bool
	visit = func(nodeID string) bool {
		if visiting[nodeID] {
			return true
		}
		if visited[nodeID] {
			return false
		}
		visiting[nodeID] = true
		for _, next := range graph[nodeID] {
			if visit(next) {
				return true
			}
		}
		visiting[nodeID] = false
		visited[nodeID] = true
		return false
	}
	keys := make([]string, 0, len(graph))
	for key := range graph {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if visit(key) {
			return true
		}
	}
	return false
}

func IsValidationFailure(err error) bool {
	var target specpkg.ValidationFailedError
	return errors.As(err, &target)
}
