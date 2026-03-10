package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (a *Application) Run(ctx context.Context) error {
	if err := a.startWorker(); err != nil {
		return fmt.Errorf("start temporal worker: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 4)
	go func() {
		a.logger.Info("http server listening", slog.String("addr", a.httpAddr))
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		if err := a.runRouter(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()
	go func() {
		if err := a.runDeliveryPoller(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()
	go func() {
		if a.runWorkflowWaits == nil {
			return
		}
		if err := a.runWorkflowWaits(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return a.Shutdown(shutdownCtx)
	case err := <-errCh:
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if shutdownErr := a.Shutdown(shutdownCtx); shutdownErr != nil {
			return fmt.Errorf("run application: %w; shutdown: %v", err, shutdownErr)
		}
		return fmt.Errorf("run application: %w", err)
	}
}
