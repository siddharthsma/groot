package workflows

import (
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/connectors/outbound"
	"groot/internal/temporal/activities"
)

const (
	WorkflowName  = "delivery_workflow"
	TaskQueueName = "groot-delivery"
)

func DeliveryWorkflow(ctx workflow.Context, deliveryJobID string, maxAttempts int) error {
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
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load delivery job: %v", err), nil)
	}

	logger.Info("delivery_attempt", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "RecordAttempt", deliveryJobID, attempt).Get(ctx, nil); err != nil {
		return fmt.Errorf("record attempt: %w", err)
	}

	var sub activities.Subscription
	if err := workflow.ExecuteActivity(ctx, "LoadSubscription", job.SubscriptionID).Get(ctx, &sub); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load subscription: %v", err), nil)
	}

	var event activities.Event
	if err := workflow.ExecuteActivity(ctx, "LoadEvent", job.EventID).Get(ctx, &event); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load event: %v", err), nil)
	}

	var err error
	var externalID *string
	var lastStatusCode *int
	var connectorName string
	var operation string
	switch sub.DestinationType {
	case "webhook":
		var app activities.ConnectedApp
		if loadErr := workflow.ExecuteActivity(ctx, "LoadConnectedApp", sub.ConnectedAppID, job.TenantID).Get(ctx, &app); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connected app: %v", loadErr), nil)
		}
		err = workflow.ExecuteActivity(ctx, activities.DeliverHTTPName, app.DestinationURL, event).Get(ctx, nil)
	case "function":
		var destination activities.FunctionDestination
		if loadErr := workflow.ExecuteActivity(ctx, "LoadFunctionDestination", sub.FunctionDestinationID, job.TenantID).Get(ctx, &destination); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load function destination: %v", loadErr), nil)
		}
		err = workflow.ExecuteActivity(ctx, activities.InvokeFunctionName, job.ID, destination.ID, event, destination.URL, destination.Secret, destination.TimeoutSeconds, attempt).Get(ctx, nil)
	case "connector":
		var instance activities.ConnectorInstance
		if loadErr := workflow.ExecuteActivity(ctx, "LoadConnectorInstance", sub.ConnectorInstanceID, job.TenantID).Get(ctx, &instance); loadErr != nil {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connector instance: %v", loadErr), nil)
		}
		connectorName = instance.ConnectorName
		operation = sub.Operation
		var result activities.ConnectorResult
		err = workflow.ExecuteActivity(ctx, activities.ExecuteConnectorName, job.ID, job.TenantID, event, instance, sub.Operation, sub.OperationParams, attempt).Get(ctx, &result)
		if err == nil {
			if result.ExternalID != "" {
				externalID = &result.ExternalID
			}
			if result.StatusCode != 0 {
				lastStatusCode = &result.StatusCode
			}
		}
	default:
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("unsupported destination type: %s", sub.DestinationType), nil)
	}

	if err != nil {
		message := err.Error()
		lastStatusCode = activities.ConnectorStatusCode(err)
		var permanent outbound.PermanentError
		if errors.As(err, &permanent) {
			return nonRetryableTerminal(ctx, deliveryJobID, attempt, message, lastStatusCode)
		}
		if attempt >= maxAttempts {
			logger.Info("delivery_dead_letter", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
			if markErr := workflow.ExecuteActivity(ctx, "MarkDeadLetter", deliveryJobID, message, connectorName, operation, job.TenantID, job.EventID, attempt, lastStatusCode).Get(ctx, nil); markErr != nil {
				return fmt.Errorf("mark dead letter: %w", markErr)
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
	return nil
}

func nonRetryableTerminal(ctx workflow.Context, deliveryJobID string, attempt int, message string, lastStatusCode *int) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("delivery_failed", "delivery_job_id", deliveryJobID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "MarkFailed", deliveryJobID, message, lastStatusCode).Get(ctx, nil); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return temporal.NewNonRetryableApplicationError(message, "delivery_failed", nil)
}
