package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/temporal/activities"
)

const (
	WorkflowName  = "delivery_workflow"
	TaskQueueName = "groot-delivery"
)

func DeliveryWorkflow(ctx workflow.Context, deliveryJobID string) error {
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
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load delivery job: %v", err))
	}

	logger.Info("delivery_attempt", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "RecordAttempt", deliveryJobID, attempt).Get(ctx, nil); err != nil {
		return fmt.Errorf("record attempt: %w", err)
	}

	var sub activities.Subscription
	if err := workflow.ExecuteActivity(ctx, "LoadSubscription", job.SubscriptionID).Get(ctx, &sub); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load subscription: %v", err))
	}

	var app activities.ConnectedApp
	if err := workflow.ExecuteActivity(ctx, "LoadConnectedApp", sub.ConnectedAppID, job.TenantID).Get(ctx, &app); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load connected app: %v", err))
	}

	var event activities.Event
	if err := workflow.ExecuteActivity(ctx, "LoadEvent", job.EventID).Get(ctx, &event); err != nil {
		return nonRetryableTerminal(ctx, deliveryJobID, attempt, fmt.Sprintf("load event: %v", err))
	}

	if err := workflow.ExecuteActivity(ctx, activities.DeliverHTTPName, app.DestinationURL, event).Get(ctx, nil); err != nil {
		message := err.Error()
		maxAttempts := 10
		if attempt >= maxAttempts {
			logger.Info("delivery_dead_letter", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
			if markErr := workflow.ExecuteActivity(ctx, "MarkDeadLetter", deliveryJobID, message).Get(ctx, nil); markErr != nil {
				return fmt.Errorf("mark dead letter: %w", markErr)
			}
			return nil
		}

		logger.Info("delivery_failed", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
		if markErr := workflow.ExecuteActivity(ctx, "MarkRetryableFailure", deliveryJobID, message).Get(ctx, nil); markErr != nil {
			return fmt.Errorf("mark retryable failure: %w", markErr)
		}
		return fmt.Errorf("deliver http: %w", err)
	}

	logger.Info("delivery_succeeded", "delivery_job_id", job.ID, "event_id", job.EventID, "tenant_id", job.TenantID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "MarkSucceeded", deliveryJobID).Get(ctx, nil); err != nil {
		return fmt.Errorf("mark succeeded: %w", err)
	}
	return nil
}

func nonRetryableTerminal(ctx workflow.Context, deliveryJobID string, attempt int, message string) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("delivery_failed", "delivery_job_id", deliveryJobID, "attempt", attempt)
	if err := workflow.ExecuteActivity(ctx, "MarkFailed", deliveryJobID, message).Get(ctx, nil); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return temporal.NewNonRetryableApplicationError(message, "delivery_failed", nil)
}
