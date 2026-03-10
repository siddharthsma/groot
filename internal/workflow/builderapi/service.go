package builderapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connection"
	"groot/internal/integrations/registry"
	"groot/internal/tenant"
	"groot/internal/workflow"
	workflowcompiler "groot/internal/workflow/compiler"
	workflowpublish "groot/internal/workflow/publish"
)

type ConnectionLister interface {
	List(context.Context, tenant.ID) ([]connection.Instance, error)
}

type AgentCatalog interface {
	List(context.Context, tenant.ID) ([]agent.Definition, error)
	ListVersions(context.Context, tenant.ID, uuid.UUID) ([]agent.Version, error)
}

type WorkflowVersionReader interface {
	GetVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Version, error)
}

type ArtifactReader interface {
	ArtifactsByVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Artifacts, error)
}

type Service struct {
	connections ConnectionLister
	agents      AgentCatalog
	versions    WorkflowVersionReader
	artifacts   ArtifactReader
}

func NewService(connections ConnectionLister, agents AgentCatalog, versions WorkflowVersionReader, artifacts ArtifactReader) *Service {
	return &Service{
		connections: connections,
		agents:      agents,
		versions:    versions,
		artifacts:   artifacts,
	}
}

func (s *Service) NodeTypes() NodeTypesResponse {
	return NodeTypesResponse{
		NodeTypes: []NodeTypeCatalog{
			{Type: workflow.NodeTypeTrigger, Label: "Trigger", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{"integration", "event_type"}, OptionalFields: []string{"connection_id", "filter"}}},
			{Type: workflow.NodeTypeAction, Label: "Action", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{"integration", "connection_id", "operation", "inputs"}, OptionalFields: []string{}}},
			{Type: workflow.NodeTypeCondition, Label: "Condition", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{"expression"}, OptionalFields: []string{}}},
			{Type: workflow.NodeTypeAgent, Label: "Agent", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{"agent_id", "agent_version_id"}, OptionalFields: []string{"input_template", "session_mode", "session_key_template"}}},
			{Type: workflow.NodeTypeWait, Label: "Wait", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{"expected_integration", "expected_event_type", "correlation_strategy"}, OptionalFields: []string{"timeout", "resume_same_agent_session"}}},
			{Type: workflow.NodeTypeEnd, Label: "End", ConfigSchema: NodeConfigCatalog{RequiredFields: []string{}, OptionalFields: []string{"terminal_status"}}},
		},
	}
}

func (s *Service) TriggerIntegrations() TriggerIntegrationsResponse {
	entries := registry.ListEntries()
	response := TriggerIntegrationsResponse{Integrations: make([]TriggerIntegrationCatalog, 0)}
	for _, entry := range entries {
		spec := entry.Integration.Spec()
		if spec.Inbound == nil || len(spec.Inbound.EventTypes) == 0 {
			continue
		}
		eventTypes := append([]string(nil), spec.Inbound.EventTypes...)
		sort.Strings(eventTypes)
		response.Integrations = append(response.Integrations, TriggerIntegrationCatalog{
			Name:       spec.Name,
			EventTypes: eventTypes,
		})
	}
	return response
}

func (s *Service) ActionIntegrations() ActionIntegrationsResponse {
	entries := registry.ListEntries()
	response := ActionIntegrationsResponse{Integrations: make([]ActionIntegrationCatalog, 0)}
	for _, entry := range entries {
		spec := entry.Integration.Spec()
		if len(spec.Operations) == 0 {
			continue
		}
		operations := make([]ActionOperationCatalog, 0, len(spec.Operations))
		for _, op := range spec.Operations {
			operations = append(operations, ActionOperationCatalog{Name: op.Name, Description: op.Description})
		}
		response.Integrations = append(response.Integrations, ActionIntegrationCatalog{
			Name:       spec.Name,
			Operations: operations,
		})
	}
	return response
}

