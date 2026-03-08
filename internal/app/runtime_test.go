package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunStopsCleanlyOnContextCancel(t *testing.T) {
	t.Parallel()

	var workerStarted atomic.Bool
	var workerStopped atomic.Bool
	routerStarted := make(chan struct{})
	pollerStarted := make(chan struct{})

	app := &Application{
		logger:   testLogger(),
		httpAddr: "127.0.0.1:0",
		server: &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NewServeMux(),
		},
		startWorker: func() error {
			workerStarted.Store(true)
			return nil
		},
		stopWorker: func() {
			workerStopped.Store(true)
		},
		runRouter: func(ctx context.Context) error {
			close(routerStarted)
			<-ctx.Done()
			return ctx.Err()
		},
		runDeliveryPoller: func(ctx context.Context) error {
			close(pollerStarted)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	waitForSignal(t, routerStarted, "router")
	waitForSignal(t, pollerStarted, "delivery poller")

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not exit after context cancellation")
	}

	if !workerStarted.Load() {
		t.Fatal("temporal worker was not started")
	}
	if !workerStopped.Load() {
		t.Fatal("temporal worker was not stopped")
	}
}

func TestRunReturnsComponentErrorAndShutsDown(t *testing.T) {
	t.Parallel()

	componentErr := errors.New("router failed")
	var workerStopped atomic.Bool

	app := &Application{
		logger:   testLogger(),
		httpAddr: "127.0.0.1:0",
		server: &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NewServeMux(),
		},
		startWorker: func() error {
			return nil
		},
		stopWorker: func() {
			workerStopped.Store(true)
		},
		runRouter: func(ctx context.Context) error {
			return componentErr
		},
		runDeliveryPoller: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	err := app.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if !errors.Is(err, componentErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, componentErr)
	}
	if !workerStopped.Load() {
		t.Fatal("temporal worker was not stopped after component failure")
	}
}

func TestRunReturnsWorkerStartupError(t *testing.T) {
	t.Parallel()

	startErr := errors.New("worker start failed")
	app := &Application{
		logger: testLogger(),
		startWorker: func() error {
			return startErr
		},
	}

	err := app.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, startErr)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitForSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not start", name)
	}
}
