package delivery

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"

	"groot/internal/config"
)

type stubStore struct {
	claimFn   func(context.Context, int) ([]Job, error)
	requeueFn func(context.Context, uuid.UUID, string) error
}

func (s stubStore) ClaimPendingJobs(ctx context.Context, limit int) ([]Job, error) {
	return s.claimFn(ctx, limit)
}

func (s stubStore) RequeueJob(ctx context.Context, id uuid.UUID, lastError string) error {
	return s.requeueFn(ctx, id, lastError)
}

type stubStarter struct {
	startFn func(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
}

func (s stubStarter) ExecuteWorkflow(ctx context.Context, opts client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return s.startFn(ctx, opts, workflow, args...)
}

func TestPollOnceStartsWorkflow(t *testing.T) {
	jobID := uuid.New()
	called := false
	p := NewPoller(
		stubStore{claimFn: func(context.Context, int) ([]Job, error) {
			return []Job{{ID: jobID, EventID: uuid.New(), TenantID: uuid.New()}}, nil
		}, requeueFn: func(context.Context, uuid.UUID, string) error { return nil }},
		stubStarter{startFn: func(_ context.Context, opts client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
			called = true
			if opts.TaskQueue != TaskQueueName {
				t.Fatalf("task queue = %q", opts.TaskQueue)
			}
			if got, want := opts.RetryPolicy.MaximumAttempts, int32(3); got != want {
				t.Fatalf("max attempts = %d, want %d", got, want)
			}
			return nil, nil
		}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		config.DeliveryRetryConfig{MaxAttempts: 3, InitialInterval: time.Second, MaxInterval: 5 * time.Second},
		nil,
	)
	if err := p.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if !called {
		t.Fatal("expected workflow start")
	}
}

type stubTicker struct{ ch chan time.Time }

func (s stubTicker) C() <-chan time.Time { return s.ch }
func (s stubTicker) Stop()               {}
