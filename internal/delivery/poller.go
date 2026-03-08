package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"groot/internal/config"
)

const (
	WorkflowName   = "delivery_workflow"
	TaskQueueName  = "groot-delivery"
	claimBatchSize = 10
)

type Store interface {
	ClaimPendingJobs(context.Context, int) ([]Job, error)
	RequeueJob(context.Context, uuid.UUID, string) error
}

type WorkflowStarter interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
}

type Poller struct {
	store   Store
	client  WorkflowStarter
	logger  *slog.Logger
	retry   config.DeliveryRetryConfig
	agent   config.AgentRuntimeConfig
	metrics Metrics
	ticker  func(time.Duration) ticker
}

type Metrics interface {
	IncDeliveryStarted()
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

func NewPoller(store Store, temporalClient WorkflowStarter, logger *slog.Logger, retry config.DeliveryRetryConfig, agent config.AgentRuntimeConfig, metrics Metrics) *Poller {
	return &Poller{
		store:   store,
		client:  temporalClient,
		logger:  logger,
		retry:   retry,
		agent:   agent,
		metrics: metrics,
		ticker:  func(d time.Duration) ticker { return realTicker{Ticker: time.NewTicker(d)} },
	}
}

func (p *Poller) Run(ctx context.Context) error {
	t := p.ticker(time.Second)
	defer t.Stop()

	for {
		if err := p.pollOnce(ctx); err != nil {
			p.logger.Error("delivery_poll_failed", slog.String("error", err.Error()))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-t.C():
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) error {
	jobs, err := p.store.ClaimPendingJobs(ctx, claimBatchSize)
	if err != nil {
		return fmt.Errorf("claim pending jobs: %w", err)
	}

	for _, job := range jobs {
		p.logger.Info("delivery_started",
			slog.String("delivery_job_id", job.ID.String()),
			slog.String("event_id", job.EventID.String()),
			slog.String("tenant_id", job.TenantID.String()),
			slog.Int("attempt", job.Attempts),
		)

		_, err := p.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        "delivery-job-" + job.ID.String(),
			TaskQueue: TaskQueueName,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts:    int32(p.retry.MaxAttempts),
				InitialInterval:    p.retry.InitialInterval,
				BackoffCoefficient: 2,
				MaximumInterval:    p.retry.MaxInterval,
			},
		}, WorkflowName, job.ID.String(), p.retry.MaxAttempts, p.agent)
		if err != nil {
			var startedErr *serviceerror.WorkflowExecutionAlreadyStarted
			if errors.As(err, &startedErr) {
				continue
			}
			if requeueErr := p.store.RequeueJob(ctx, job.ID, err.Error()); requeueErr != nil {
				return fmt.Errorf("start workflow: %w; requeue job: %v", err, requeueErr)
			}
			return fmt.Errorf("start workflow: %w", err)
		}
		if p.metrics != nil {
			p.metrics.IncDeliveryStarted()
		}
	}

	return nil
}
