package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/temporal/activities"
)

const AgentWorkflowName = "agent_workflow"

type AgentRequest struct {
	TenantID        string
	SubscriptionID  string
	Event           activities.Event
	AgentID         string
	AgentVersionID  string
	SessionKey      string
	CreateIfMissing bool
}

type AgentToolSummary struct {
	Tool string `json:"tool"`
	OK   bool   `json:"ok"`
}

type AgentResult struct {
	Output         []byte
	ToolCalls      []AgentToolSummary
	AgentRunID     string
	AgentID        string
	AgentSessionID string
	SessionKey     string
}

func AgentWorkflow(ctx workflow.Context, req AgentRequest) (AgentResult, error) {
	logger := workflow.GetLogger(ctx)
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})

	var agentRunID string
	if err := workflow.ExecuteActivity(ctx, "StartAgentRun", req.TenantID, req.Event.EventID, req.SubscriptionID, req.Event.WorkflowRunID, req.Event.WorkflowNodeID, req.AgentVersionID).Get(ctx, &agentRunID); err != nil {
		return AgentResult{}, err
	}

	failRun := func(steps int, message string) error {
		_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, steps, message).Get(ctx, nil)
		return temporal.NewNonRetryableApplicationError(message, "agent_failed", nil)
	}

	var definition activities.AgentDefinition
	if err := workflow.ExecuteActivity(ctx, "LoadAgent", req.TenantID, req.AgentID).Get(ctx, &definition); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("load agent: %v", err))
	}

	var resolution activities.AgentSessionResolution
	if err := workflow.ExecuteActivity(ctx, "ResolveAgentSession", req.TenantID, req.AgentID, req.SessionKey, req.CreateIfMissing).Get(ctx, &resolution); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("resolve session: %v", err))
	}
	if err := workflow.ExecuteActivity(ctx, "AttachAgentRunContext", agentRunID, req.AgentID, resolution.Session.ID).Get(ctx, nil); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("attach agent run context: %v", err))
	}
	if err := workflow.ExecuteActivity(ctx, "LinkAgentSessionEvent", resolution.Session.ID, req.Event.EventID).Get(ctx, nil); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("link session event: %v", err))
	}

	logger.Info("agent_runtime_called",
		"tenant_id", req.TenantID,
		"agent_id", req.AgentID,
		"agent_session_id", resolution.Session.ID,
		"session_key", resolution.Session.SessionKey,
		"event_id", req.Event.EventID,
		"agent_run_id", agentRunID,
	)

	var runtimeResult activities.AgentRuntimeCallResult
	if err := workflow.ExecuteActivity(ctx, "RunAgentRuntime", activities.AgentRuntimeCallRequest{
		AgentRunID: agentRunID,
		TenantID:   req.TenantID,
		Agent:      definition,
		Session:    resolution.Session,
		Event:      req.Event,
	}).Get(ctx, &runtimeResult); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("run agent runtime: %v", err))
	}

	if err := workflow.ExecuteActivity(ctx, "RecordAgentRuntimeSteps", agentRunID, runtimeResult.Usage, runtimeResult.ToolCalls).Get(ctx, nil); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("record runtime steps: %v", err))
	}
	if err := workflow.ExecuteActivity(ctx, "UpdateAgentSessionAfterRun", resolution.Session.ID, req.Event.EventID, runtimeResult.SessionSummary).Get(ctx, nil); err != nil {
		return AgentResult{}, failRun(0, fmt.Sprintf("update session: %v", err))
	}
	steps := 1 + len(runtimeResult.ToolCalls)
	if err := workflow.ExecuteActivity(ctx, "CompleteAgentRun", agentRunID, steps).Get(ctx, nil); err != nil {
		return AgentResult{}, err
	}

	toolCalls := make([]AgentToolSummary, 0, len(runtimeResult.ToolCalls))
	for _, call := range runtimeResult.ToolCalls {
		toolCalls = append(toolCalls, AgentToolSummary{Tool: call.Tool, OK: call.OK})
	}
	return AgentResult{
		Output:         runtimeResult.Output,
		ToolCalls:      toolCalls,
		AgentRunID:     agentRunID,
		AgentID:        definition.ID,
		AgentSessionID: resolution.Session.ID,
		SessionKey:     resolution.Session.SessionKey,
	}, nil
}
