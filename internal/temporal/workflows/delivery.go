package workflows

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/agent"
	"groot/internal/config"
	eventpkg "groot/internal/event"
	"groot/internal/integrations/llm"
	"groot/internal/integrations/notion"
	"groot/internal/integrations/registry"
	"groot/internal/integrations/resend"
	"groot/internal/integrations/slack"
	"groot/internal/temporal/activities"
)

const (
	WorkflowName         = "delivery_workflow"
	DefaultTaskQueueName = "groot-delivery"
)

func DeliveryWorkflow(ctx workflow.Context, deliveryJobID string, maxAttempts int, agentConfig config.AgentRuntimeConfig) error {
	logger := workflow.GetLogger(ctx)
	info := workflow.GetInfo(ctx)
	attempt := int(info.Attempt)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var job activities.DeliveryJob
	if err := workflow.ExecuteActivity(ctx, "LoadDeliveryJob", deliveryJobID).Get(ctx, &job); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load delivery job: %v", err), nil, activities.DeliveryJob{}, activities.Subscription{}, activities.Event{}, "", "")
	}

	logger.Info("delivery_attempt", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "RecordAttempt", deliveryJobID, attempt).Get(ctx, nil); err != nil {
		return fmt.Errorf("record attempt: %w", err)
	}

	var sub activities.Subscription
	if err := workflow.ExecuteActivity(ctx, "LoadSubscription", job.SubscriptionID).Get(ctx, &sub); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load subscription: %v", err), nil, job, activities.Subscription{}, activities.Event{}, "", "")
	}

	var event activities.Event
	if err := workflow.ExecuteActivity(ctx, "LoadEvent", job.EventID).Get(ctx, &event); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load event: %v", err), nil, job, sub, activities.Event{}, "", "")
	}

	var err error
	var externalID *string
	var lastStatusCode *int
	var connectorName string
	var operation string
	var successOutput []byte
	var successToolCalls []byte
	var agentID string
	var agentSessionID string
	var sessionKey string
	switch sub.DestinationType {
	case "webhook":
		var app activities.ConnectedApp
		if loadErr := workflow.ExecuteActivity(ctx, "LoadConnectedApp", sub.ConnectedAppID, job.TenantID).Get(ctx, &app); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connected app: %v", loadErr), nil, job, sub, event, "", "")
		}
		err = workflow.ExecuteActivity(ctx, activities.DeliverHTTPName, app.DestinationURL, event).Get(ctx, nil)
	case "function":
		connectorName = "function"
		operation = "invoke"
		var destination activities.FunctionDestination
		if loadErr := workflow.ExecuteActivity(ctx, "LoadFunctionDestination", sub.FunctionDestinationID, job.TenantID).Get(ctx, &destination); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load function destination: %v", loadErr), nil, job, sub, event, connectorName, operation)
		}
		var result activities.FunctionResult
		err = workflow.ExecuteActivity(ctx, activities.InvokeFunctionName, job.ID, destination.ID, event, destination.URL, destination.Secret, destination.TimeoutSeconds, attempt).Get(ctx, &result)
		if err == nil {
			statusCode := result.StatusCode
			lastStatusCode = &statusCode
			successOutput, _ = marshalOutput(map[string]any{
				"response_status":      result.StatusCode,
				"response_body_sha256": result.ResponseBodySHA,
			})
		}
	case "connection":
		connectorName = llm.IntegrationName
		operation = sub.Operation
		if sub.Operation == llm.OperationAgent {
			agentID = sub.AgentID
			sourceEvent := activityEventToStreamEvent(event)
			sessionKey = agent.ResolveSessionKey(sub.SessionKeyTemplate, sourceEvent)
			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowExecutionTimeout: agentConfig.Timeout,
			})
			var result AgentResult
			err = workflow.ExecuteChildWorkflow(childCtx, AgentWorkflowName, AgentRequest{
				TenantID:        job.TenantID,
				SubscriptionID:  sub.ID,
				Event:           event,
				AgentID:         sub.AgentID,
				AgentVersionID:  sub.AgentVersionID,
				SessionKey:      sessionKey,
				CreateIfMissing: sub.SessionCreateIfMissing,
			}).Get(childCtx, &result)
			if err == nil {
				successOutput = result.Output
				successToolCalls, _ = json.Marshal(result.ToolCalls)
				agentSessionID = result.AgentSessionID
				sessionKey = result.SessionKey
			}
		} else {
			resolvedConnectionID := sub.ConnectionID
			if strings.TrimSpace(resolvedConnectionID) == "" {
				resolvedConnectionID = defaultConnectionIDForEvent(sub.Operation, event)
			}
			if strings.TrimSpace(resolvedConnectionID) == "" {
				return nonRetryableTerminal(ctx, deliveryJobID, attempt, "load connection: no default connection available", nil, job, sub, event, connectorName, operation)
			}
			var instance activities.Connection
			if loadErr := workflow.ExecuteActivity(ctx, "LoadConnection", resolvedConnectionID, job.TenantID).Get(ctx, &instance); loadErr != nil {
				return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connection: %v", loadErr), nil, job, sub, event, connectorName, operation)
			}
			connectorName = instance.IntegrationName
			var result activities.ConnectionResult
			err = workflow.ExecuteActivity(ctx, activities.ExecuteIntegrationName, job.ID, job.TenantID, event, instance, sub.Operation, sub.OperationParams, attempt).Get(ctx, &result)
			if err == nil {
				if result.ExternalID != "" {
					externalID = &result.ExternalID
				}
				if result.StatusCode != 0 {
					lastStatusCode = &result.StatusCode
				}
				successOutput, _ = connectorOutput(connectorName, operation, result)
			}
		}
	default:
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("unsupported destination type: %s", sub.DestinationType), nil, job, sub, event, connectorName, operation)
	}

	if err != nil {
		message := err.Error()
		lastStatusCode = activities.ConnectorStatusCode(err)
		if activities.IsPermanentError(err) {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, message, lastStatusCode, job, sub, event, connectorName, operation)
		}
		if attempt >= maxAttempts {
			logger.Info("delivery_dead_letter", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
			if markErr := workflow.ExecuteActivity(ctx, "MarkDeadLetter", deliveryJobID, message, connectorName, operation, job.TenantID, job.EventID, attempt, lastStatusCode).Get(ctx, nil); markErr != nil {
				return fmt.Errorf("mark dead letter: %w", markErr)
			}
			if shouldEmitFailure(sub, connectorName, operation) {
				_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, eventpkg.ResultStatusFailed, nil, nil, message, "dead_letter", externalID, lastStatusCode, agentID, agentSessionID, sessionKey)).Get(ctx, nil)
			}
			return nil
		}

		logger.Info("delivery_failed", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
		if markErr := workflow.ExecuteActivity(ctx, "MarkRetryableFailure", deliveryJobID, message, lastStatusCode).Get(ctx, nil); markErr != nil {
			return fmt.Errorf("mark retryable failure: %w", markErr)
		}
		return fmt.Errorf("deliver: %w", err)
	}

	logger.Info("delivery_succeeded", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "MarkSucceeded", deliveryJobID, externalID, lastStatusCode).Get(ctx, nil); err != nil {
		return fmt.Errorf("mark succeeded: %w", err)
	}
	if shouldEmitSuccess(sub, connectorName, operation) {
		_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, eventpkg.ResultStatusSucceeded, successOutput, successToolCalls, "", "", externalID, lastStatusCode, agentID, agentSessionID, sessionKey)).Get(ctx, nil)
	}
	return nil
}

