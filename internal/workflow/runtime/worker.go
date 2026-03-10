package runtime

import (
	"context"
	"log/slog"
	"time"
)

type TimeoutWorker struct {
	service  *Service
	logger   *slog.Logger
	interval time.Duration
	limit    int
}

func NewTimeoutWorker(service *Service, logger *slog.Logger, interval time.Duration, limit int) *TimeoutWorker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if limit <= 0 {
		limit = 100
	}
	return &TimeoutWorker{
		service:  service,
		logger:   logger,
		interval: interval,
		limit:    limit,
	}
}

func (w *TimeoutWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.service.SweepExpiredWaits(ctx, w.limit); err != nil && w.logger != nil {
			w.logger.Error("workflow_wait_timeout_sweep_failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
