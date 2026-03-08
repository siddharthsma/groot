package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("GROOT_EDITION", "internal")
	t.Setenv("GROOT_TENANCY_MODE", "multi")
	t.Setenv("GROOT_HTTP_ADDR", ":8081")
	t.Setenv("POSTGRES_DSN", "postgres://groot:groot@postgres:5432/groot?sslmode=disable")
	t.Setenv("KAFKA_BROKERS", "kafka:19092,kafka-2:19092")
	t.Setenv("ROUTER_CONSUMER_GROUP", "phase20-router")
	t.Setenv("TEMPORAL_ADDRESS", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "default")
	t.Setenv("GROOT_DELIVERY_TASK_QUEUE", "phase20-delivery")
	t.Setenv("GROOT_SYSTEM_API_KEY", "system-secret")
	t.Setenv("DELIVERY_MAX_ATTEMPTS", "3")
	t.Setenv("DELIVERY_INITIAL_INTERVAL", "1s")
	t.Setenv("DELIVERY_MAX_INTERVAL", "10s")
	t.Setenv("MAX_REPLAY_EVENTS", "25")
	t.Setenv("MAX_REPLAY_WINDOW_HOURS", "12")
	t.Setenv("STRIPE_WEBHOOK_TOLERANCE_SECONDS", "123")
	t.Setenv("OPENAI_API_KEY", "openai-secret")
	t.Setenv("OPENAI_API_BASE_URL", "http://127.0.0.1:18100")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret")
	t.Setenv("ANTHROPIC_API_BASE_URL", "http://127.0.0.1:18101")
	t.Setenv("LLM_DEFAULT_PROVIDER", "anthropic")
	t.Setenv("LLM_TIMEOUT_SECONDS", "45")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("RESEND_WEBHOOK_PUBLIC_URL", "https://example.com/webhooks/resend")
	t.Setenv("RESEND_RECEIVING_DOMAIN", "example.resend.app")
	t.Setenv("RESEND_API_BASE_URL", "http://127.0.0.1:18090")
	t.Setenv("RESEND_WEBHOOK_EVENTS", "email.received,email.delivered")
	t.Setenv("SLACK_API_BASE_URL", "http://127.0.0.1:18091")
	t.Setenv("SLACK_SIGNING_SECRET", "slack-signing-secret")
	t.Setenv("NOTION_API_BASE_URL", "http://127.0.0.1:18093/v1")
	t.Setenv("NOTION_API_VERSION", "2024-01-01")
	t.Setenv("GROOT_ALLOW_GLOBAL_INSTANCES", "false")
	t.Setenv("LLM_DEFAULT_CLASSIFY_MODEL", "gpt-4o-mini")
	t.Setenv("LLM_DEFAULT_EXTRACT_MODEL", "gpt-4o-mini")
	t.Setenv("AGENT_MAX_STEPS", "9")
	t.Setenv("AGENT_STEP_TIMEOUT_SECONDS", "15")
	t.Setenv("AGENT_TOTAL_TIMEOUT_SECONDS", "90")
	t.Setenv("AGENT_MAX_TOOL_CALLS", "6")
	t.Setenv("AGENT_MAX_TOOL_OUTPUT_BYTES", "8192")
	t.Setenv("SCHEMA_VALIDATION_MODE", "reject")
	t.Setenv("SCHEMA_REGISTRATION_MODE", "migrate")
	t.Setenv("SCHEMA_MAX_PAYLOAD_BYTES", "2048")
	t.Setenv("GRAPH_MAX_NODES", "900")
	t.Setenv("GRAPH_MAX_EDGES", "1900")
	t.Setenv("GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS", "123")
	t.Setenv("GRAPH_EXECUTION_MAX_DEPTH", "17")
	t.Setenv("GRAPH_DEFAULT_LIMIT", "321")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.HTTPAddr, ":8081"; got != want {
		t.Fatalf("HTTPAddr = %q, want %q", got, want)
	}
	if got, want := string(cfg.Edition.BuildEdition), "internal"; got != want {
		t.Fatalf("Edition.BuildEdition = %q, want %q", got, want)
	}
	if got, want := string(cfg.Edition.TenancyMode), "multi"; got != want {
		t.Fatalf("Edition.TenancyMode = %q, want %q", got, want)
	}
	if got, want := cfg.License.EnforceSignature, true; got != want {
		t.Fatalf("License.EnforceSignature = %t, want %t", got, want)
	}
	if got, want := len(cfg.KafkaBrokers), 2; got != want {
		t.Fatalf("len(KafkaBrokers) = %d, want %d", got, want)
	}
	if got, want := cfg.RouterConsumerGroup, "phase20-router"; got != want {
		t.Fatalf("RouterConsumerGroup = %q, want %q", got, want)
	}
	if got, want := cfg.DeliveryTaskQueue, "phase20-delivery"; got != want {
		t.Fatalf("DeliveryTaskQueue = %q, want %q", got, want)
	}
	if got, want := cfg.DeliveryRetry.MaxAttempts, 3; got != want {
		t.Fatalf("DeliveryRetry.MaxAttempts = %d, want %d", got, want)
	}
	if got, want := cfg.Resend.APIBaseURL, "http://127.0.0.1:18090"; got != want {
		t.Fatalf("Resend.APIBaseURL = %q, want %q", got, want)
	}
	if got, want := len(cfg.Resend.WebhookEvents), 2; got != want {
		t.Fatalf("len(Resend.WebhookEvents) = %d, want %d", got, want)
	}
	if got, want := cfg.Slack.APIBaseURL, "http://127.0.0.1:18091"; got != want {
		t.Fatalf("Slack.APIBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Slack.SigningSecret, "slack-signing-secret"; got != want {
		t.Fatalf("Slack.SigningSecret = %q, want %q", got, want)
	}
	if got, want := cfg.AllowGlobalInstances, false; got != want {
		t.Fatalf("AllowGlobalInstances = %t, want %t", got, want)
	}
	if got, want := cfg.Replay.MaxEvents, 25; got != want {
		t.Fatalf("Replay.MaxEvents = %d, want %d", got, want)
	}
	if got, want := cfg.Replay.MaxWindowHours, 12; got != want {
		t.Fatalf("Replay.MaxWindowHours = %d, want %d", got, want)
	}
	if got, want := cfg.Stripe.WebhookToleranceSeconds, 123; got != want {
		t.Fatalf("Stripe.WebhookToleranceSeconds = %d, want %d", got, want)
	}
	if got, want := cfg.Notion.APIBaseURL, "http://127.0.0.1:18093/v1"; got != want {
		t.Fatalf("Notion.APIBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Notion.APIVersion, "2024-01-01"; got != want {
		t.Fatalf("Notion.APIVersion = %q, want %q", got, want)
	}
	if got, want := cfg.LLM.OpenAIAPIBaseURL, "http://127.0.0.1:18100"; got != want {
		t.Fatalf("LLM.OpenAIAPIBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.LLM.AnthropicAPIBaseURL, "http://127.0.0.1:18101"; got != want {
		t.Fatalf("LLM.AnthropicAPIBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.LLM.DefaultProvider, "anthropic"; got != want {
		t.Fatalf("LLM.DefaultProvider = %q, want %q", got, want)
	}
	if got, want := cfg.LLM.TimeoutSeconds, 45; got != want {
		t.Fatalf("LLM.TimeoutSeconds = %d, want %d", got, want)
	}
	if got, want := cfg.LLM.DefaultClassifyModel, "gpt-4o-mini"; got != want {
		t.Fatalf("LLM.DefaultClassifyModel = %q, want %q", got, want)
	}
	if got, want := cfg.LLM.DefaultExtractModel, "gpt-4o-mini"; got != want {
		t.Fatalf("LLM.DefaultExtractModel = %q, want %q", got, want)
	}
	if got, want := cfg.Agent.MaxSteps, 9; got != want {
		t.Fatalf("Agent.MaxSteps = %d, want %d", got, want)
	}
	if got, want := int(cfg.Agent.StepTimeout.Seconds()), 15; got != want {
		t.Fatalf("Agent.StepTimeout = %d, want %d", got, want)
	}
	if got, want := int(cfg.Agent.TotalTimeout.Seconds()), 90; got != want {
		t.Fatalf("Agent.TotalTimeout = %d, want %d", got, want)
	}
	if got, want := cfg.Agent.MaxToolCalls, 6; got != want {
		t.Fatalf("Agent.MaxToolCalls = %d, want %d", got, want)
	}
	if got, want := cfg.Agent.MaxToolOutputBytes, 8192; got != want {
		t.Fatalf("Agent.MaxToolOutputBytes = %d, want %d", got, want)
	}
	if got, want := cfg.Schema.ValidationMode, "reject"; got != want {
		t.Fatalf("Schema.ValidationMode = %q, want %q", got, want)
	}
	if got, want := cfg.Schema.RegistrationMode, "migrate"; got != want {
		t.Fatalf("Schema.RegistrationMode = %q, want %q", got, want)
	}
	if got, want := cfg.Schema.MaxPayloadBytes, 2048; got != want {
		t.Fatalf("Schema.MaxPayloadBytes = %d, want %d", got, want)
	}
	if got, want := cfg.Auth.Mode, "api_key"; got != want {
		t.Fatalf("Auth.Mode = %q, want %q", got, want)
	}
	if got, want := cfg.Auth.APIKeyHeader, "X-API-Key"; got != want {
		t.Fatalf("Auth.APIKeyHeader = %q, want %q", got, want)
	}
	if got, want := cfg.Audit.Enabled, true; got != want {
		t.Fatalf("Audit.Enabled = %t, want %t", got, want)
	}
	if got, want := cfg.Admin.Enabled, false; got != want {
		t.Fatalf("Admin.Enabled = %t, want %t", got, want)
	}
	if got, want := cfg.Admin.AuthMode, "api_key"; got != want {
		t.Fatalf("Admin.AuthMode = %q, want %q", got, want)
	}
	if got, want := cfg.Graph.MaxNodes, 900; got != want {
		t.Fatalf("Graph.MaxNodes = %d, want %d", got, want)
	}
	if got, want := cfg.Graph.MaxEdges, 1900; got != want {
		t.Fatalf("Graph.MaxEdges = %d, want %d", got, want)
	}
	if got, want := cfg.Graph.ExecutionTraversalMaxEvents, 123; got != want {
		t.Fatalf("Graph.ExecutionTraversalMaxEvents = %d, want %d", got, want)
	}
	if got, want := cfg.Graph.ExecutionMaxDepth, 17; got != want {
		t.Fatalf("Graph.ExecutionMaxDepth = %d, want %d", got, want)
	}
	if got, want := cfg.Graph.DefaultLimit, 321; got != want {
		t.Fatalf("Graph.DefaultLimit = %d, want %d", got, want)
	}
}

func TestLoadRejectsInvalidEditionTenancy(t *testing.T) {
	t.Setenv("GROOT_EDITION", "community")
	t.Setenv("GROOT_TENANCY_MODE", "multi")
	t.Setenv("GROOT_HTTP_ADDR", ":8081")
	t.Setenv("POSTGRES_DSN", "postgres://groot:groot@postgres:5432/groot?sslmode=disable")
	t.Setenv("KAFKA_BROKERS", "kafka:19092")
	t.Setenv("TEMPORAL_ADDRESS", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "default")
	t.Setenv("GROOT_SYSTEM_API_KEY", "system-secret")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("RESEND_WEBHOOK_PUBLIC_URL", "https://example.com/webhooks/resend")
	t.Setenv("RESEND_RECEIVING_DOMAIN", "example.resend.app")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadMissingEnv(t *testing.T) {
	t.Setenv("GROOT_HTTP_ADDR", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("TEMPORAL_ADDRESS", "")
	t.Setenv("TEMPORAL_NAMESPACE", "")
	t.Setenv("GROOT_SYSTEM_API_KEY", "")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("RESEND_WEBHOOK_PUBLIC_URL", "")
	t.Setenv("RESEND_RECEIVING_DOMAIN", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
