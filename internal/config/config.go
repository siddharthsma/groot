package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"groot/internal/edition"
)

type Config struct {
	HTTPAddr                   string
	Edition                    edition.Runtime
	License                    edition.LicenseConfig
	CommunityTenantName        string
	IntegrationPluginDir       string
	IntegrationTrustedKeysPath string
	IntegrationInstalledPath   string
	IntegrationCacheDir        string
	IntegrationRegistryURL     string
	PostgresDSN                string
	KafkaBrokers               []string
	RouterConsumerGroup        string
	TemporalAddress            string
	TemporalNamespace          string
	DeliveryTaskQueue          string
	Auth                       AuthConfig
	Admin                      AdminConfig
	Audit                      AuditConfig
	DeliveryRetry              DeliveryRetryConfig
	Agent                      AgentConfig
	AgentRuntime               AgentRuntimeConfig
	Replay                     ReplayConfig
	MaxChainDepth              int
	AllowGlobalInstances       bool
	SystemAPIKey               string
	Stripe                     StripeConfig
	Resend                     ResendConfig
	Slack                      SlackConfig
	Notion                     NotionConfig
	LLM                        LLMConfig
	Schema                     SchemaConfig
	Graph                      GraphConfig
}

type DeliveryRetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
}

type AgentConfig struct {
	MaxSteps           int
	StepTimeout        time.Duration
	TotalTimeout       time.Duration
	MaxToolCalls       int
	MaxToolOutputBytes int
}

type AgentRuntimeConfig struct {
	Enabled               bool
	BaseURL               string
	Timeout               time.Duration
	AutoCreate            bool
	MaxIdleDays           int
	MemoryMode            string
	MemorySummaryMaxBytes int
	SharedSecret          string
}

type ResendConfig struct {
	APIKey           string
	APIBaseURL       string
	WebhookPublicURL string
	ReceivingDomain  string
	WebhookEvents    []string
}

type StripeConfig struct {
	WebhookToleranceSeconds int
}

type SlackConfig struct {
	APIBaseURL    string
	SigningSecret string
}

type NotionConfig struct {
	APIBaseURL string
	APIVersion string
}

type LLMConfig struct {
	OpenAIAPIKey         string
	OpenAIAPIBaseURL     string
	AnthropicAPIKey      string
	AnthropicAPIBaseURL  string
	DefaultIntegration   string
	DefaultClassifyModel string
	DefaultExtractModel  string
	TimeoutSeconds       int
}

type ReplayConfig struct {
	MaxEvents      int
	MaxWindowHours int
}

type SchemaConfig struct {
	ValidationMode   string
	RegistrationMode string
	MaxPayloadBytes  int
}

type AuthConfig struct {
	Mode              string
	APIKeyHeader      string
	TenantHeader      string
	ActorIDHeader     string
	ActorTypeHeader   string
	ActorEmailHeader  string
	JWTJWKSURL        string
	JWTAudience       string
	JWTIssuer         string
	JWTRequiredClaims []string
	JWTTenantClaim    string
	JWTClockSkew      time.Duration
}

type AuditConfig struct {
	Enabled        bool
	LogRequestBody bool
}

type AdminConfig struct {
	Enabled           bool
	AuthMode          string
	APIKey            string
	JWTJWKSURL        string
	JWTIssuer         string
	JWTAudience       string
	JWTRequiredClaims []string
	AllowViewPayloads bool
	ReplayEnabled     bool
	ReplayMaxEvents   int
	RateLimitRPS      int
}

type GraphConfig struct {
	MaxNodes                    int
	MaxEdges                    int
	ExecutionTraversalMaxEvents int
	ExecutionMaxDepth           int
	DefaultLimit                int
}

