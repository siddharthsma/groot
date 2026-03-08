package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	adminauth "groot/internal/admin/auth"
	"groot/internal/agent"
	agentruntime "groot/internal/agent/runtime"
	"groot/internal/apikey"
	"groot/internal/audit"
	authn "groot/internal/auth"
	"groot/internal/config"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/catalog"
	"groot/internal/connectors/installer"
	"groot/internal/connectors/pluginloader"
	_ "groot/internal/connectors/providers/builtin"
	"groot/internal/connectors/providers/resend"
	slackconnector "groot/internal/connectors/providers/slack"
	stripeconnector "groot/internal/connectors/providers/stripe"
	"groot/internal/connectors/registry"
	"groot/internal/delivery"
	"groot/internal/edition"
	"groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/graph"
	"groot/internal/httpapi"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/observability"
	"groot/internal/replay"
	"groot/internal/router"
	"groot/internal/schema"
	"groot/internal/storage"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
	groottemporal "groot/internal/temporal"
	"groot/internal/temporal/activities"
	"groot/internal/tenant"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type Application struct {
	logger *slog.Logger

	server            *http.Server
	httpAddr          string
	runRouter         func(context.Context) error
	runDeliveryPoller func(context.Context) error
	startWorker       func() error
	stopWorker        func()

	db                *storage.DB
	kafkaClient       *stream.Client
	temporalSDKClient client.Client
}