func (s *Service) ListConnections(ctx context.Context, tenantID tenant.ID, integrationFilter, scopeFilter, statusFilter string) (ConnectionsResponse, error) {
	instances, err := s.connections.List(ctx, tenantID)
	if err != nil {
		return ConnectionsResponse{}, fmt.Errorf("list connections: %w", err)
	}
	statusFilter = builderConnectionStatus(statusFilter)
	response := ConnectionsResponse{Connections: make([]ConnectionOption, 0, len(instances))}
	for _, instance := range instances {
		normalizedStatus := builderConnectionStatus(instance.Status)
		if integrationFilter != "" && strings.TrimSpace(instance.IntegrationName) != integrationFilter {
			continue
		}
		if scopeFilter != "" && strings.TrimSpace(instance.Scope) != scopeFilter {
			continue
		}
		if statusFilter != "" && normalizedStatus != statusFilter {
			continue
		}
		response.Connections = append(response.Connections, ConnectionOption{
			ID:          instance.ID.String(),
			Name:        "",
			Integration: instance.IntegrationName,
			Scope:       instance.Scope,
			Status:      normalizedStatus,
		})
	}
	sort.SliceStable(response.Connections, func(i, j int) bool {
		if response.Connections[i].Integration == response.Connections[j].Integration {
			return response.Connections[i].ID < response.Connections[j].ID
		}
		return response.Connections[i].Integration < response.Connections[j].Integration
	})
	return response, nil
}

func (s *Service) ListAgents(ctx context.Context, tenantID tenant.ID) (AgentsResponse, error) {
	agents, err := s.agents.List(ctx, tenantID)
	if err != nil {
		return AgentsResponse{}, fmt.Errorf("list agents: %w", err)
	}
	response := AgentsResponse{Agents: make([]AgentOption, 0, len(agents))}
	for _, record := range agents {
		response.Agents = append(response.Agents, AgentOption{ID: record.ID.String(), Name: record.Name})
	}
	sort.SliceStable(response.Agents, func(i, j int) bool { return response.Agents[i].Name < response.Agents[j].Name })
	return response, nil
}

