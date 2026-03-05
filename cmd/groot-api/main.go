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
	"groot/internal/connectedapp"
	"groot/internal/delivery"
	"groot/internal/httpapi"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/router"
	"groot/internal/storage"
	"groot/internal/stream"
	"groot/internal/subscription"
	groottemporal "groot/internal/temporal"
	"groot/internal/temporal/activities"
	"groot/internal/tenant"
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
	defer func() {
		if closeErr := kafkaClient.Close(); closeErr != nil {
			logger.Error("close kafka writer", slog.String("error", closeErr.Error()))
		}
	}()
	if err := kafkaClient.EnsureTopic(ctx); err != nil {
		logger.Error("ensure kafka topic", slog.String("error", err.Error()))
		os.Exit(1)
	}

	temporalClient := groottemporal.New(cfg.TemporalAddress, cfg.TemporalNamespace)
	if err := temporalClient.Check(ctx); err != nil {
		logger.Error("connect temporal", slog.String("error", err.Error()))
		os.Exit(1)
	}
	temporalSDKClient, err := temporalClient.Dial()
	if err != nil {
		logger.Error("dial temporal sdk client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer temporalSDKClient.Close()
	temporalWorker := groottemporal.NewWorker(temporalSDKClient, activities.Dependencies{Store: db})
	if err := groottemporal.StartWorker(temporalWorker); err != nil {
		logger.Error("start temporal worker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer temporalWorker.Stop()

	tenantService := tenant.NewService(db)
	appService := connectedapp.NewService(db)
	subscriptionService := subscription.NewService(db, appService)
	eventService := ingest.NewService(kafkaClient, db, logger)
	routerConsumer := router.NewConsumer(cfg.KafkaBrokers, db, logger)
	deliveryPoller := delivery.NewPoller(db, temporalSDKClient, logger)

	handler := httpapi.NewHandler(httpapi.Options{
		Logger: logger,
		Checkers: []httpapi.NamedChecker{
			{Name: "postgres", Checker: db},
			{Name: "kafka", Checker: kafkaClient},
			{Name: "temporal", Checker: temporalClient},
		},
		Tenants:  tenantService,
		Apps:     appService,
		Subs:     subscriptionService,
		EventSvc: eventService,
	})

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
	}()
	go func() {
		if runErr := routerConsumer.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			errCh <- runErr
		}
	}()
	go func() {
		if runErr := deliveryPoller.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			errCh <- runErr
		}
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