func Load() (Config, error) {
	var cfg Config

	var err error
	if cfg.Edition, err = edition.Parse(os.Getenv("GROOT_EDITION"), os.Getenv("GROOT_TENANCY_MODE")); err != nil {
		return Config{}, err
	}
	cfg.License = loadLicenseConfig()
	cfg.CommunityTenantName = strings.TrimSpace(os.Getenv("COMMUNITY_TENANT_NAME"))
	cfg.IntegrationPluginDir = strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_PLUGIN_DIR"))
	if cfg.IntegrationPluginDir == "" {
		cfg.IntegrationPluginDir = "integrations/plugins"
	}
	cfg.IntegrationTrustedKeysPath = strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_TRUSTED_KEYS_PATH"))
	if cfg.IntegrationTrustedKeysPath == "" {
		cfg.IntegrationTrustedKeysPath = "integrations/trusted_keys.json"
	}
	cfg.IntegrationInstalledPath = strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_INSTALLED_PATH"))
	if cfg.IntegrationInstalledPath == "" {
		cfg.IntegrationInstalledPath = "integrations/installed.json"
	}
	cfg.IntegrationCacheDir = strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_CACHE_DIR"))
	if cfg.IntegrationCacheDir == "" {
		cfg.IntegrationCacheDir = "integrations/cache"
	}
	cfg.IntegrationRegistryURL = strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_REGISTRY_URL"))
	if cfg.HTTPAddr, err = requiredEnv("GROOT_HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.PostgresDSN, err = requiredEnv("POSTGRES_DSN"); err != nil {
		return Config{}, err
	}

	kafkaBrokers, err := requiredEnv("KAFKA_BROKERS")
	if err != nil {
		return Config{}, err
	}
	cfg.KafkaBrokers = splitAndTrim(kafkaBrokers)
	if len(cfg.KafkaBrokers) == 0 {
		return Config{}, fmt.Errorf("KAFKA_BROKERS must contain at least one broker")
	}

	if cfg.TemporalAddress, err = requiredEnv("TEMPORAL_ADDRESS"); err != nil {
		return Config{}, err
	}
	if cfg.TemporalNamespace, err = requiredEnv("TEMPORAL_NAMESPACE"); err != nil {
		return Config{}, err
	}
	cfg.DeliveryTaskQueue = strings.TrimSpace(os.Getenv("GROOT_DELIVERY_TASK_QUEUE"))
	if cfg.DeliveryTaskQueue == "" {
		cfg.DeliveryTaskQueue = "groot-delivery"
	}
	cfg.RouterConsumerGroup = strings.TrimSpace(os.Getenv("ROUTER_CONSUMER_GROUP"))
	if cfg.RouterConsumerGroup == "" {
		cfg.RouterConsumerGroup = "groot-router"
	}
	if cfg.SystemAPIKey, err = requiredEnv("GROOT_SYSTEM_API_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.Auth, err = loadAuthConfig(); err != nil {
		return Config{}, err
	}
	if cfg.Admin, err = loadAdminConfig(); err != nil {
		return Config{}, err
	}
	cfg.Audit = loadAuditConfig()
	cfg.AllowGlobalInstances = boolEnv("GROOT_ALLOW_GLOBAL_INSTANCES", true)
	if cfg.DeliveryRetry, err = loadDeliveryRetryConfig(); err != nil {
		return Config{}, err
	}
	if cfg.Agent, err = loadAgentConfig(); err != nil {
		return Config{}, err
	}
	if cfg.AgentRuntime, err = loadAgentRuntimeConfig(); err != nil {
		return Config{}, err
	}
	if cfg.Replay, err = loadReplayConfig(); err != nil {
		return Config{}, err
	}
	if cfg.MaxChainDepth, err = intEnv("MAX_CHAIN_DEPTH", 10); err != nil {
		return Config{}, err
	}
	if cfg.MaxChainDepth < 0 {
		return Config{}, fmt.Errorf("MAX_CHAIN_DEPTH must be at least 0")
	}
	if cfg.Stripe, err = loadStripeConfig(); err != nil {
		return Config{}, err
	}
	cfg.LLM = loadLLMConfig()
	if cfg.Resend, err = loadResendConfig(); err != nil {
		return Config{}, err
	}
	cfg.Slack = loadSlackConfig()
	cfg.Notion = loadNotionConfig()
	if cfg.Schema, err = loadSchemaConfig(); err != nil {
		return Config{}, err
	}
	if cfg.Graph, err = loadGraphConfig(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadLicenseConfig() edition.LicenseConfig {
	return edition.LicenseConfig{
		Path:             strings.TrimSpace(os.Getenv("GROOT_LICENSE_PATH")),
		Required:         boolEnv("GROOT_LICENSE_REQUIRED", false),
		PublicKeyPath:    strings.TrimSpace(os.Getenv("GROOT_LICENSE_PUBLIC_KEY_PATH")),
		EnforceSignature: boolEnv("GROOT_LICENSE_ENFORCE_SIGNATURE", true),
	}
}

func loadAuthConfig() (AuthConfig, error) {
	mode := strings.TrimSpace(os.Getenv("AUTH_MODE"))
	if mode == "" {
		mode = "api_key"
	}
	switch mode {
	case "api_key", "jwt", "mixed":
	default:
		return AuthConfig{}, fmt.Errorf("AUTH_MODE must be api_key, jwt, or mixed")
	}
	clockSkewSeconds, err := intEnv("JWT_CLOCK_SKEW_SECONDS", 60)
	if err != nil {
		return AuthConfig{}, err
	}
	if clockSkewSeconds < 0 {
		return AuthConfig{}, fmt.Errorf("JWT_CLOCK_SKEW_SECONDS must be at least 0")
	}
	apiKeyHeader := strings.TrimSpace(os.Getenv("API_KEY_HEADER"))
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}
	tenantHeader := strings.TrimSpace(os.Getenv("TENANT_HEADER"))
	if tenantHeader == "" {
		tenantHeader = "X-Tenant-Id"
	}
	actorIDHeader := strings.TrimSpace(os.Getenv("ACTOR_ID_HEADER"))
	if actorIDHeader == "" {
		actorIDHeader = "X-Actor-Id"
	}
	actorTypeHeader := strings.TrimSpace(os.Getenv("ACTOR_TYPE_HEADER"))
	if actorTypeHeader == "" {
		actorTypeHeader = "X-Actor-Type"
	}
	actorEmailHeader := strings.TrimSpace(os.Getenv("ACTOR_EMAIL_HEADER"))
	if actorEmailHeader == "" {
		actorEmailHeader = "X-Actor-Email"
	}
	requiredClaims := splitAndTrim(strings.TrimSpace(os.Getenv("JWT_REQUIRED_CLAIMS")))
	if len(requiredClaims) == 0 {
		requiredClaims = []string{"sub", "tenant_id"}
	}
	tenantClaim := strings.TrimSpace(os.Getenv("JWT_TENANT_CLAIM"))
	if tenantClaim == "" {
		tenantClaim = "tenant_id"
	}
	return AuthConfig{
		Mode:              mode,
		APIKeyHeader:      apiKeyHeader,
		TenantHeader:      tenantHeader,
		ActorIDHeader:     actorIDHeader,
		ActorTypeHeader:   actorTypeHeader,
		ActorEmailHeader:  actorEmailHeader,
		JWTJWKSURL:        strings.TrimSpace(os.Getenv("JWT_JWKS_URL")),
		JWTAudience:       strings.TrimSpace(os.Getenv("JWT_AUDIENCE")),
		JWTIssuer:         strings.TrimSpace(os.Getenv("JWT_ISSUER")),
		JWTRequiredClaims: requiredClaims,
		JWTTenantClaim:    tenantClaim,
		JWTClockSkew:      time.Duration(clockSkewSeconds) * time.Second,
	}, nil
}

func loadAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:        boolEnv("AUDIT_ENABLED", true),
		LogRequestBody: boolEnv("AUDIT_LOG_REQUEST_BODY", false),
	}
}

