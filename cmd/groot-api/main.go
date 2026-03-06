package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	adminauth "groot/internal/admin/auth"
	"groot/internal/apikey"
	"groot/internal/audit"
	authn "groot/internal/auth"
	"groot/internal/config"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	slackconnector "groot/internal/connectors/inbound/slack"
	stripeconnector "groot/internal/connectors/inbound/stripe"
	"groot/internal/connectors/resend"
	"groot/internal/delivery"
	"groot/internal/eventquery"
	resultevents "groot/internal/events"
	"groot/internal/functiondestination"
	"groot/internal/graph"
	"groot/internal/httpapi"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/replay"
	"groot/internal/router"
	"groot/internal/schemas"
	"groot/internal/storage"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
	groottemporal "groot/internal/temporal"
	"groot/internal/temporal/activities"
	"groot/internal/tenant"
)

func main() {
	logger := observability.NewLogger()
	metrics := observability.NewMetrics()

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
	schemaService := schemas.NewService(db, schemas.Config{
		ValidationMode:   schemas.NormalizeValidationMode(cfg.Schema.ValidationMode),
		RegistrationMode: cfg.Schema.RegistrationMode,
		MaxPayloadBytes:  cfg.Schema.MaxPayloadBytes,
	}, logger, metrics)
	if cfg.Schema.RegistrationMode == schemas.RegistrationModeStartup {
		if err := schemaService.RegisterBundles(ctx, schemas.DefaultBundles()); err != nil {
			if strings.Contains(err.Error(), `relation "event_schemas" does not exist`) {
				logger.Info("schema registration skipped until migrations are applied", slog.String("error", err.Error()))
			} else {
				logger.Error("register event schemas", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}
	}
	resultEmitter := resultevents.NewEmitter(kafkaClient, db, logger, metrics, cfg.MaxChainDepth, resultevents.WithSchemaResolver(schemaService))
	temporalWorker := groottemporal.NewWorker(temporalSDKClient, activities.Dependencies{
		Store:            db,
		Slack:            cfg.Slack,
		Resend:           cfg.Resend,
		NotionAPIBaseURL: cfg.Notion.APIBaseURL,
		NotionAPIVersion: cfg.Notion.APIVersion,
		LLM:              cfg.LLM,
		ResultEmitter:    resultEmitter,
		Metrics:          metrics,
		Logger:           logger,
	})
	if err := groottemporal.StartWorker(temporalWorker); err != nil {
		logger.Error("start temporal worker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer temporalWorker.Stop()

	tenantService := tenant.NewService(db)
	apiKeyService := apikey.NewService(db)
	jwtVerifier, err := authn.NewJWTVerifier(ctx, cfg.Auth)
	if err != nil {
		logger.Error("initialize jwt verifier", slog.String("error", err.Error()))
		os.Exit(1)
	}
	authService := authn.NewService(cfg.Auth, tenantService, apiKeyService, jwtVerifier)
	adminAuthService, err := adminauth.New(ctx, cfg.Admin)
	if err != nil {
		logger.Error("initialize admin auth", slog.String("error", err.Error()))
		os.Exit(1)
	}
	auditService := audit.NewService(db, cfg.Audit.Enabled)
	appService := connectedapp.NewService(db)
	connectorInstanceService := connectorinstance.NewService(db, cfg.AllowGlobalInstances, cfg.LLM.DefaultProvider)
	inboundRouteService := inboundroute.NewService(db, metrics)
	functionService := functiondestination.NewService(db)
	filterService := subscriptionfilter.NewService(schemaService)
	subscriptionService := subscription.NewService(db, appService, functionService, connectorInstanceService, cfg.AllowGlobalInstances, subscription.WithTemplateValidator(schemaService), subscription.WithFilterValidator(filterService), subscription.WithLogger(logger))
	eventService := ingest.NewService(kafkaClient, db, logger, metrics, ingest.WithSchemaValidator(schemaService))
	eventQueryService := eventquery.NewService(db)
	deliveryService := delivery.NewService(db, metrics)
	graphService := graph.NewService(db, graph.Config{
		MaxNodes:                    cfg.Graph.MaxNodes,
		MaxEdges:                    cfg.Graph.MaxEdges,
		ExecutionTraversalMaxEvents: cfg.Graph.ExecutionTraversalMaxEvents,
		ExecutionMaxDepth:           cfg.Graph.ExecutionMaxDepth,
		DefaultLimit:                cfg.Graph.DefaultLimit,
	}, logger, metrics)
	replayService := replay.NewService(db, cfg.Replay, metrics)
	adminReplayService := replay.NewService(db, config.ReplayConfig{
		MaxEvents:      cfg.Admin.ReplayMaxEvents,
		MaxWindowHours: cfg.Replay.MaxWindowHours,
	}, metrics)
	resendService := resend.NewService(cfg.Resend, db, eventService, logger, metrics, nil)
	slackService := slackconnector.NewService(cfg.Slack, db, eventService, logger, metrics)
	stripeService := stripeconnector.NewService(cfg.Stripe, db, eventService, logger, metrics)
	routerConsumer := router.NewConsumer(cfg.KafkaBrokers, db, logger, metrics)
	deliveryPoller := delivery.NewPoller(db, temporalSDKClient, logger, cfg.DeliveryRetry, cfg.Agent, metrics)

	handler := httpapi.NewHandler(httpapi.Options{
		Logger: logger,
		Checkers: []httpapi.NamedChecker{
			{Name: "postgres", Checker: db},
			{Name: "kafka", Checker: kafkaClient},
			{Name: "temporal", Checker: temporalClient},
		},
		RouterCheckers: []httpapi.NamedChecker{
			{Name: "postgres", Checker: db},
			{Name: "kafka", Checker: kafkaClient},
		},
		DeliveryCheckers: []httpapi.NamedChecker{
			{Name: "postgres", Checker: db},
			{Name: "temporal", Checker: temporalClient},
		},
		Tenants:                tenantService,
		Apps:                   appService,
		Functions:              functionService,
		Subs:                   subscriptionService,
		ConnectorInstances:     connectorInstanceService,
		InboundRoutes:          inboundRouteService,
		EventSvc:               eventService,
		EventQuerySvc:          eventQueryService,
		Deliveries:             deliveryService,
		Replay:                 replayService,
		AdminReplay:            adminReplayService,
		Schemas:                schemaService,
		Resend:                 resendService,
		Slack:                  slackService,
		Stripe:                 stripeService,
		Auth:                   authService,
		AdminAuth:              adminAuthService,
		AdminEnabled:           cfg.Admin.Enabled,
		AdminAllowViewPayloads: cfg.Admin.AllowViewPayloads,
		AdminReplayEnabled:     cfg.Admin.ReplayEnabled,
		AdminRateLimitRPS:      cfg.Admin.RateLimitRPS,
		AdminReplayMaxEvents:   cfg.Admin.ReplayMaxEvents,
		APIKeys:                apiKeyService,
		Graph:                  graphService,
		Audit:                  auditService,
		SystemAPIKey:           cfg.SystemAPIKey,
		Metrics:                metrics,
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
