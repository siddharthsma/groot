package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/delivery"
	eventpkg "groot/internal/event"
	"groot/internal/ingest"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
	"groot/internal/tenant"
	"groot/internal/workflow"
	workflowcompiler "groot/internal/workflow/compiler"
)

const workflowIntegration = "workflow"

type Store interface {
	ListActiveWorkflowEntryBindingsByEvent(context.Context, tenant.ID, string, string) ([]workflow.EntryBinding, error)
	GetActiveWorkflowSubscriptionArtifactByNode(context.Context, tenant.ID, uuid.UUID, string) (workflow.SubscriptionArtifact, error)
	CreateWorkflowRun(context.Context, workflow.RunRecord, workflow.RunStepRecord) (workflow.Run, workflow.RunStep, error)
	CreateWorkflowRunStep(context.Context, workflow.RunStepRecord) (workflow.RunStep, error)
	AttachWorkflowRunStepDelivery(context.Context, uuid.UUID, uuid.UUID) error
	CompleteWorkflowRunStep(context.Context, uuid.UUID, string, time.Time, *uuid.UUID, *uuid.UUID, *uuid.UUID, json.RawMessage, json.RawMessage) error
	CreateWorkflowRunWait(context.Context, workflow.RunWaitRecord) (workflow.RunWait, error)
	ListMatchingWorkflowRunWaits(context.Context, tenant.ID, string, string, string) ([]workflow.RunWait, error)
	MatchWorkflowRunWait(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListExpiredWorkflowRunWaits(context.Context, time.Time, int) ([]workflow.RunWait, error)
	TimeoutWorkflowRunWait(context.Context, uuid.UUID, time.Time) error
	SetWorkflowRunStatus(context.Context, uuid.UUID, string, *time.Time, *string) error
	GetWorkflowRunByID(context.Context, uuid.UUID) (workflow.Run, error)
	GetWorkflowRun(context.Context, tenant.ID, uuid.UUID) (workflow.Run, error)
	ListWorkflowRuns(context.Context, tenant.ID, uuid.UUID, int) ([]workflow.Run, error)
	ListWorkflowRunSteps(context.Context, tenant.ID, uuid.UUID) ([]workflow.RunStep, error)
	ListWorkflowRunWaits(context.Context, tenant.ID, uuid.UUID) ([]workflow.RunWait, error)
	CancelWorkflowRun(context.Context, tenant.ID, uuid.UUID, time.Time) (workflow.Run, error)
	GetWorkflowVersionCompiledByID(context.Context, uuid.UUID) (workflow.Version, error)
	CreateDeliveryJob(context.Context, delivery.JobRecord) (bool, error)
}

type EventIngestor interface {
	Ingest(context.Context, ingest.Request) (eventpkg.Event, error)
}

type Metrics interface {
	IncWorkflowRunsStarted()
	IncWorkflowRunsCompleted()
	IncWorkflowRunsFailed()
	IncWorkflowRunsWaiting()
	IncWorkflowWaitsRegistered()
	IncWorkflowWaitsMatched()
	IncWorkflowWaitsTimedOut()
}

type Service struct {
	store   Store
	ingest  EventIngestor
	logger  *slog.Logger
	metrics Metrics
	now     func() time.Time
}

func NewService(store Store, ingestor EventIngestor, logger *slog.Logger, metrics Metrics) *Service {
	return &Service{
		store:   store,
		ingest:  ingestor,
		logger:  logger,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ProcessEvent(ctx context.Context, evt eventpkg.Event) error {
	if evt.WorkflowRunID == nil {
		if err := s.startRunsForEvent(ctx, evt); err != nil {
			return err
		}
		if err := s.resumeWaitsForEvent(ctx, evt); err != nil {
			return err
		}
		return nil
	}
	return s.processWorkflowEvent(ctx, evt)
}

func (s *Service) ScheduleSubscription(ctx context.Context, evt eventpkg.Event, sub subscription.Subscription) (bool, error) {
	if evt.WorkflowRunID == nil || !sub.ManagedByWorkflow {
		return false, nil
	}
	run, err := s.store.GetWorkflowRunByID(ctx, *evt.WorkflowRunID)
	if err != nil {
		return false, fmt.Errorf("get workflow run: %w", err)
	}
	if sub.WorkflowVersionID == nil || run.WorkflowVersionID != *sub.WorkflowVersionID {
		return false, nil
	}

	jobID := uuid.New()
	step, err := s.store.CreateWorkflowRunStep(ctx, workflow.RunStepRecord{
		ID:             uuid.New(),
		WorkflowRunID:  run.ID,
		WorkflowNodeID: sub.WorkflowNodeID,
		NodeType:       workflowNodeTypeForSubscription(sub),
		Status:         workflow.RunStepStatusRunning,
		InputEventID:   &evt.EventID,
		SubscriptionID: &sub.ID,
		StartedAt:      s.now(),
	})
	if err != nil {
		return false, fmt.Errorf("create workflow run step: %w", err)
	}
	created, err := s.store.CreateDeliveryJob(ctx, delivery.JobRecord{
		ID:             jobID,
		TenantID:       evt.TenantID,
		SubscriptionID: sub.ID,
		EventID:        evt.EventID,
		WorkflowRunID:  &run.ID,
		WorkflowNodeID: sub.WorkflowNodeID,
		Status:         delivery.StatusPending,
		CreatedAt:      s.now(),
	})
	if err != nil {
		return false, fmt.Errorf("create workflow delivery job: %w", err)
	}
	if !created {
		_ = s.store.CompleteWorkflowRunStep(ctx, step.ID, workflow.RunStepStatusSkipped, s.now(), nil, nil, nil, nil, nil)
		return false, nil
	}
	if err := s.store.AttachWorkflowRunStepDelivery(ctx, step.ID, jobID); err != nil {
		return false, fmt.Errorf("attach workflow delivery job: %w", err)
	}
	if s.logger != nil {
		s.logger.Info("workflow_delivery_scheduled",
			slog.String("workflow_run_id", run.ID.String()),
			slog.String("workflow_node_id", sub.WorkflowNodeID),
			slog.String("delivery_job_id", jobID.String()),
			slog.String("event_id", evt.EventID.String()),
		)
	}
	return true, nil
}

func (s *Service) ListRuns(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, limit int) ([]workflow.Run, error) {
	return s.store.ListWorkflowRuns(ctx, tenantID, workflowID, normalizeRunLimit(limit))
}

func (s *Service) GetRun(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) (workflow.Run, error) {
	return s.store.GetWorkflowRun(ctx, tenantID, runID)
}

func (s *Service) ListRunSteps(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) ([]workflow.RunStep, error) {
	return s.store.ListWorkflowRunSteps(ctx, tenantID, runID)
}

func (s *Service) ListRunWaits(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) ([]workflow.RunWait, error) {
	return s.store.ListWorkflowRunWaits(ctx, tenantID, runID)
}

func (s *Service) CancelRun(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) (workflow.Run, error) {
	run, err := s.store.CancelWorkflowRun(ctx, tenantID, runID, s.now())
	if err != nil {
		return workflow.Run{}, err
	}
	_ = s.emitWorkflowNodeEvent(ctx, run, workflow.RunStep{}, "cancelled", nil, nil)
	return run, nil
}

func (s *Service) SweepExpiredWaits(ctx context.Context, limit int) error {
	waits, err := s.store.ListExpiredWorkflowRunWaits(ctx, s.now(), limit)
	if err != nil {
		return fmt.Errorf("list expired workflow waits: %w", err)
	}
	for _, wait := range waits {
		if err := s.store.TimeoutWorkflowRunWait(ctx, wait.ID, s.now()); err != nil {
			return fmt.Errorf("timeout workflow wait %s: %w", wait.ID.String(), err)
		}
		if s.metrics != nil {
			s.metrics.IncWorkflowWaitsTimedOut()
			s.metrics.IncWorkflowRunsFailed()
		}
		run, runErr := s.store.GetWorkflowRunByID(ctx, wait.WorkflowRunID)
		if runErr == nil {
			_ = s.emitWorkflowNodeEvent(ctx, run, workflow.RunStep{WorkflowNodeID: wait.WorkflowNodeID, NodeType: workflow.NodeTypeWait}, "timed_out", nil, nil)
		}
	}
	return nil
}

func (s *Service) startRunsForEvent(ctx context.Context, evt eventpkg.Event) error {
	bindings, err := s.store.ListActiveWorkflowEntryBindingsByEvent(ctx, evt.TenantID, evt.Type, evt.SourceIntegration())
	if err != nil {
		return fmt.Errorf("list active workflow entry bindings: %w", err)
	}
	for _, binding := range bindings {
		if binding.ConnectionID != nil && (evt.Source.ConnectionID == nil || *binding.ConnectionID != *evt.Source.ConnectionID) {
			continue
		}
		if len(binding.FilterJSON) > 0 {
			matched, err := subscriptionfilter.Evaluate(binding.FilterJSON, evt)
			if err != nil || !matched {
				continue
			}
		}
		run, _, err := s.store.CreateWorkflowRun(ctx, workflow.RunRecord{
			ID:                      uuid.New(),
			WorkflowID:              binding.WorkflowID,
			WorkflowVersionID:       binding.WorkflowVersionID,
			TenantID:                evt.TenantID,
			TriggerEventID:          evt.EventID,
			Status:                  workflow.RunStatusRunning,
			RootWorkflowNodeID:      binding.WorkflowNodeID,
			TriggeredByEventType:    evt.Type,
			TriggeredByConnectionID: evt.Source.ConnectionID,
			StartedAt:               s.now(),
		}, workflow.RunStepRecord{
			ID:             uuid.New(),
			WorkflowNodeID: binding.WorkflowNodeID,
			NodeType:       workflow.NodeTypeTrigger,
			Status:         workflow.RunStepStatusSucceeded,
			InputEventID:   &evt.EventID,
			OutputEventID:  &evt.EventID,
			StartedAt:      s.now(),
			CompletedAt:    timePtr(s.now()),
		})
		if err != nil {
			return fmt.Errorf("create workflow run: %w", err)
		}
		if s.metrics != nil {
			s.metrics.IncWorkflowRunsStarted()
		}
		_ = s.emitWorkflowNodeEvent(ctx, run, workflow.RunStep{WorkflowNodeID: binding.WorkflowNodeID, NodeType: workflow.NodeTypeTrigger}, "started", &evt, nil)
		if err := s.advanceDirect(ctx, run, binding.WorkflowNodeID, evt, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processWorkflowEvent(ctx context.Context, evt eventpkg.Event) error {
	if evt.SourceIntegration() == workflowIntegration {
		return nil
	}
	run, err := s.store.GetWorkflowRunByID(ctx, *evt.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("get workflow run for event: %w", err)
	}
	if strings.TrimSpace(evt.WorkflowNodeID) == "" {
		return nil
	}
	return s.advanceDirect(ctx, run, evt.WorkflowNodeID, evt, false)
}

func (s *Service) resumeWaitsForEvent(ctx context.Context, evt eventpkg.Event) error {
	keys := correlationCandidates(evt)
	for strategy, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		waits, err := s.store.ListMatchingWorkflowRunWaits(ctx, evt.TenantID, evt.Type, evt.SourceIntegration(), key)
		if err != nil {
			return fmt.Errorf("list matching workflow waits: %w", err)
		}
		for _, wait := range waits {
			if wait.CorrelationStrategy != strategy {
				continue
			}
			if err := s.store.MatchWorkflowRunWait(ctx, wait.ID, evt.EventID, s.now()); err != nil {
				return fmt.Errorf("match workflow wait: %w", err)
			}
			run, err := s.store.GetWorkflowRunByID(ctx, wait.WorkflowRunID)
			if err != nil {
				return fmt.Errorf("get workflow run for wait match: %w", err)
			}
			if err := s.store.SetWorkflowRunStatus(ctx, run.ID, workflow.RunStatusRunning, nil, nil); err != nil {
				return fmt.Errorf("set workflow run status running: %w", err)
			}
			steps, err := s.store.ListWorkflowRunSteps(ctx, tenant.ID(run.TenantID), run.ID)
			if err == nil {
				for _, step := range steps {
					if step.WorkflowNodeID == wait.WorkflowNodeID && step.Status == workflow.RunStepStatusWaiting {
						_ = s.store.CompleteWorkflowRunStep(ctx, step.ID, workflow.RunStepStatusSucceeded, s.now(), &evt.EventID, nil, nil, nil, nil)
					}
				}
			}
			if s.metrics != nil {
				s.metrics.IncWorkflowWaitsMatched()
			}
			_ = s.emitWorkflowNodeEvent(ctx, run, workflow.RunStep{WorkflowNodeID: wait.WorkflowNodeID, NodeType: workflow.NodeTypeWait}, "matched", &evt, nil)
			if err := s.advanceDirect(ctx, run, wait.WorkflowNodeID, evt, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) advanceDirect(ctx context.Context, run workflow.Run, sourceNodeID string, inputEvent eventpkg.Event, allowDeliveries bool) error {
	version, compiled, err := s.loadCompiled(ctx, run.WorkflowVersionID)
	if err != nil {
		return err
	}
	nodeByID := make(map[string]workflowcompiler.CompiledNodeBinding, len(compiled.NodeBindings))
	for _, binding := range compiled.NodeBindings {
		nodeByID[binding.NodeID] = binding
	}
	outgoing := make(map[string][]string)
	for _, edge := range compiled.RuntimeEdges {
		outgoing[edge.SourceNodeID] = append(outgoing[edge.SourceNodeID], edge.TargetNodeID)
	}
	visited := map[string]bool{}
	return s.followTargets(ctx, run, version, nodeByID, outgoing, sourceNodeID, inputEvent, visited, allowDeliveries)
}

func (s *Service) followTargets(ctx context.Context, run workflow.Run, version workflow.Version, nodes map[string]workflowcompiler.CompiledNodeBinding, outgoing map[string][]string, sourceNodeID string, inputEvent eventpkg.Event, visited map[string]bool, allowDeliveries bool) error {
	for _, targetNodeID := range outgoing[sourceNodeID] {
		binding, ok := nodes[targetNodeID]
		if !ok {
			continue
		}
		switch binding.NodeType {
		case workflow.NodeTypeCondition:
			raw := json.RawMessage(strings.TrimSpace(binding.Expression))
			matched, err := subscriptionfilter.Evaluate(raw, inputEvent)
			if err != nil {
				return fmt.Errorf("evaluate condition %s: %w", binding.NodeID, err)
			}
			status := workflow.RunStepStatusSkipped
			if matched {
				status = workflow.RunStepStatusSucceeded
			}
			branchKey := "skipped"
			if matched {
				branchKey = "matched"
			}
			step, err := s.store.CreateWorkflowRunStep(ctx, workflow.RunStepRecord{
				ID:             uuid.New(),
				WorkflowRunID:  run.ID,
				WorkflowNodeID: binding.NodeID,
				NodeType:       workflow.NodeTypeCondition,
				Status:         status,
				BranchKey:      stringPtr(branchKey),
				InputEventID:   &inputEvent.EventID,
				StartedAt:      s.now(),
				CompletedAt:    timePtr(s.now()),
			})
			if err != nil {
				return fmt.Errorf("create condition step: %w", err)
			}
			_ = s.emitWorkflowNodeEvent(ctx, run, step, branchKey, &inputEvent, nil)
			if matched && !visited[binding.NodeID] {
				visited[binding.NodeID] = true
				if err := s.followTargets(ctx, run, version, nodes, outgoing, binding.NodeID, inputEvent, visited, allowDeliveries); err != nil {
					return err
				}
			}
		case workflow.NodeTypeAction, workflow.NodeTypeAgent:
			if !allowDeliveries {
				continue
			}
			artifact, err := s.store.GetActiveWorkflowSubscriptionArtifactByNode(ctx, tenant.ID(run.TenantID), version.ID, binding.NodeID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				return fmt.Errorf("get workflow subscription artifact: %w", err)
			}
			jobID := uuid.New()
			step, err := s.store.CreateWorkflowRunStep(ctx, workflow.RunStepRecord{
				ID:             uuid.New(),
				WorkflowRunID:  run.ID,
				WorkflowNodeID: binding.NodeID,
				NodeType:       binding.NodeType,
				Status:         workflow.RunStepStatusRunning,
				InputEventID:   &inputEvent.EventID,
				SubscriptionID: &artifact.SubscriptionID,
				StartedAt:      s.now(),
			})
			if err != nil {
				return fmt.Errorf("create workflow action step: %w", err)
			}
			created, err := s.store.CreateDeliveryJob(ctx, delivery.JobRecord{
				ID:             jobID,
				TenantID:       run.TenantID,
				SubscriptionID: artifact.SubscriptionID,
				EventID:        inputEvent.EventID,
				WorkflowRunID:  &run.ID,
				WorkflowNodeID: binding.NodeID,
				Status:         delivery.StatusPending,
				CreatedAt:      s.now(),
			})
			if err != nil {
				return fmt.Errorf("create workflow delivery job: %w", err)
			}
			if !created {
				_ = s.store.CompleteWorkflowRunStep(ctx, step.ID, workflow.RunStepStatusSkipped, s.now(), nil, nil, nil, nil, nil)
				continue
			}
			if err := s.store.AttachWorkflowRunStepDelivery(ctx, step.ID, jobID); err != nil {
				return fmt.Errorf("attach workflow delivery job: %w", err)
			}
			_ = s.emitWorkflowNodeEvent(ctx, run, step, "started", &inputEvent, nil)
		case workflow.NodeTypeWait:
			correlationKey, err := deriveCorrelationKey(binding.CorrelationStrategy, inputEvent)
			if err != nil {
				return err
			}
			step, err := s.store.CreateWorkflowRunStep(ctx, workflow.RunStepRecord{
				ID:             uuid.New(),
				WorkflowRunID:  run.ID,
				WorkflowNodeID: binding.NodeID,
				NodeType:       workflow.NodeTypeWait,
				Status:         workflow.RunStepStatusWaiting,
				InputEventID:   &inputEvent.EventID,
				StartedAt:      s.now(),
			})
			if err != nil {
				return fmt.Errorf("create workflow wait step: %w", err)
			}
			expiresAt, err := parseTimeoutDeadline(binding.Timeout, s.now())
			if err != nil {
				return err
			}
			if _, err := s.store.CreateWorkflowRunWait(ctx, workflow.RunWaitRecord{
				ID:                  uuid.New(),
				WorkflowRunID:       run.ID,
				WorkflowVersionID:   version.ID,
				WorkflowNodeID:      binding.NodeID,
				Status:              workflow.RunWaitStatusWaiting,
				ExpectedEventType:   binding.ExpectedEventType,
				ExpectedIntegration: binding.ExpectedIntegration,
				CorrelationStrategy: binding.CorrelationStrategy,
				CorrelationKey:      correlationKey,
				ExpiresAt:           expiresAt,
				CreatedAt:           s.now(),
			}); err != nil {
				return fmt.Errorf("create workflow wait: %w", err)
			}
			if err := s.store.SetWorkflowRunStatus(ctx, run.ID, workflow.RunStatusWaiting, nil, nil); err != nil {
				return fmt.Errorf("set workflow run waiting: %w", err)
			}
			if s.metrics != nil {
				s.metrics.IncWorkflowRunsWaiting()
				s.metrics.IncWorkflowWaitsRegistered()
			}
			_ = s.emitWorkflowNodeEvent(ctx, run, step, "waiting", &inputEvent, map[string]any{"correlation_key": correlationKey})
		case workflow.NodeTypeEnd:
			status := terminalRunStatus(binding.TerminalStatus)
			step, err := s.store.CreateWorkflowRunStep(ctx, workflow.RunStepRecord{
				ID:             uuid.New(),
				WorkflowRunID:  run.ID,
				WorkflowNodeID: binding.NodeID,
				NodeType:       workflow.NodeTypeEnd,
				Status:         workflow.RunStepStatusSucceeded,
				InputEventID:   &inputEvent.EventID,
				StartedAt:      s.now(),
				CompletedAt:    timePtr(s.now()),
			})
			if err != nil {
				return fmt.Errorf("create workflow end step: %w", err)
			}
			completedAt := s.now()
			if err := s.store.SetWorkflowRunStatus(ctx, run.ID, status, &completedAt, nil); err != nil {
				return fmt.Errorf("set workflow run terminal status: %w", err)
			}
			if s.metrics != nil {
				s.metrics.IncWorkflowRunsCompleted()
			}
			_ = s.emitWorkflowNodeEvent(ctx, run, step, "completed", &inputEvent, map[string]any{"terminal_status": status})
		}
	}
	return nil
}

func (s *Service) loadCompiled(ctx context.Context, versionID uuid.UUID) (workflow.Version, workflowcompiler.CompiledWorkflow, error) {
	version, err := s.store.GetWorkflowVersionCompiledByID(ctx, versionID)
	if err != nil {
		return workflow.Version{}, workflowcompiler.CompiledWorkflow{}, fmt.Errorf("get workflow version: %w", err)
	}
	var compiled workflowcompiler.CompiledWorkflow
	if err := json.Unmarshal(version.CompiledJSON, &compiled); err != nil {
		return workflow.Version{}, workflowcompiler.CompiledWorkflow{}, fmt.Errorf("decode compiled workflow: %w", err)
	}
	return version, compiled, nil
}

func (s *Service) emitWorkflowNodeEvent(ctx context.Context, run workflow.Run, step workflow.RunStep, status string, inputEvent *eventpkg.Event, extra map[string]any) error {
	if s.ingest == nil {
		return nil
	}
	payload := map[string]any{
		"workflow_id":         run.WorkflowID.String(),
		"workflow_version_id": run.WorkflowVersionID.String(),
		"workflow_run_id":     run.ID.String(),
		"workflow_node_id":    step.WorkflowNodeID,
		"node_type":           step.NodeType,
		"status":              status,
	}
	if inputEvent != nil {
		payload["input_event_id"] = inputEvent.EventID.String()
	}
	for key, value := range extra {
		payload[key] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var lineage *eventpkg.Lineage
	chainDepth := 0
	if inputEvent != nil {
		lineage = inheritedLineage(*inputEvent)
		chainDepth = inputEvent.ChainDepth + 1
	}
	eventType := fmt.Sprintf("workflow.node.%s.v1", strings.TrimSpace(status))
	_, err = s.ingest.Ingest(ctx, ingest.Request{
		TenantID:       tenant.ID(run.TenantID),
		WorkflowRunID:  &run.ID,
		WorkflowNodeID: step.WorkflowNodeID,
		Type:           eventType,
		Source:         workflowIntegration,
		SourceInfo: eventpkg.Source{
			Kind:        eventpkg.SourceKindInternal,
			Integration: workflowIntegration,
		},
		SourceKind: eventpkg.SourceKindInternal,
		Lineage:    lineage,
		ChainDepth: chainDepth,
		Payload:    body,
	})
	return err
}

func inheritedLineage(input eventpkg.Event) *eventpkg.Lineage {
	if input.Lineage != nil {
		return eventpkg.NormalizeLineage(input.Lineage)
	}
	if input.Source.Kind != eventpkg.SourceKindExternal {
		return nil
	}
	return eventpkg.NormalizeLineage(&eventpkg.Lineage{
		Integration:       input.Source.Integration,
		ConnectionID:      input.Source.ConnectionID,
		ConnectionName:    input.Source.ConnectionName,
		ExternalAccountID: input.Source.ExternalAccountID,
	})
}

func deriveCorrelationKey(strategy string, evt eventpkg.Event) (string, error) {
	switch trimmed := strings.TrimSpace(strategy); {
	case trimmed == "event_id":
		return evt.EventID.String(), nil
	case strings.HasPrefix(trimmed, "payload."):
		value, ok := eventpkg.BuildTemplateReplacements(evt)["{{"+trimmed+"}}"]
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("workflow wait correlation strategy %q resolved empty", trimmed)
		}
		return value, nil
	case trimmed == "source.connection_id":
		if evt.Source.ConnectionID == nil {
			return "", fmt.Errorf("workflow wait correlation strategy %q resolved empty", trimmed)
		}
		return evt.Source.ConnectionID.String(), nil
	default:
		return "", fmt.Errorf("unsupported workflow wait correlation strategy: %s", trimmed)
	}
}

func correlationCandidates(evt eventpkg.Event) map[string]string {
	candidates := map[string]string{
		"event_id": evt.EventID.String(),
	}
	if evt.Source.ConnectionID != nil {
		candidates["source.connection_id"] = evt.Source.ConnectionID.String()
	}
	var payload any
	if err := json.Unmarshal(evt.Payload, &payload); err == nil {
		flattenPayload("", payload, candidates)
	}
	return candidates
}

func flattenPayload(prefix string, value any, out map[string]string) {
	object, ok := value.(map[string]any)
	if !ok {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(prefix) != "" {
				out["payload."+prefix] = typed
			}
		case float64, bool:
			if strings.TrimSpace(prefix) != "" {
				out["payload."+prefix] = fmt.Sprint(typed)
			}
		}
		return
	}
	for key, nested := range object {
		current := key
		if prefix != "" {
			current = prefix + "." + key
		}
		flattenPayload(current, nested, out)
	}
}

func parseTimeoutDeadline(raw json.RawMessage, now time.Time) (*time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode workflow wait timeout: %w", err)
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse workflow wait timeout: %w", err)
	}
	deadline := now.Add(duration)
	return &deadline, nil
}

func terminalRunStatus(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case workflow.RunStatusPartial:
		return workflow.RunStatusPartial
	case workflow.RunStatusCancelled:
		return workflow.RunStatusCancelled
	default:
		return workflow.RunStatusSucceeded
	}
}

func workflowNodeTypeForSubscription(sub subscription.Subscription) string {
	if sub.AgentID != nil {
		return workflow.NodeTypeAgent
	}
	return workflow.NodeTypeAction
}

func normalizeRunLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func timePtr(value time.Time) *time.Time { return &value }
func stringPtr(value string) *string     { return &value }