func loadAdminConfig() (AdminConfig, error) {
	enabled := boolEnv("ADMIN_MODE_ENABLED", false)
	mode := strings.TrimSpace(os.Getenv("ADMIN_AUTH_MODE"))
	if mode == "" {
		mode = "api_key"
	}
	switch mode {
	case "api_key", "jwt":
	default:
		return AdminConfig{}, fmt.Errorf("ADMIN_AUTH_MODE must be api_key or jwt")
	}
	requiredClaims := splitAndTrim(strings.TrimSpace(os.Getenv("ADMIN_JWT_REQUIRED_CLAIMS")))
	if len(requiredClaims) == 0 {
		requiredClaims = []string{"sub"}
	}
	replayMaxEvents, err := intEnv("ADMIN_REPLAY_MAX_EVENTS", 100)
	if err != nil {
		return AdminConfig{}, err
	}
	if replayMaxEvents < 1 {
		return AdminConfig{}, fmt.Errorf("ADMIN_REPLAY_MAX_EVENTS must be at least 1")
	}
	rateLimitRPS, err := intEnv("ADMIN_RATE_LIMIT_RPS", 5)
	if err != nil {
		return AdminConfig{}, err
	}
	if rateLimitRPS < 1 {
		return AdminConfig{}, fmt.Errorf("ADMIN_RATE_LIMIT_RPS must be at least 1")
	}
	cfg := AdminConfig{
		Enabled:           enabled,
		AuthMode:          mode,
		APIKey:            strings.TrimSpace(os.Getenv("ADMIN_API_KEY")),
		JWTJWKSURL:        strings.TrimSpace(os.Getenv("ADMIN_JWT_JWKS_URL")),
		JWTIssuer:         strings.TrimSpace(os.Getenv("ADMIN_JWT_ISSUER")),
		JWTAudience:       strings.TrimSpace(os.Getenv("ADMIN_JWT_AUDIENCE")),
		JWTRequiredClaims: requiredClaims,
		AllowViewPayloads: boolEnv("ADMIN_ALLOW_VIEW_PAYLOADS", false),
		ReplayEnabled:     boolEnv("ADMIN_REPLAY_ENABLED", true),
		ReplayMaxEvents:   replayMaxEvents,
		RateLimitRPS:      rateLimitRPS,
	}
	if !enabled {
		return cfg, nil
	}
	switch mode {
	case "api_key":
		if cfg.APIKey == "" {
			return AdminConfig{}, fmt.Errorf("ADMIN_API_KEY is required when admin mode uses api_key auth")
		}
	case "jwt":
		if cfg.JWTJWKSURL == "" {
			return AdminConfig{}, fmt.Errorf("ADMIN_JWT_JWKS_URL is required when admin mode uses jwt auth")
		}
	}
	return cfg, nil
}

