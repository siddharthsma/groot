package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (a *Application) Shutdown(ctx context.Context) error {
	var shutdownErr error
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr = fmt.Errorf("shutdown http server: %w", err)
		}
	}
	if a.stopWorker != nil {
		a.stopWorker()
	}
	if a.temporalSDKClient != nil {
		a.temporalSDKClient.Close()
	}
	if a.kafkaClient != nil {
		if err := a.kafkaClient.Close(); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("close kafka writer: %w", err)
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("close postgres: %w", err)
		}
	}
	return shutdownErr
}