func Bootstrap(ctx context.Context, cfg Config, logger *slog.Logger, metrics *observability.Metrics) (*Application, error) {
	db, kafkaClient, temporalSDKClient, temporalWorker, handler, routerConsumer, deliveryPoller, err := bootstrapDependencies(ctx, cfg, logger, metrics)
	if err != nil {
		return nil, err
	}

	server := &http.Server{
		Addr:              cfg.Runtime.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Application{
		logger:            logger,
		server:            server,
		httpAddr:          cfg.Runtime.HTTPAddr,
		runRouter:         routerConsumer.Run,
		runDeliveryPoller: deliveryPoller.Run,
		startWorker: func() error {
			return groottemporal.StartWorker(temporalWorker)
		},
		stopWorker:        temporalWorker.Stop,
		db:                db,
		kafkaClient:       kafkaClient,
		temporalSDKClient: temporalSDKClient,
	}, nil
}

func bootstrapDependencies(
	ctx context.Context,
	cfg Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) (
	*storage.DB,
	*stream.Client,
	client.Client,
	worker.Worker,
	http.Handler,
	*router.Consumer,
	*delivery.Poller,
	error,
) {
	db, err := storage.New(ctx, cfg.Runtime.PostgresDSN)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	tenantService := tenant.NewService(db)

	runtimeEdition, err := edition.Resolve(cfg.BuildEdition, cfg.Runtime.Edition, cfg.Runtime.License, cfg.BuildLicensePublicKeyBase64)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("resolve edition: %w", err)
	}
	edition.SetRuntime(runtimeEdition)
	metrics.SetEditionInfo(string(runtimeEdition.BuildEdition), string(runtimeEdition.TenancyMode))
	metrics.SetLicenseInfo(string(runtimeEdition.BuildEdition), runtimeEdition.License.Present)
	logger.Info("edition_initialized",
		slog.String("build_edition", string(runtimeEdition.BuildEdition)),
		slog.String("effective_edition", string(runtimeEdition.EffectiveEdition)),
		slog.String("tenancy_mode", string(runtimeEdition.TenancyMode)),
		slog.Bool("license_present", runtimeEdition.License.Present),
		slog.Bool("license_valid", runtimeEdition.License.Valid),
		slog.String("licensee", runtimeEdition.License.Licensee),
		slog.Int("effective_max_tenants", runtimeEdition.MaxTenants),
		slog.Bool("multi_tenant", runtimeEdition.Capabilities.MultiTenant),
		slog.Bool("cross_tenant_admin", runtimeEdition.Capabilities.CrossTenantAdmin),
		slog.Bool("tenant_creation_allowed", runtimeEdition.Capabilities.TenantCreationAllowed),
		slog.Bool("hosted_billing_enabled", runtimeEdition.Capabilities.HostedBillingEnabled),
		slog.Bool("internal_runtime_tool_access", runtimeEdition.Capabilities.InternalRuntimeToolAccess),
	)
	for _, line := range runtimeEdition.BannerLines() {
		logger.Info(line,
			slog.String("build_edition", string(runtimeEdition.BuildEdition)),
			slog.String("tenancy_mode", string(runtimeEdition.TenancyMode)),
		)
	}
	bootstrapTenant, _, err := edition.EnsureCommunityTenant(ctx, runtimeEdition, tenantService, db, cfg.Runtime.CommunityTenantName)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("enforce edition tenancy: %w", err)
	}
	tenantRecords, err := tenantService.ListTenants(ctx)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("list tenants for tenant-limit validation: %w", err)
	}
	if err := edition.ValidateTenantCount(runtimeEdition, len(tenantRecords)); err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("validate tenant count: %w", err)
	}
	bootstrapTenantID, err := edition.LoadBootstrapTenantID(ctx, runtimeEdition, db)
	if err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("load community bootstrap tenant: %w", err)
	}
	if bootstrapTenantID == nil && runtimeEdition.IsCommunity() {
		id := bootstrapTenant.ID
		bootstrapTenantID = &id
	}

	kafkaClient := stream.New(cfg.Runtime.KafkaBrokers)
	if err := kafkaClient.Check(ctx); err != nil {
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("connect kafka: %w", err)
	}
	if err := kafkaClient.EnsureTopic(ctx); err != nil {
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("ensure kafka topic: %w", err)
	}

	temporalClient := groottemporal.New(cfg.Runtime.TemporalAddress, cfg.Runtime.TemporalNamespace)
	if err := temporalClient.Check(ctx); err != nil {
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("connect temporal: %w", err)
	}
	temporalSDKClient, err := temporalClient.Dial()
	if err != nil {
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("dial temporal sdk client: %w", err)
	}

	schemaService := schema.NewService(db, schema.Config{
		ValidationMode:   schema.NormalizeValidationMode(cfg.Runtime.Schema.ValidationMode),
		RegistrationMode: cfg.Runtime.Schema.RegistrationMode,
		MaxPayloadBytes:  cfg.Runtime.Schema.MaxPayloadBytes,
	}, logger, metrics)
	installedProviders, err := installer.LoadInstalled(cfg.Runtime.ProviderInstalledPath)
	if err != nil {
		temporalSDKClient.Close()
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("load installed providers metadata: %w", err)
	}
	pluginInstalled := make([]pluginloader.InstalledMetadata, 0, len(installedProviders.Providers))
	for _, current := range installedProviders.Providers {
		pluginInstalled = append(pluginInstalled, pluginloader.InstalledMetadata{
			Name:       current.Name,
			Version:    current.Version,
			Publisher:  current.Publisher,
			PluginPath: current.PluginPath,
		})
	}
	pluginLoader := pluginloader.New(cfg.Runtime.ProviderPluginDir, pluginInstalled, logger, schemaService)
	if _, err := pluginLoader.Load(ctx); err != nil {
		temporalSDKClient.Close()
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("load provider plugins: %w", err)
	}
	if cfg.Runtime.Schema.RegistrationMode == schema.RegistrationModeStartup {
		if err := registry.Validate(); err != nil {
			temporalSDKClient.Close()
			_ = kafkaClient.Close()
			_ = db.Close()
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("validate providers: %w", err)
		}
		bundles := append(schema.CoreBundles(), registry.BundlesBySource(registry.SourceCore)...)
		if err := schemaService.RegisterBundles(ctx, bundles); err != nil {
			if strings.Contains(err.Error(), `relation "event_schemas" does not exist`) {
				logger.Info("schema registration skipped until migrations are applied", slog.String("error", err.Error()))
			} else {
				temporalSDKClient.Close()
				_ = kafkaClient.Close()
				_ = db.Close()
				return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("register event schemas: %w", err)
			}
		}
	}
	providerCatalogService := catalog.NewService(schemaService)
	if err := providerCatalogService.Validate(ctx); err != nil {
		temporalSDKClient.Close()
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("validate provider catalog: %w", err)
	}

	resultEmitter := event.NewEmitter(kafkaClient, db, logger, metrics, cfg.Runtime.MaxChainDepth, event.WithSchemaResolver(schemaService))
	agentRuntimeClient := agentruntime.NewClient(agentruntime.Config{
		Enabled:         cfg.Runtime.AgentRuntime.Enabled,
		BaseURL:         cfg.Runtime.AgentRuntime.BaseURL,
		Timeout:         cfg.Runtime.AgentRuntime.Timeout,
		SharedSecret:    cfg.Runtime.AgentRuntime.SharedSecret,
		ToolEndpointURL: deriveInternalToolEndpoint(cfg.Runtime.HTTPAddr),
	}, nil)
	temporalWorker := groottemporal.NewWorker(temporalSDKClient, cfg.Runtime.DeliveryTaskQueue, activities.Dependencies{
		Store:            db,
		AgentManager:     agent.NewService(db, functiondestination.NewService(db)),
		Slack:            cfg.Runtime.Slack,
		Resend:           cfg.Runtime.Resend,
		NotionAPIBaseURL: cfg.Runtime.Notion.APIBaseURL,
		NotionAPIVersion: cfg.Runtime.Notion.APIVersion,
		LLM:              cfg.Runtime.LLM,
		ResultEmitter:    resultEmitter,
		Metrics:          metrics,
		Logger:           logger,
		AgentRuntime:     agentRuntimeClient,
		AgentRuntimeCfg:  cfg.Runtime.AgentRuntime,
	})

	apiKeyService := apikey.NewService(db)
	jwtVerifier, err := authn.NewJWTVerifier(ctx, cfg.Runtime.Auth)
	if err != nil {
		temporalSDKClient.Close()
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("initialize jwt verifier: %w", err)
	}
	authService := authn.NewService(cfg.Runtime.Auth, tenantService, apiKeyService, jwtVerifier)
	adminAuthService, err := adminauth.New(ctx, cfg.Runtime.Admin)
	if err != nil {
		temporalSDKClient.Close()
		_ = kafkaClient.Close()
		_ = db.Close()
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("initialize admin auth: %w", err)
	}

	auditService := audit.NewService(db, cfg.Runtime.Audit.Enabled)
	appService := connectedapp.NewService(db)
	connectorInstanceService := connectorinstance.NewService(db, cfg.Runtime.AllowGlobalInstances, cfg.Runtime.LLM.DefaultProvider)
	inboundRouteService := inboundroute.NewService(db, metrics)
	functionService := functiondestination.NewService(db)
	agentService := agent.NewService(db, functionService)
	agentExecutor := agent.NewExecutor(db, functionService, cfg.Runtime.Slack, cfg.Runtime.Resend, cfg.Runtime.Notion, cfg.Runtime.LLM, nil)
	filterService := subscriptionfilter.NewService(schemaService)
	subscriptionService := subscription.NewService(db, appService, functionService, connectorInstanceService, cfg.Runtime.AllowGlobalInstances, subscription.WithAgentStore(agentService), subscription.WithTemplateValidator(schemaService), subscription.WithFilterValidator(filterService), subscription.WithLogger(logger))
	eventService := ingest.NewService(kafkaClient, db, logger, metrics, ingest.WithSchemaValidator(schemaService))
	eventQueryService := event.NewQueryService(db)
	deliveryService := delivery.NewService(db, metrics)
	graphService := graph.NewService(db, graph.Config{
		MaxNodes:                    cfg.Runtime.Graph.MaxNodes,
		MaxEdges:                    cfg.Runtime.Graph.MaxEdges,
		ExecutionTraversalMaxEvents: cfg.Runtime.Graph.ExecutionTraversalMaxEvents,
		ExecutionMaxDepth:           cfg.Runtime.Graph.ExecutionMaxDepth,
		DefaultLimit:                cfg.Runtime.Graph.DefaultLimit,
	}, logger, metrics)
	replayService := replay.NewService(db, cfg.Runtime.Replay, metrics)
	adminReplayService := replay.NewService(db, config.ReplayConfig{
		MaxEvents:      cfg.Runtime.Admin.ReplayMaxEvents,
		MaxWindowHours: cfg.Runtime.Replay.MaxWindowHours,
	}, metrics)
	resendService := resend.NewService(cfg.Runtime.Resend, db, eventService, logger, metrics, nil)
	slackService := slackconnector.NewService(cfg.Runtime.Slack, db, eventService, logger, metrics)
	stripeService := stripeconnector.NewService(cfg.Runtime.Stripe, db, eventService, logger, metrics)
	routerConsumer := router.NewConsumer(cfg.Runtime.KafkaBrokers, cfg.Runtime.RouterConsumerGroup, db, logger, metrics)
	deliveryPoller := delivery.NewPoller(db, temporalSDKClient, logger, cfg.Runtime.DeliveryRetry, cfg.Runtime.AgentRuntime, cfg.Runtime.DeliveryTaskQueue, metrics)

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
		Tenants:                  tenantService,
		Apps:                     appService,
		Functions:                functionService,
		Subs:                     subscriptionService,
		ConnectorInstances:       connectorInstanceService,
		InboundRoutes:            inboundRouteService,
		EventSvc:                 eventService,
		EventQuerySvc:            eventQueryService,
		Deliveries:               deliveryService,
		Replay:                   replayService,
		AdminReplay:              adminReplayService,
		Schemas:                  schemaService,
		ProviderCatalog:          providerCatalogService,
		Resend:                   resendService,
		Slack:                    slackService,
		Stripe:                   stripeService,
		Auth:                     authService,
		AdminAuth:                adminAuthService,
		AdminEnabled:             cfg.Runtime.Admin.Enabled,
		AdminAllowViewPayloads:   cfg.Runtime.Admin.AllowViewPayloads,
		AdminReplayEnabled:       cfg.Runtime.Admin.ReplayEnabled,
		AdminRateLimitRPS:        cfg.Runtime.Admin.RateLimitRPS,
		AdminReplayMaxEvents:     cfg.Runtime.Admin.ReplayMaxEvents,
		APIKeys:                  apiKeyService,
		Graph:                    graphService,
		Audit:                    auditService,
		Agents:                   agentService,
		AgentTools:               agentExecutor,
		SystemAPIKey:             cfg.Runtime.SystemAPIKey,
		AgentRuntimeSharedSecret: cfg.Runtime.AgentRuntime.SharedSecret,
		Edition:                  runtimeEdition,
		CommunityBootstrapTenant: bootstrapTenantID,
		Metrics:                  metrics,
	})

	return db, kafkaClient, temporalSDKClient, temporalWorker, handler, routerConsumer, deliveryPoller, nil
}
