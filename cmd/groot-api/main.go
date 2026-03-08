package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"groot/internal/app"
	"groot/internal/edition"
	"groot/internal/observability"
)

var (
	BuildEdition                = "internal"
	BuildLicensePublicKeyBase64 = edition.EmbeddedPublicKeyBase64
)

func main() {
	logger := observability.NewLogger()
	metrics := observability.NewMetrics()

	cfg, err := app.LoadConfig(BuildEdition, BuildLicensePublicKeyBase64)
	if err != nil {
		logger.Error("load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.Bootstrap(ctx, cfg, logger, metrics)
	if err != nil {
		logger.Error("bootstrap application", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil {
		logger.Error("run application", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
