package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	agenttools "groot/internal/agent/tools"
	"groot/internal/connection"
	"groot/internal/connectors/outbound"
	eventpkg "groot/internal/event"
)

type AgentToolTarget struct {
	RequestedName         string
	DefinitionName        string
	ExecutionKind         string
	IntegrationName       string
	Operation             string
	FunctionDestinationID string
}

type AgentToolExecutionRequest struct {
	DeliveryJobID string
	TenantID      string
	Event         Event
	Target        AgentToolTarget
	Arguments     json.RawMessage
	Attempt       int
}

type AgentToolExecutionResult struct {
	Tool        string          `json:"tool"`
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ExternalID  string          `json:"external_id,omitempty"`
	StatusCode  int             `json:"status_code,omitempty"`
	Integration string          `json:"integration,omitempty"`
	Model       string          `json:"model,omitempty"`
	Usage       outbound.Usage  `json:"usage"`
}

func (a *Activities) StartAgentRun(ctx context.Context, tenantID string, inputEventID string, subscriptionID string, workflowRunID string, workflowNodeID string, agentVersionID string) (string, error) {
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return "", err
	}
	eventID, err := uuid.Parse(inputEventID)
	if err != nil {
		return "", err
	}
	subID, err := uuid.Parse(subscriptionID)
	if err != nil {
		return "", err
	}
	run, err := a.store.CreateAgentRun(ctx, agent.RunRecord{
		ID:             uuid.New(),
		TenantID:       tID,
		InputEventID:   eventID,
		SubscriptionID: subID,
		WorkflowRunID:  optionalParsedUUID(workflowRunID),
		WorkflowNodeID: optionalStringPtr(workflowNodeID),
		AgentVersionID: optionalParsedUUID(agentVersionID),
		Status:         agent.StatusRunning,
		Steps:          0,
		StartedAt:      time.Now().UTC(),
	})
	if err != nil {
		return "", err
	}
	if a.metrics != nil {
		a.metrics.IncAgentRuns()
	}
	if a.logger != nil {
		a.logger.Info("agent_run_started",
			slog.String("agent_run_id", run.ID.String()),
			slog.String("tenant_id", tenantID),
			slog.String("input_event_id", inputEventID),
			slog.String("subscription_id", subscriptionID),
		)
	}
	return run.ID.String(), nil
}

func (a *Activities) RecordAgentLLMStep(ctx context.Context, agentRunID string, stepNum int, integration string, model string, usage outbound.Usage) error {
	runID, err := uuid.Parse(agentRunID)
	if err != nil {
		return err
	}
	usageBody, err := json.Marshal(map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	})
	if err != nil {
		return fmt.Errorf("marshal agent llm usage: %w", err)
	}
	if err := a.store.CreateAgentStep(ctx, agent.StepRecord{
		ID:             uuid.New(),
		AgentRunID:     runID,
		StepNum:        stepNum,
		Kind:           agent.StepKindLLMCall,
		LLMIntegration: optionalStringPtr(integration),
		LLMModel:       optionalStringPtr(model),
		Usage:          usageBody,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncAgentSteps()
	}
	if a.logger != nil {
		a.logger.Info("agent_step_llm_call",
			slog.String("agent_run_id", agentRunID),
			slog.Int("step", stepNum),
			slog.String("integration", integration),
			slog.String("model", model),
		)
	}
	return nil
}

