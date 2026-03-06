package workflows

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/config"
	llmconnector "groot/internal/connectors/outbound/llm"
	notionconnector "groot/internal/connectors/outbound/notion"
	resendconnector "groot/internal/connectors/outbound/resend"
	slackconnector "groot/internal/connectors/outbound/slack"
	resultevents "groot/internal/events"
	"groot/internal/temporal/activities"
)

const (
	WorkflowName  = "delivery_workflow"
	TaskQueueName = "groot-delivery"
)

func DeliveryWorkflow(ctx workflow.Context, deliveryJobID string, maxAttempts int, agentConfig config.AgentConfig) error {
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
	case "connector":
		var instance activities.ConnectorInstance
		if loadErr := workflow.ExecuteActivity(ctx, "LoadConnectorInstance", sub.ConnectorInstanceID, job.TenantID).Get(ctx, &instance); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connector instance: %v", loadErr), nil, job, sub, event, connectorName, operation)
		}
		connectorName = instance.ConnectorName
		operation = sub.Operation
		if instance.ConnectorName == llmconnector.ConnectorName && sub.Operation == llmconnector.OperationAgent {
			childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
				WorkflowExecutionTimeout: agentConfig.TotalTimeout,
			})
			var result AgentResult
			err = workflow.ExecuteChildWorkflow(childCtx, AgentWorkflowName, AgentRequest{
				DeliveryJobID:      job.ID,
				TenantID:           job.TenantID,
				SubscriptionID:     sub.ID,
				Event:              event,
				ConnectorInstance:  instance,
				OperationParams:    sub.OperationParams,
				Attempt:            attempt,
				MaxSteps:           agentConfig.MaxSteps,
				StepTimeout:        agentConfig.StepTimeout,
				TotalTimeout:       agentConfig.TotalTimeout,
				MaxToolCalls:       agentConfig.MaxToolCalls,
				MaxToolOutputBytes: agentConfig.MaxToolOutputBytes,
			}).Get(childCtx, &result)
			if err == nil {
				statusCode := result.StatusCode
				lastStatusCode = &statusCode
				successOutput = result.Output
			}
		} else {
			var result activities.ConnectorResult
			err = workflow.ExecuteActivity(ctx, activities.ExecuteConnectorName, job.ID, job.TenantID, event, instance, sub.Operation, sub.OperationParams, attempt).Get(ctx, &result)
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
				_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, resultevents.ResultStatusFailed, nil, message, "dead_letter", externalID, lastStatusCode)).Get(ctx, nil)
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
		_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, resultevents.ResultStatusSucceeded, successOutput, "", "", externalID, lastStatusCode)).Get(ctx, nil)
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
		_ = workflow.ExecuteActivity(ctx, "EmitResultEvent", buildResultEventRequest(job, sub, event, connectorName, operation, resultevents.ResultStatusFailed, nil, message, "failed", nil, lastStatusCode)).Get(ctx, nil)
	}
	return temporal.NewNonRetryableApplicationError(message, "delivery_failed", nil)
}

func shouldEmitSuccess(sub activities.Subscription, connectorName, operation string) bool {
	return sub.EmitSuccessEvent && connectorName != "" && operation != ""
}

func shouldEmitFailure(sub activities.Subscription, connectorName, operation string) bool {
	return sub.EmitFailureEvent && connectorName != "" && operation != ""
}

func buildResultEventRequest(job activities.DeliveryJob, sub activities.Subscription, event activities.Event, connectorName, operation, status string, output []byte, errorMessage, errorType string, externalID *string, httpStatus *int) activities.ResultEventRequest {
	return activities.ResultEventRequest{
		DeliveryJobID:         job.ID,
		TenantID:              job.TenantID,
		SubscriptionID:        sub.ID,
		InputEventID:          job.EventID,
		ExistingResultEventID: job.ResultEventID,
		InputEvent:            event,
		ConnectorName:         connectorName,
		Operation:             operation,
		Status:                status,
		ExternalID:            externalID,
		HTTPStatusCode:        httpStatus,
		Output:                output,
		ErrorMessage:          errorMessage,
		ErrorType:             errorType,
	}
}

func connectorOutput(connectorName, operation string, result activities.ConnectorResult) ([]byte, error) {
	if len(result.Output) > 0 {
		return result.Output, nil
	}
	var payload map[string]any
	switch connectorName {
	case llmconnector.ConnectorName:
		payload = map[string]any{
			"text":     result.Text,
			"provider": result.Provider,
			"model":    result.Model,
			"usage": map[string]any{
				"prompt_tokens":     result.Usage.PromptTokens,
				"completion_tokens": result.Usage.CompletionTokens,
				"total_tokens":      result.Usage.TotalTokens,
			},
		}
	case slackconnector.ConnectorName:
		payload = map[string]any{
			"channel": result.Channel,
			"ts":      result.ExternalID,
		}
	case resendconnector.ConnectorName:
		payload = map[string]any{
			"email_id": result.ExternalID,
		}
	case notionconnector.ConnectorName:
		key := "page_id"
		if operation == notionconnector.OperationAppendBlock {
			key = "block_id"
		}
		payload = map[string]any{key: result.ExternalID}
	default:
		payload = map[string]any{}
	}
	return marshalOutput(payload)
}

func marshalOutput(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}