func nonRetryableTerminal(ctx workflow.Context, deliveryJobID string, attempt int, message string, lastStatusCode *int, job activities.DeliveryJob, sub activities.Subscription, event activities.Event, connectorName string, operation string) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("delivery_failed", "delivery_job_id", deliveryJobID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "MarkFailed", deliveryJobID, message, lastStatusCode).Get(ctx, nil); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	if shouldEmitFailure(sub, connectorName, operation) {
		sessionKey := ""
		if connectorName == llm.IntegrationName && operation == llm.OperationAgent {
			sessionKey = agent.ResolveSessionKey(sub.SessionKeyTemplate, activityEventToStreamEvent(event))
		}
		_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, eventpkg.ResultStatusFailed, nil, nil, message, "failed", nil, lastStatusCode, sub.AgentID, "", sessionKey)).Get(ctx, nil)
	}
	return temporal.NewNonRetryableApplicationError(message, "delivery_failed", nil)
}

func shouldEmitSuccess(sub activities.Subscription, connectorName, operation string) bool {
	return sub.EmitSuccessEvent && connectorName != "" && operation != ""
}

func shouldEmitFailure(sub activities.Subscription, connectorName, operation string) bool {
	return sub.EmitFailureEvent && connectorName != "" && operation != ""
}

func buildResultEventRequest(job activities.DeliveryJob, sub activities.Subscription, event activities.Event, connectorName, operation, status string, output []byte, toolCalls []byte, errorMessage, errorType string, externalID *string, httpStatus *int, agentID, agentSessionID, sessionKey string) activities.ResultEventRequest {
	return activities.ResultEventRequest{
		DeliveryJobID:         job.ID,
		TenantID:              job.TenantID,
		SubscriptionID:        sub.ID,
		InputEventID:          job.EventID,
		ExistingResultEventID: job.ResultEventID,
		WorkflowRunID:         job.WorkflowRunID,
		WorkflowNodeID:        job.WorkflowNodeID,
		InputEvent:            event,
		IntegrationName:       connectorName,
		Operation:             operation,
		Status:                status,
		ExternalID:            externalID,
		HTTPStatusCode:        httpStatus,
		Output:                output,
		ToolCalls:             toolCalls,
		ErrorMessage:          errorMessage,
		ErrorType:             errorType,
		AgentID:               agentID,
		AgentSessionID:        agentSessionID,
		SessionKey:            sessionKey,
	}
}