func (a *Activities) RecordAgentToolStep(ctx context.Context, agentRunID string, stepNum int, toolName string, args json.RawMessage, result json.RawMessage) error {
	runID, err := uuid.Parse(agentRunID)
	if err != nil {
		return err
	}
	if err := a.store.CreateAgentStep(ctx, agent.StepRecord{
		ID:         uuid.New(),
		AgentRunID: runID,
		StepNum:    stepNum,
		Kind:       agent.StepKindToolCall,
		ToolName:   optionalStringPtr(toolName),
		ToolArgs:   args,
		ToolResult: result,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncAgentSteps()
		a.metrics.IncAgentToolCalls()
	}
	if a.logger != nil {
		a.logger.Info("agent_step_tool_call",
			slog.String("agent_run_id", agentRunID),
			slog.Int("step", stepNum),
			slog.String("tool", toolName),
		)
	}
	return nil
}

func (a *Activities) CompleteAgentRun(ctx context.Context, agentRunID string, steps int) error {
	runID, err := uuid.Parse(agentRunID)
	if err != nil {
		return err
	}
	if err := a.store.MarkAgentRunSucceeded(ctx, runID, steps, time.Now().UTC()); err != nil {
		return err
	}
	if a.logger != nil {
		a.logger.Info("agent_run_succeeded",
			slog.String("agent_run_id", agentRunID),
			slog.Int("steps", steps),
		)
	}
	return nil
}

func (a *Activities) FailAgentRun(ctx context.Context, agentRunID string, steps int, lastError string) error {
	runID, err := uuid.Parse(agentRunID)
	if err != nil {
		return err
	}
	if err := a.store.MarkAgentRunFailed(ctx, runID, steps, time.Now().UTC(), lastError); err != nil {
		return err
	}
	if a.logger != nil {
		a.logger.Info("agent_run_failed",
			slog.String("agent_run_id", agentRunID),
			slog.Int("steps", steps),
			slog.String("error", lastError),
		)
	}
	return nil
}

func (a *Activities) ExecuteAgentTool(ctx context.Context, req AgentToolExecutionRequest) (AgentToolExecutionResult, error) {
	registry, err := agenttools.NewDefaultRegistry()
	if err != nil {
		return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: fmt.Errorf("load agent tool registry: %w", err)})
	}
	if err := registry.Validate(req.Target.DefinitionName, req.Arguments); err != nil {
		return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: err})
	}
	switch req.Target.ExecutionKind {
	case "connector":
		instance, err := a.resolveToolConnector(ctx, req.TenantID, req.Target.IntegrationName)
		if err != nil {
			return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: err})
		}
		result, err := a.ExecuteConnection(ctx, req.DeliveryJobID, req.TenantID, req.Event, instance, req.Target.Operation, req.Arguments, req.Attempt)
		if err != nil {
			return AgentToolExecutionResult{}, err
		}
		return AgentToolExecutionResult{
			Tool:        req.Target.RequestedName,
			OK:          true,
			Result:      normalizeAgentConnectionResult(result),
			ExternalID:  result.ExternalID,
			StatusCode:  result.StatusCode,
			Integration: result.Integration,
			Model:       result.Model,
			Usage:       result.Usage,
		}, nil
	case "function":
		destination, err := a.LoadFunctionDestination(ctx, req.Target.FunctionDestinationID, req.TenantID)
		if err != nil {
			return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: fmt.Errorf("load function destination: %w", err)})
		}
		toolEvent, err := buildAgentFunctionEvent(req.Event, req.Target.RequestedName, req.Arguments)
		if err != nil {
			return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: err})
		}
		result, err := a.InvokeFunction(ctx, req.DeliveryJobID, destination.ID, toolEvent, destination.URL, destination.Secret, destination.TimeoutSeconds, req.Attempt)
		if err != nil {
			return AgentToolExecutionResult{}, err
		}
		output, _ := json.Marshal(map[string]any{
			"response_status":      result.StatusCode,
			"response_body_sha256": result.ResponseBodySHA,
		})
		return AgentToolExecutionResult{
			Tool:       req.Target.RequestedName,
			OK:         true,
			Result:     output,
			StatusCode: result.StatusCode,
		}, nil
	default:
		return AgentToolExecutionResult{}, wrapActivityError(outbound.PermanentError{Err: fmt.Errorf("unsupported agent tool execution kind: %s", req.Target.ExecutionKind)})
	}
}

func (a *Activities) resolveToolConnector(ctx context.Context, tenantID string, connectorName string) (Connection, error) {
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return Connection{}, err
	}
	instance, err := a.store.GetTenantConnectionByName(ctx, tID, connectorName)
	if err == nil {
		return Connection{
			ID:              instance.ID.String(),
			IntegrationName: instance.IntegrationName,
			Scope:           instance.Scope,
			Config:          instance.Config,
		}, nil
	}
	if !errors.Is(err, connection.ErrNotFound) {
		return Connection{}, err
	}
	instance, err = a.store.GetGlobalConnectionByName(ctx, connectorName)
	if err != nil {
		return Connection{}, err
	}
	return Connection{
		ID:              instance.ID.String(),
		IntegrationName: instance.IntegrationName,
		Scope:           instance.Scope,
		Config:          instance.Config,
	}, nil
}

func normalizeAgentConnectionResult(result ConnectionResult) json.RawMessage {
	if len(result.Output) > 0 {
		return result.Output
	}
	output, _ := json.Marshal(map[string]any{
		"external_id": result.ExternalID,
		"status_code": result.StatusCode,
	})
	return output
}

func buildAgentFunctionEvent(event Event, toolName string, args json.RawMessage) (Event, error) {
	var arguments any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return Event{}, fmt.Errorf("decode function tool args: %w", err)
		}
	}
	payload, err := json.Marshal(map[string]any{
		"tool":        toolName,
		"arguments":   arguments,
		"input_event": event,
	})
	if err != nil {
		return Event{}, fmt.Errorf("marshal function tool payload: %w", err)
	}
	return Event{
		EventID:    event.EventID,
		TenantID:   event.TenantID,
		Type:       "llm.agent.tool_call.v1",
		Source:     eventpkg.Source{Kind: eventpkg.SourceKindInternal, Integration: "llm"},
		SourceKind: eventpkg.SourceKindInternal,
		Lineage:    event.Lineage,
		ChainDepth: event.ChainDepth,
		Timestamp:  event.Timestamp,
		Payload:    payload,
	}, nil
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