func loadGraphConfig() (GraphConfig, error) {
	maxNodes, err := intEnv("GRAPH_MAX_NODES", 5000)
	if err != nil {
		return GraphConfig{}, err
	}
	maxEdges, err := intEnv("GRAPH_MAX_EDGES", 20000)
	if err != nil {
		return GraphConfig{}, err
	}
	maxTraversalEvents, err := intEnv("GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS", 500)
	if err != nil {
		return GraphConfig{}, err
	}
	maxDepth, err := intEnv("GRAPH_EXECUTION_MAX_DEPTH", 25)
	if err != nil {
		return GraphConfig{}, err
	}
	defaultLimit, err := intEnv("GRAPH_DEFAULT_LIMIT", 500)
	if err != nil {
		return GraphConfig{}, err
	}
	if maxNodes < 1 || maxEdges < 1 || maxTraversalEvents < 1 || maxDepth < 1 || defaultLimit < 1 {
		return GraphConfig{}, fmt.Errorf("graph limits must be at least 1")
	}
	return GraphConfig{
		MaxNodes:                    maxNodes,
		MaxEdges:                    maxEdges,
		ExecutionTraversalMaxEvents: maxTraversalEvents,
		ExecutionMaxDepth:           maxDepth,
		DefaultLimit:                defaultLimit,
	}, nil
}