func connectorOutput(connectorName, operation string, result activities.ConnectionResult) ([]byte, error) {
	if len(result.Output) > 0 && connectorName != llm.IntegrationName {
		return result.Output, nil
	}
	var payload map[string]any
	switch connectorName {
	case llm.IntegrationName:
		switch operation {
		case llm.OperationGenerate, llm.OperationSummarize:
			if len(result.Output) > 0 {
				if err := json.Unmarshal(result.Output, &payload); err != nil {
					return nil, err
				}
			} else {
				payload = map[string]any{"text": result.Text}
			}
			payload["integration"] = result.Integration
			payload["model"] = result.Model
			payload["usage"] = map[string]any{
				"prompt_tokens":     result.Usage.PromptTokens,
				"completion_tokens": result.Usage.CompletionTokens,
				"total_tokens":      result.Usage.TotalTokens,
			}
		case llm.OperationClassify, llm.OperationExtract, llm.OperationAgent:
			if len(result.Output) > 0 {
				return result.Output, nil
			}
			payload = map[string]any{}
		default:
			payload = map[string]any{}
		}
	case slack.IntegrationName:
		payload = map[string]any{
			"channel": result.Channel,
			"ts":      result.ExternalID,
		}
	case resend.IntegrationName:
		payload = map[string]any{
			"email_id": result.ExternalID,
		}
	case notion.IntegrationName:
		key := "page_id"
		if operation == notion.OperationAppendBlock {
			key = "block_id"
		}
		payload = map[string]any{key: result.ExternalID}
	default:
		payload = map[string]any{}
	}
	return marshalOutput(payload)
}

func activityEventToStreamEvent(event activities.Event) eventpkg.Event {
	return eventpkg.Event{
		EventID:        uuidFromString(event.EventID),
		TenantID:       uuidFromString(event.TenantID),
		WorkflowRunID:  optionalParsedUUID(event.WorkflowRunID),
		WorkflowNodeID: event.WorkflowNodeID,
		Type:           event.Type,
		Source:         event.Source,
		SourceKind:     event.SourceKind,
		Lineage:        event.Lineage,
		ChainDepth:     event.ChainDepth,
		Timestamp:      event.Timestamp,
		Payload:        event.Payload,
	}
}

func defaultConnectionIDForEvent(operation string, event activities.Event) string {
	targetIntegration, ok := registry.FindIntegrationByOperation(operation)
	if !ok {
		return ""
	}
	originIntegration := strings.TrimSpace(event.Source.Integration)
	originConnectionID := optionalActivityUUID(event.Source.ConnectionID)
	if event.Lineage != nil && strings.TrimSpace(event.Lineage.Integration) != "" {
		originIntegration = strings.TrimSpace(event.Lineage.Integration)
		originConnectionID = optionalActivityUUID(event.Lineage.ConnectionID)
	}
	if originIntegration == "" || originConnectionID == "" || targetIntegration != originIntegration {
		return ""
	}
	return originConnectionID
}

func optionalActivityUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func optionalParsedUUID(value string) *uuid.UUID {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &id
}

func uuidFromString(value string) uuid.UUID {
	id, _ := uuid.Parse(value)
	return id
}

func marshalOutput(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}
