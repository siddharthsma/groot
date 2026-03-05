package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"groot/internal/config"
	"groot/internal/httpapi"
	"groot/internal/observability"
	"groot/internal/storage"
	"groot/internal/stream"
	groottemporal "groot/internal/temporal"
)

func main() {
	logger := observability.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := storage.New(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error("connect postgres", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("close postgres", slog.String("error", closeErr.Error()))
		}
	}()

	kafkaClient := stream.New(cfg.KafkaBrokers)
	if err := kafkaClient.Check(ctx); err != nil {
		logger.Error("connect kafka", slog.String("error", err.Error()))
		os.Exit(1)
	}

	temporalClient := groottemporal.New(cfg.TemporalAddress, cfg.TemporalNamespace)
	if err := temporalClient.Check(ctx); err != nil {
		logger.Error("connect temporal", slog.String("error", err.Error()))
		os.Exit(1)
	}

	handler := httpapi.NewHandler(
		httpapi.NamedChecker{Name: "postgres", Checker: db},
		httpapi.NamedChecker{Name: "kafka", Checker: kafkaClient},
		httpapi.NamedChecker{Name: "temporal", Checker: temporalClient},
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", cfg.HTTPAddr))
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown http server", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil {
			logger.Error("serve http", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}