func loadReplayConfig() (ReplayConfig, error) {
	maxEvents, err := intEnv("MAX_REPLAY_EVENTS", 1000)
	if err != nil {
		return ReplayConfig{}, err
	}
	if maxEvents < 1 {
		return ReplayConfig{}, fmt.Errorf("MAX_REPLAY_EVENTS must be at least 1")
	}
	maxWindowHours, err := intEnv("MAX_REPLAY_WINDOW_HOURS", 24)
	if err != nil {
		return ReplayConfig{}, err
	}
	if maxWindowHours < 1 {
		return ReplayConfig{}, fmt.Errorf("MAX_REPLAY_WINDOW_HOURS must be at least 1")
	}
	return ReplayConfig{MaxEvents: maxEvents, MaxWindowHours: maxWindowHours}, nil
}

func loadAgentConfig() (AgentConfig, error) {
	maxSteps, err := intEnv("AGENT_MAX_STEPS", 8)
	if err != nil {
		return AgentConfig{}, err
	}
	if maxSteps < 1 {
		return AgentConfig{}, fmt.Errorf("AGENT_MAX_STEPS must be at least 1")
	}
	stepTimeoutSeconds, err := intEnv("AGENT_STEP_TIMEOUT_SECONDS", 30)
	if err != nil {
		return AgentConfig{}, err
	}
	if stepTimeoutSeconds < 1 {
		return AgentConfig{}, fmt.Errorf("AGENT_STEP_TIMEOUT_SECONDS must be at least 1")
	}
	totalTimeoutSeconds, err := intEnv("AGENT_TOTAL_TIMEOUT_SECONDS", 120)
	if err != nil {
		return AgentConfig{}, err
	}
	if totalTimeoutSeconds < 1 {
		return AgentConfig{}, fmt.Errorf("AGENT_TOTAL_TIMEOUT_SECONDS must be at least 1")
	}
	maxToolCalls, err := intEnv("AGENT_MAX_TOOL_CALLS", 8)
	if err != nil {
		return AgentConfig{}, err
	}
	if maxToolCalls < 1 {
		return AgentConfig{}, fmt.Errorf("AGENT_MAX_TOOL_CALLS must be at least 1")
	}
	maxToolOutputBytes, err := intEnv("AGENT_MAX_TOOL_OUTPUT_BYTES", 16384)
	if err != nil {
		return AgentConfig{}, err
	}
	if maxToolOutputBytes < 1 {
		return AgentConfig{}, fmt.Errorf("AGENT_MAX_TOOL_OUTPUT_BYTES must be at least 1")
	}
	return AgentConfig{
		MaxSteps:           maxSteps,
		StepTimeout:        time.Duration(stepTimeoutSeconds) * time.Second,
		TotalTimeout:       time.Duration(totalTimeoutSeconds) * time.Second,
		MaxToolCalls:       maxToolCalls,
		MaxToolOutputBytes: maxToolOutputBytes,
	}, nil
}