func (s *Service) ListAgentVersions(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) (AgentVersionsResponse, error) {
	versions, err := s.agents.ListVersions(ctx, tenantID, agentID)
	if err != nil {
		return AgentVersionsResponse{}, err
	}
	response := AgentVersionsResponse{
		AgentID:  agentID.String(),
		Versions: make([]AgentVersionOption, 0, len(versions)),
	}
	for _, version := range versions {
		response.Versions = append(response.Versions, AgentVersionOption{
			ID:            version.ID.String(),
			VersionNumber: version.VersionNumber,
			Status:        "active",
			CreatedAt:     version.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return response, nil
}

func (s *Service) WaitStrategies() WaitStrategiesResponse {
	return WaitStrategiesResponse{
		Strategies: []WaitStrategyOption{
			{Name: "event_id", Label: "Event ID"},
			{Name: "payload.<path>", Label: "Payload Path"},
			{Name: "source.connection_id", Label: "Source Connection ID"},
		},
	}
}

func (s *Service) ArtifactMap(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (ArtifactMapResponse, error) {
	version, err := s.versions.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return ArtifactMapResponse{}, err
	}
	var compiled workflowcompiler.CompiledWorkflow
	response := ArtifactMapResponse{
		WorkflowVersionID: versionID.String(),
		Nodes:             []ArtifactMapNode{},
	}
	if len(version.CompiledJSON) == 0 {
		return response, nil
	}
	if err := json.Unmarshal(version.CompiledJSON, &compiled); err != nil {
		return ArtifactMapResponse{}, fmt.Errorf("decode compiled workflow: %w", err)
	}
	artifacts, err := s.artifacts.ArtifactsByVersion(ctx, tenantID, versionID)
	if err != nil {
		return ArtifactMapResponse{}, err
	}

	nodeTypes := make(map[string]string, len(compiled.NodeBindings))
	nodes := make(map[string]*ArtifactMapNode, len(compiled.NodeBindings))
	for _, binding := range compiled.NodeBindings {
		nodeTypes[binding.NodeID] = binding.NodeType
		nodes[binding.NodeID] = &ArtifactMapNode{
			WorkflowNodeID: binding.NodeID,
			NodeType:       binding.NodeType,
			Artifacts:      map[string][]string{},
		}
	}
	for _, entry := range compiled.Entrypoints {
		if _, ok := nodes[entry.NodeID]; !ok {
			nodes[entry.NodeID] = &ArtifactMapNode{
				WorkflowNodeID: entry.NodeID,
				NodeType:       workflow.NodeTypeTrigger,
				Artifacts:      map[string][]string{},
			}
		}
	}

	for _, binding := range artifacts.EntryBindings {
		node := ensureNode(nodes, binding.WorkflowNodeID, workflow.NodeTypeTrigger)
		node.Artifacts["entry_bindings"] = append(node.Artifacts["entry_bindings"], binding.ID.String())
	}
	for _, sub := range artifacts.Subscriptions {
		nodeType := nodeTypes[sub.WorkflowNodeID]
		node := ensureNode(nodes, sub.WorkflowNodeID, nodeType)
		node.Artifacts["subscriptions"] = append(node.Artifacts["subscriptions"], sub.SubscriptionID.String())
	}
	for _, resume := range compiled.Artifacts.ResumeBindingArtifacts {
		node := ensureNode(nodes, resume.WaitNodeID, workflow.NodeTypeWait)
		node.Artifacts["wait_bindings"] = append(node.Artifacts["wait_bindings"], resume.ArtifactID)
	}
	for _, terminal := range compiled.Artifacts.TerminalArtifacts {
		node := ensureNode(nodes, terminal.NodeID, workflow.NodeTypeEnd)
		node.Artifacts["terminals"] = append(node.Artifacts["terminals"], terminal.ArtifactID)
	}

	response.Nodes = make([]ArtifactMapNode, 0, len(nodes))
	for _, node := range nodes {
		sort.Strings(node.Artifacts["entry_bindings"])
		sort.Strings(node.Artifacts["subscriptions"])
		sort.Strings(node.Artifacts["wait_bindings"])
		sort.Strings(node.Artifacts["terminals"])
		response.Nodes = append(response.Nodes, *node)
	}
	sort.SliceStable(response.Nodes, func(i, j int) bool { return response.Nodes[i].WorkflowNodeID < response.Nodes[j].WorkflowNodeID })
	return response, nil
}

func ValidationErrors(issues []workflow.ValidationIssue) []ValidationError {
	if len(issues) == 0 {
		return []ValidationError{}
	}
	errorsOut := make([]ValidationError, 0, len(issues))
	for _, issue := range issues {
		nodeID, fieldPath := normalizeIssuePath(issue.Path)
		errorsOut = append(errorsOut, ValidationError{
			Code:           issue.Code,
			Message:        issue.Message,
			WorkflowNodeID: nodeID,
			FieldPath:      fieldPath,
			Severity:       "error",
		})
	}
	return errorsOut
}

func CompileResponseFromVersion(version workflow.Version) (CompileResponse, error) {
	response := CompileResponse{
		OK:                true,
		WorkflowVersionID: version.ID.String(),
		NodeSummary:       map[string]int{},
		ArtifactSummary:   map[string]int{},
		Errors:            []ValidationError{},
	}
	if version.CompiledHash != nil {
		response.CompiledHash = "sha256:" + strings.TrimSpace(*version.CompiledHash)
	}
	_, definition, err := workflow.NormalizeDefinitionJSON(version.DefinitionJSON)
	if err != nil {
		return CompileResponse{}, fmt.Errorf("normalize definition: %w", err)
	}
	for _, node := range definition.Nodes {
		response.NodeSummary[strings.TrimSpace(node.Type)]++
	}
	if len(version.CompiledJSON) == 0 {
		return response, nil
	}
	var compiled workflowcompiler.CompiledWorkflow
	if err := json.Unmarshal(version.CompiledJSON, &compiled); err != nil {
		return CompileResponse{}, fmt.Errorf("decode compiled workflow: %w", err)
	}
	response.ArtifactSummary["entry_bindings"] = len(compiled.Entrypoints)
	response.ArtifactSummary["subscriptions"] = len(compiled.Artifacts.SubscriptionArtifacts)
	response.ArtifactSummary["wait_bindings"] = len(compiled.Artifacts.ResumeBindingArtifacts)
	return response, nil
}

func PublishResponseFromResult(result workflowpublish.PublishResult) PublishResponse {
	var publishedAt *string
	if result.Version.PublishedAt != nil {
		value := result.Version.PublishedAt.UTC().Format(time.RFC3339)
		publishedAt = &value
	}
	created := result.Artifacts.ArtifactsSummary.EntryBindingsActive + result.Artifacts.ArtifactsSummary.SubscriptionsActive
	superseded := result.Artifacts.ArtifactsSummary.EntryBindingsSupersede + result.Artifacts.ArtifactsSummary.SubscriptionsSupersede
	return PublishResponse{
		OK:                  true,
		WorkflowID:          result.Workflow.ID.String(),
		WorkflowVersionID:   result.Version.ID.String(),
		PublishedAt:         publishedAt,
		ArtifactsCreated:    created,
		ArtifactsUpdated:    0,
		ArtifactsSuperseded: superseded,
		EntryBindingsActive: result.Artifacts.ArtifactsSummary.EntryBindingsActive,
	}
}

func StepResponses(steps []workflow.RunStep, waits []workflow.RunWait) []StepResponse {
	waitIDsByNode := make(map[string]string, len(waits))
	for _, wait := range waits {
		waitIDsByNode[wait.WorkflowNodeID] = wait.ID.String()
	}
	response := make([]StepResponse, 0, len(steps))
	for _, step := range steps {
		var waitID *string
		if value, ok := waitIDsByNode[step.WorkflowNodeID]; ok && step.NodeType == workflow.NodeTypeWait {
			waitValue := value
			waitID = &waitValue
		}
		response = append(response, StepResponse{
			ID:             step.ID.String(),
			WorkflowRunID:  step.WorkflowRunID.String(),
			WorkflowNodeID: step.WorkflowNodeID,
			NodeType:       step.NodeType,
			Status:         step.Status,
			InputEventID:   uuidPtrString(step.InputEventID),
			OutputEventID:  uuidPtrString(step.OutputEventID),
			DeliveryJobID:  uuidPtrString(step.DeliveryJobID),
			AgentRunID:     uuidPtrString(step.AgentRunID),
			WaitID:         waitID,
			BranchKey:      step.BranchKey,
			StartedAt:      step.StartedAt.UTC().Format(time.RFC3339),
			CompletedAt:    timePtrString(step.CompletedAt),
			ErrorSummary:   errorSummary(step.ErrorJSON),
			OutputSummary:  step.OutputSummaryJSON,
		})
	}
	return response
}

func normalizeIssuePath(path string) (*string, *string) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "nodes.") {
		parts := strings.Split(trimmed, ".")
		if len(parts) >= 2 && parts[1] != "" {
			nodeID := parts[1]
			if len(parts) >= 3 {
				field := strings.Join(parts[2:], ".")
				return &nodeID, &field
			}
			return &nodeID, nil
		}
	}
	field := trimmed
	return nil, &field
}

func ensureNode(nodes map[string]*ArtifactMapNode, nodeID string, nodeType string) *ArtifactMapNode {
	node, ok := nodes[nodeID]
	if ok {
		if node.NodeType == "" {
			node.NodeType = nodeType
		}
		return node
	}
	node = &ArtifactMapNode{
		WorkflowNodeID: nodeID,
		NodeType:       nodeType,
		Artifacts:      map[string][]string{},
	}
	nodes[nodeID] = node
	return node
}

func builderConnectionStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "enabled":
		return "active"
	default:
		return strings.TrimSpace(status)
	}
}

func uuidPtrString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func timePtrString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(time.RFC3339)
	return &text
}

func errorSummary(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		if message, ok := decoded["message"].(string); ok && strings.TrimSpace(message) != "" {
			text := message
			return &text
		}
	}
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil
	}
	return &text
}
