package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connection"
	"groot/internal/schema"
	"groot/internal/subscription"
	"groot/internal/tenant"
	"groot/internal/workflow"
	workflowcompiler "groot/internal/workflow/compiler"
)

var (
	ErrVersionNotCompiled = errors.New("workflow version must be compiled before publish")
	ErrVersionNotValid    = errors.New("workflow version must be valid before publish")
)

type Store interface {
	GetWorkflow(context.Context, tenant.ID, uuid.UUID) (workflow.Workflow, error)
	GetWorkflowVersion(context.Context, tenant.ID, uuid.UUID) (workflow.Version, error)
	GetConnection(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
	GetAgentVersion(context.Context, tenant.ID, uuid.UUID) (agent.Version, error)
	ListWorkflowEntryBindings(context.Context, tenant.ID, uuid.UUID, *uuid.UUID) ([]workflow.EntryBinding, error)
	ListWorkflowSubscriptionArtifacts(context.Context, tenant.ID, uuid.UUID, *uuid.UUID) ([]workflow.SubscriptionArtifact, error)
	ApplyWorkflowPublish(context.Context, tenant.ID, workflow.Workflow, workflow.Version, []workflow.EntryBinding, []subscription.Record) (workflow.Workflow, workflow.Version, error)
	ApplyWorkflowUnpublish(context.Context, tenant.ID, uuid.UUID) (workflow.Workflow, *workflow.Version, error)
}

type Metrics interface {
	IncWorkflowPublish()
	IncWorkflowPublishFailures()
	AddWorkflowArtifactsCreated(int)
	AddWorkflowArtifactsUpdated(int)
	AddWorkflowArtifactsSuperseded(int)
}

type Service struct {
	store   Store
	metrics Metrics
	now     func() time.Time
}

type PublishResult struct {
	Workflow  workflow.Workflow  `json:"workflow"`
	Version   workflow.Version   `json:"version"`
	Artifacts workflow.Artifacts `json:"artifacts"`
}

type UnpublishResult struct {
	Workflow workflow.Workflow `json:"workflow"`
}

func NewService(store Store, metrics Metrics) *Service {
	return &Service{
		store:   store,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Publish(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (PublishResult, error) {
	version, err := s.store.GetWorkflowVersion(ctx, tenantID, versionID)
	if err != nil {
		return PublishResult{}, err
	}
	if !version.IsValid {
		s.fail()
		return PublishResult{}, ErrVersionNotValid
	}
	if len(version.CompiledJSON) == 0 {
		s.fail()
		return PublishResult{}, ErrVersionNotCompiled
	}
	wf, err := s.store.GetWorkflow(ctx, tenantID, version.WorkflowID)
	if err != nil {
		return PublishResult{}, err
	}

	desiredEntries, desiredSubs, err := s.buildArtifacts(ctx, tenantID, wf, version)
	if err != nil {
		s.fail()
		return PublishResult{}, err
	}

	existingEntries, err := s.store.ListWorkflowEntryBindings(ctx, tenantID, wf.ID, nil)
	if err != nil {
		s.fail()
		return PublishResult{}, fmt.Errorf("list workflow entry bindings: %w", err)
	}
	existingSubs, err := s.store.ListWorkflowSubscriptionArtifacts(ctx, tenantID, wf.ID, nil)
	if err != nil {
		s.fail()
		return PublishResult{}, fmt.Errorf("list workflow subscriptions: %w", err)
	}

	updatedWorkflow, updatedVersion, err := s.store.ApplyWorkflowPublish(ctx, tenantID, wf, version, desiredEntries, desiredSubs)
	if err != nil {
		s.fail()
		return PublishResult{}, err
	}
	artifacts, err := s.ArtifactsByVersion(ctx, tenantID, updatedVersion.ID)
	if err != nil {
		s.fail()
		return PublishResult{}, err
	}
	if s.metrics != nil {
		s.metrics.IncWorkflowPublish()
		s.metrics.AddWorkflowArtifactsCreated(len(desiredEntries) + len(desiredSubs))
		s.metrics.AddWorkflowArtifactsUpdated(0)
		s.metrics.AddWorkflowArtifactsSuperseded(len(activeEntryBindings(existingEntries)) + len(activeSubscriptionArtifacts(existingSubs)))
	}
	return PublishResult{
		Workflow:  updatedWorkflow,
		Version:   updatedVersion,
		Artifacts: artifacts,
	}, nil
}

func (s *Service) Unpublish(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) (UnpublishResult, error) {
	wf, _, err := s.store.ApplyWorkflowUnpublish(ctx, tenantID, workflowID)
	if err != nil {
		return UnpublishResult{}, err
	}
	return UnpublishResult{Workflow: wf}, nil
}

func (s *Service) ArtifactsByWorkflow(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) (workflow.Artifacts, error) {
	entryBindings, err := s.store.ListWorkflowEntryBindings(ctx, tenantID, workflowID, nil)
	if err != nil {
		return workflow.Artifacts{}, err
	}
	subs, err := s.store.ListWorkflowSubscriptionArtifacts(ctx, tenantID, workflowID, nil)
	if err != nil {
		return workflow.Artifacts{}, err
	}
	return buildArtifactsResponse(entryBindings, subs), nil
}

func (s *Service) ArtifactsByVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (workflow.Artifacts, error) {
	version, err := s.store.GetWorkflowVersion(ctx, tenantID, versionID)
	if err != nil {
		return workflow.Artifacts{}, err
	}
	entryBindings, err := s.store.ListWorkflowEntryBindings(ctx, tenantID, version.WorkflowID, &versionID)
	if err != nil {
		return workflow.Artifacts{}, err
	}
	subs, err := s.store.ListWorkflowSubscriptionArtifacts(ctx, tenantID, version.WorkflowID, &versionID)
	if err != nil {
		return workflow.Artifacts{}, err
	}
	return buildArtifactsResponse(entryBindings, subs), nil
}

func (s *Service) buildArtifacts(ctx context.Context, tenantID tenant.ID, wf workflow.Workflow, version workflow.Version) ([]workflow.EntryBinding, []subscription.Record, error) {
	var compiled workflowcompiler.CompiledWorkflow
	if err := json.Unmarshal(version.CompiledJSON, &compiled); err != nil {
		return nil, nil, fmt.Errorf("decode compiled workflow: %w", err)
	}

	nodeBindings := make(map[string]workflowcompiler.CompiledNodeBinding, len(compiled.NodeBindings))
	for _, binding := range compiled.NodeBindings {
		nodeBindings[binding.NodeID] = binding
	}
	outgoing := make(map[string][]string)
	for _, edge := range compiled.RuntimeEdges {
		outgoing[edge.SourceNodeID] = append(outgoing[edge.SourceNodeID], edge.TargetNodeID)
	}

	entryBindings := make([]workflow.EntryBinding, 0, len(compiled.Entrypoints))
	subscriptionsOut := make([]subscription.Record, 0)
	for _, entry := range compiled.Entrypoints {
		entryBindings = append(entryBindings, workflow.EntryBinding{
			ID:                uuid.New(),
			WorkflowID:        wf.ID,
			WorkflowVersionID: version.ID,
			WorkflowNodeID:    entry.NodeID,
			Integration:       entry.Integration,
			EventType:         entry.EventType,
			ConnectionID:      entry.ConnectionID,
			FilterJSON:        entry.Filter,
			Status:            artifactStatusActive,
			CreatedAt:         s.now(),
		})
		source := nodeEventSource{
			WorkflowID:  wf.ID,
			NodeID:      entry.NodeID,
			EventType:   entry.EventType,
			EventSource: normalizeStringPointer(&entry.Integration),
			BaseFilter:  entry.Filter,
		}
		records, err := s.expandSubscriptions(ctx, tenantID, version.ID, outgoing, nodeBindings, source, entry.NodeID, nil)
		if err != nil {
			return nil, nil, err
		}
		subscriptionsOut = append(subscriptionsOut, records...)
	}

	for _, binding := range compiled.NodeBindings {
		if binding.NodeType != workflow.NodeTypeAction && binding.NodeType != workflow.NodeTypeAgent {
			continue
		}
		producedEventType, producedEventSource, ok := producedEvent(binding)
		if !ok {
			continue
		}
		source := nodeEventSource{
			WorkflowID:  wf.ID,
			NodeID:      binding.NodeID,
			EventType:   producedEventType,
			EventSource: normalizeStringPointer(&producedEventSource),
		}
		records, err := s.expandSubscriptions(ctx, tenantID, version.ID, outgoing, nodeBindings, source, binding.NodeID, nil)
		if err != nil {
			return nil, nil, err
		}
		subscriptionsOut = append(subscriptionsOut, records...)
	}

	return entryBindings, subscriptionsOut, nil
}

func (s *Service) expandSubscriptions(ctx context.Context, tenantID tenant.ID, workflowVersionID uuid.UUID, outgoing map[string][]string, nodeBindings map[string]workflowcompiler.CompiledNodeBinding, source nodeEventSource, currentNodeID string, inheritedFilter json.RawMessage) ([]subscription.Record, error) {
	targets := outgoing[currentNodeID]
	if len(targets) == 0 {
		return nil, nil
	}
	combinedBase := combineFilters(source.BaseFilter, inheritedFilter)
	records := make([]subscription.Record, 0)
	for _, targetNodeID := range targets {
		target, ok := nodeBindings[targetNodeID]
		if !ok {
			continue
		}
		switch target.NodeType {
		case workflow.NodeTypeCondition:
			filterValue, err := parseConditionFilter(target.Expression)
			if err != nil {
				return nil, fmt.Errorf("parse condition %s: %w", target.NodeID, err)
			}
			nextRecords, err := s.expandSubscriptions(ctx, tenantID, workflowVersionID, outgoing, nodeBindings, source, target.NodeID, combineFilters(combinedBase, filterValue))
			if err != nil {
				return nil, err
			}
			records = append(records, nextRecords...)
		case workflow.NodeTypeWait:
			waitIntegration := strings.TrimSpace(target.ExpectedIntegration)
			waitEventType := strings.TrimSpace(target.ExpectedEventType)
			if waitIntegration == "" || waitEventType == "" {
				return nil, fmt.Errorf("workflow node %s missing wait trigger metadata", target.NodeID)
			}
			waitSource := nodeEventSource{
				WorkflowID:  source.WorkflowID,
				NodeID:      target.NodeID,
				EventType:   waitEventType,
				EventSource: normalizeStringPointer(&waitIntegration),
				BaseFilter:  combinedBase,
			}
			nextRecords, err := s.expandSubscriptions(ctx, tenantID, workflowVersionID, outgoing, nodeBindings, waitSource, target.NodeID, nil)
			if err != nil {
				return nil, err
			}
			records = append(records, nextRecords...)
		case workflow.NodeTypeAction:
			if target.ConnectionID == nil {
				return nil, fmt.Errorf("workflow node %s missing connection_id", target.NodeID)
			}
			if _, err := s.store.GetConnection(ctx, tenantID, *target.ConnectionID); err != nil {
				return nil, fmt.Errorf("get action connection %s: %w", target.ConnectionID.String(), err)
			}
			op := strings.TrimSpace(target.Operation)
			records = append(records, subscription.Record{
				ID:                     uuid.New(),
				TenantID:               tenantID,
				DestinationType:        subscription.DestinationTypeConnection,
				ConnectionID:           target.ConnectionID,
				Operation:              normalizeStringPointer(&op),
				OperationParams:        normalizeRawJSON(target.Inputs, `{}`),
				Filter:                 combinedBase,
				EventType:              source.EventType,
				EventSource:            source.EventSource,
				EmitSuccessEvent:       true,
				EmitFailureEvent:       true,
				Status:                 subscription.StatusActive,
				CreatedAt:              s.now(),
				WorkflowID:             &source.WorkflowID,
				WorkflowVersionID:      &workflowVersionID,
				WorkflowNodeID:         target.NodeID,
				ManagedByWorkflow:      true,
				WorkflowArtifactStatus: artifactStatusActive,
			})
		case workflow.NodeTypeAgent:
			if target.AgentVersionID == nil {
				return nil, fmt.Errorf("workflow node %s missing agent_version_id", target.NodeID)
			}
			if _, err := s.store.GetAgentVersion(ctx, tenantID, *target.AgentVersionID); err != nil {
				return nil, fmt.Errorf("get agent version %s: %w", target.AgentVersionID.String(), err)
			}
			agentOperation := llmAgentOperation
			sessionKeyTemplate := strings.TrimSpace(target.SessionKeyTemplate)
			createIfMissing := sessionCreateIfMissing(target.SessionMode)
			records = append(records, subscription.Record{
				ID:                     uuid.New(),
				TenantID:               tenantID,
				DestinationType:        subscription.DestinationTypeConnection,
				AgentID:                target.AgentID,
				AgentVersionID:         target.AgentVersionID,
				SessionKeyTemplate:     normalizeStringPointer(&sessionKeyTemplate),
				SessionCreateIfMissing: createIfMissing,
				Operation:              &agentOperation,
				OperationParams:        json.RawMessage(`{}`),
				Filter:                 combinedBase,
				EventType:              source.EventType,
				EventSource:            source.EventSource,
				EmitSuccessEvent:       true,
				EmitFailureEvent:       true,
				Status:                 subscription.StatusActive,
				CreatedAt:              s.now(),
				WorkflowID:             &source.WorkflowID,
				WorkflowVersionID:      &workflowVersionID,
				WorkflowNodeID:         target.NodeID,
				ManagedByWorkflow:      true,
				WorkflowArtifactStatus: artifactStatusActive,
			})
		}
	}
	return records, nil
}

func (s *Service) fail() {
	if s.metrics != nil {
		s.metrics.IncWorkflowPublishFailures()
	}
}

type nodeEventSource struct {
	WorkflowID  uuid.UUID
	NodeID      string
	EventType   string
	EventSource *string
	BaseFilter  json.RawMessage
}

func buildArtifactsResponse(entryBindings []workflow.EntryBinding, subs []workflow.SubscriptionArtifact) workflow.Artifacts {
	summary := workflow.ArtifactsSummary{}
	for _, binding := range entryBindings {
		switch binding.Status {
		case artifactStatusActive:
			summary.EntryBindingsActive++
		case artifactStatusSuperseded:
			summary.EntryBindingsSupersede++
		case artifactStatusInactive:
			summary.EntryBindingsInactive++
		}
	}
	for _, sub := range subs {
		switch sub.ArtifactStatus {
		case artifactStatusActive:
			summary.SubscriptionsActive++
		case artifactStatusSuperseded:
			summary.SubscriptionsSupersede++
		case artifactStatusInactive:
			summary.SubscriptionsInactive++
		}
	}
	return workflow.Artifacts{
		EntryBindings:    entryBindings,
		Subscriptions:    subs,
		ArtifactsSummary: summary,
	}
}

func activeEntryBindings(bindings []workflow.EntryBinding) []workflow.EntryBinding {
	out := make([]workflow.EntryBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Status == artifactStatusActive {
			out = append(out, binding)
		}
	}
	return out
}

func activeSubscriptionArtifacts(subs []workflow.SubscriptionArtifact) []workflow.SubscriptionArtifact {
	out := make([]workflow.SubscriptionArtifact, 0, len(subs))
	for _, sub := range subs {
		if sub.ArtifactStatus == artifactStatusActive {
			out = append(out, sub)
		}
	}
	return out
}

func producedEvent(binding workflowcompiler.CompiledNodeBinding) (string, string, bool) {
	switch binding.NodeType {
	case workflow.NodeTypeAction:
		if strings.TrimSpace(binding.Integration) == "" || strings.TrimSpace(binding.Operation) == "" {
			return "", "", false
		}
		return schema.FullName(fmt.Sprintf("%s.%s.completed", binding.Integration, binding.Operation), 1), strings.TrimSpace(binding.Integration), true
	case workflow.NodeTypeAgent:
		return schema.FullName("llm.agent.completed", 1), connection.IntegrationNameLLM, true
	default:
		return "", "", false
	}
}

func sessionCreateIfMissing(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "existing", "existing_only", "reuse_only":
		return false
	default:
		return true
	}
}

func parseConditionFilter(expression string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, nil
	}
	var normalized any
	if err := json.Unmarshal([]byte(trimmed), &normalized); err != nil {
		return nil, err
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func combineFilters(first, second json.RawMessage) json.RawMessage {
	switch {
	case len(first) == 0:
		return second
	case len(second) == 0:
		return first
	}
	return json.RawMessage(fmt.Sprintf(`{"all":[%s,%s]}`, strings.TrimSpace(string(first)), strings.TrimSpace(string(second))))
}

func normalizeStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeRawJSON(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(fallback)
	}
	return value
}

const (
	artifactStatusActive     = "active"
	artifactStatusSuperseded = "superseded"
	artifactStatusInactive   = "inactive"
	llmAgentOperation        = "agent"
)