func loadAgentRuntimeConfig() (AgentRuntimeConfig, error) {
	timeoutSeconds, err := intEnv("AGENT_RUNTIME_TIMEOUT_SECONDS", 30)
	if err != nil {
		return AgentRuntimeConfig{}, err
	}
	if timeoutSeconds < 1 {
		return AgentRuntimeConfig{}, fmt.Errorf("AGENT_RUNTIME_TIMEOUT_SECONDS must be at least 1")
	}
	maxIdleDays, err := intEnv("AGENT_SESSION_MAX_IDLE_DAYS", 30)
	if err != nil {
		return AgentRuntimeConfig{}, err
	}
	if maxIdleDays < 1 {
		return AgentRuntimeConfig{}, fmt.Errorf("AGENT_SESSION_MAX_IDLE_DAYS must be at least 1")
	}
	summaryMaxBytes, err := intEnv("AGENT_MEMORY_SUMMARY_MAX_BYTES", 16384)
	if err != nil {
		return AgentRuntimeConfig{}, err
	}
	if summaryMaxBytes < 1 {
		return AgentRuntimeConfig{}, fmt.Errorf("AGENT_MEMORY_SUMMARY_MAX_BYTES must be at least 1")
	}
	memoryMode := strings.TrimSpace(os.Getenv("AGENT_MEMORY_MODE"))
	if memoryMode == "" {
		memoryMode = "runtime_managed"
	}
	if memoryMode != "runtime_managed" {
		return AgentRuntimeConfig{}, fmt.Errorf("AGENT_MEMORY_MODE must be runtime_managed")
	}
	baseURL := strings.TrimSpace(os.Getenv("AGENT_RUNTIME_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}
	sharedSecret := strings.TrimSpace(os.Getenv("AGENT_RUNTIME_SHARED_SECRET"))
	if sharedSecret == "" {
		sharedSecret = "agent-runtime-secret"
	}
	return AgentRuntimeConfig{
		Enabled:               boolEnv("AGENT_RUNTIME_ENABLED", true),
		BaseURL:               baseURL,
		Timeout:               time.Duration(timeoutSeconds) * time.Second,
		AutoCreate:            boolEnv("AGENT_SESSION_AUTO_CREATE", true),
		MaxIdleDays:           maxIdleDays,
		MemoryMode:            memoryMode,
		MemorySummaryMaxBytes: summaryMaxBytes,
		SharedSecret:          sharedSecret,
	}, nil
}

func loadStripeConfig() (StripeConfig, error) {
	tolerance, err := intEnv("STRIPE_WEBHOOK_TOLERANCE_SECONDS", 300)
	if err != nil {
		return StripeConfig{}, err
	}
	if tolerance < 1 {
		return StripeConfig{}, fmt.Errorf("STRIPE_WEBHOOK_TOLERANCE_SECONDS must be at least 1")
	}
	return StripeConfig{WebhookToleranceSeconds: tolerance}, nil
}

func loadNotionConfig() NotionConfig {
	apiBaseURL := strings.TrimSpace(os.Getenv("NOTION_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = "https://api.notion.com/v1"
	}
	apiVersion := strings.TrimSpace(os.Getenv("NOTION_API_VERSION"))
	if apiVersion == "" {
		apiVersion = "2022-06-28"
	}
	return NotionConfig{
		APIBaseURL: apiBaseURL,
		APIVersion: apiVersion,
	}
}

func loadSlackConfig() SlackConfig {
	apiBaseURL := strings.TrimSpace(os.Getenv("SLACK_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = "https://slack.com/api"
	}
	return SlackConfig{
		APIBaseURL:    apiBaseURL,
		SigningSecret: strings.TrimSpace(os.Getenv("SLACK_SIGNING_SECRET")),
	}
}

func loadLLMConfig() LLMConfig {
	openAIBaseURL := strings.TrimSpace(os.Getenv("OPENAI_API_BASE_URL"))
	if openAIBaseURL == "" {
		openAIBaseURL = "https://api.openai.com/v1"
	}
	anthropicBaseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_API_BASE_URL"))
	if anthropicBaseURL == "" {
		anthropicBaseURL = "https://api.anthropic.com"
	}
	defaultIntegration := strings.TrimSpace(os.Getenv("LLM_DEFAULT_PROVIDER"))
	if defaultIntegration == "" {
		defaultIntegration = "openai"
	}
	defaultClassifyModel := strings.TrimSpace(os.Getenv("LLM_DEFAULT_CLASSIFY_MODEL"))
	defaultExtractModel := strings.TrimSpace(os.Getenv("LLM_DEFAULT_EXTRACT_MODEL"))
	timeoutSeconds, err := intEnv("LLM_TIMEOUT_SECONDS", 30)
	if err != nil || timeoutSeconds < 1 {
		timeoutSeconds = 30
	}
	return LLMConfig{
		OpenAIAPIKey:         strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIAPIBaseURL:     openAIBaseURL,
		AnthropicAPIKey:      strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AnthropicAPIBaseURL:  anthropicBaseURL,
		DefaultIntegration:   defaultIntegration,
		DefaultClassifyModel: defaultClassifyModel,
		DefaultExtractModel:  defaultExtractModel,
		TimeoutSeconds:       timeoutSeconds,
	}
}

func loadSchemaConfig() (SchemaConfig, error) {
	maxPayloadBytes, err := intEnv("SCHEMA_MAX_PAYLOAD_BYTES", 262144)
	if err != nil {
		return SchemaConfig{}, err
	}
	if maxPayloadBytes < 1 {
		return SchemaConfig{}, fmt.Errorf("SCHEMA_MAX_PAYLOAD_BYTES must be at least 1")
	}
	registrationMode := strings.TrimSpace(os.Getenv("SCHEMA_REGISTRATION_MODE"))
	if registrationMode == "" {
		registrationMode = "startup"
	}
	if registrationMode != "startup" && registrationMode != "migrate" {
		return SchemaConfig{}, fmt.Errorf("SCHEMA_REGISTRATION_MODE must be startup or migrate")
	}
	validationMode := strings.TrimSpace(os.Getenv("SCHEMA_VALIDATION_MODE"))
	if validationMode == "" {
		validationMode = "warn"
	}
	switch validationMode {
	case "off", "warn", "reject":
	default:
		return SchemaConfig{}, fmt.Errorf("SCHEMA_VALIDATION_MODE must be off, warn, or reject")
	}
	return SchemaConfig{
		ValidationMode:   validationMode,
		RegistrationMode: registrationMode,
		MaxPayloadBytes:  maxPayloadBytes,
	}, nil
}

func boolEnv(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func loadDeliveryRetryConfig() (DeliveryRetryConfig, error) {
	maxAttempts, err := intEnv("DELIVERY_MAX_ATTEMPTS", 10)
	if err != nil {
		return DeliveryRetryConfig{}, err
	}
	if maxAttempts < 1 {
		return DeliveryRetryConfig{}, fmt.Errorf("DELIVERY_MAX_ATTEMPTS must be at least 1")
	}

	initialInterval, err := durationEnv("DELIVERY_INITIAL_INTERVAL", 2*time.Second)
	if err != nil {
		return DeliveryRetryConfig{}, err
	}
	maxInterval, err := durationEnv("DELIVERY_MAX_INTERVAL", 5*time.Minute)
	if err != nil {
		return DeliveryRetryConfig{}, err
	}
	if initialInterval <= 0 {
		return DeliveryRetryConfig{}, fmt.Errorf("DELIVERY_INITIAL_INTERVAL must be greater than 0")
	}
	if maxInterval <= 0 {
		return DeliveryRetryConfig{}, fmt.Errorf("DELIVERY_MAX_INTERVAL must be greater than 0")
	}
	if maxInterval < initialInterval {
		return DeliveryRetryConfig{}, fmt.Errorf("DELIVERY_MAX_INTERVAL must be greater than or equal to DELIVERY_INITIAL_INTERVAL")
	}

	return DeliveryRetryConfig{
		MaxAttempts:     maxAttempts,
		InitialInterval: initialInterval,
		MaxInterval:     maxInterval,
	}, nil
}

func intEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", name, err)
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", name, err)
	}
	return parsed, nil
}

func loadResendConfig() (ResendConfig, error) {
	apiKey, err := requiredEnv("RESEND_API_KEY")
	if err != nil {
		return ResendConfig{}, err
	}
	webhookPublicURL, err := requiredEnv("RESEND_WEBHOOK_PUBLIC_URL")
	if err != nil {
		return ResendConfig{}, err
	}
	receivingDomain, err := requiredEnv("RESEND_RECEIVING_DOMAIN")
	if err != nil {
		return ResendConfig{}, err
	}

	apiBaseURL := strings.TrimSpace(os.Getenv("RESEND_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = "https://api.resend.com"
	}

	webhookEvents := splitAndTrim(strings.TrimSpace(os.Getenv("RESEND_WEBHOOK_EVENTS")))
	if len(webhookEvents) == 0 {
		webhookEvents = []string{"email.received"}
	}

	return ResendConfig{
		APIKey:           apiKey,
		APIBaseURL:       apiBaseURL,
		WebhookPublicURL: webhookPublicURL,
		ReceivingDomain:  receivingDomain,
		WebhookEvents:    webhookEvents,
	}, nil
}
