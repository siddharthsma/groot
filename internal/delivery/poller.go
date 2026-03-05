package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
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
	store  Store
	client WorkflowStarter
	logger *slog.Logger
	ticker func(time.Duration) ticker
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

func NewPoller(store Store, temporalClient WorkflowStarter, logger *slog.Logger) *Poller {
	return &Poller{
		store:  store,
		client: temporalClient,
		logger: logger,
		ticker: func(d time.Duration) ticker { return realTicker{Ticker: time.NewTicker(d)} },
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
				MaximumAttempts:    10,
				InitialInterval:    2 * time.Second,
				BackoffCoefficient: 2,
				MaximumInterval:    5 * time.Minute,
			},
		}, WorkflowName, job.ID.String())
		if err != nil {
			if requeueErr := p.store.RequeueJob(ctx, job.ID, err.Error()); requeueErr != nil {
				return fmt.Errorf("start workflow: %w; requeue job: %v", err, requeueErr)
			}
			return fmt.Errorf("start workflow: %w", err)
		}
	}

	return nil
}
