package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("GROOT_HTTP_ADDR", ":8081")
	t.Setenv("POSTGRES_DSN", "postgres://groot:groot@postgres:5432/groot?sslmode=disable")
	t.Setenv("KAFKA_BROKERS", "kafka:19092,kafka-2:19092")
	t.Setenv("TEMPORAL_ADDRESS", "temporal:7233")
	t.Setenv("TEMPORAL_NAMESPACE", "default")
	t.Setenv("GROOT_SYSTEM_API_KEY", "system-secret")
	t.Setenv("DELIVERY_MAX_ATTEMPTS", "3")
	t.Setenv("DELIVERY_INITIAL_INTERVAL", "1s")
	t.Setenv("DELIVERY_MAX_INTERVAL", "10s")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("RESEND_WEBHOOK_PUBLIC_URL", "https://example.com/webhooks/resend")
	t.Setenv("RESEND_RECEIVING_DOMAIN", "example.resend.app")
	t.Setenv("RESEND_API_BASE_URL", "http://127.0.0.1:18090")
	t.Setenv("RESEND_WEBHOOK_EVENTS", "email.received,email.delivered")
	t.Setenv("SLACK_API_BASE_URL", "http://127.0.0.1:18091")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.HTTPAddr, ":8081"; got != want {
		t.Fatalf("HTTPAddr = %q, want %q", got, want)
	}
	if got, want := len(cfg.KafkaBrokers), 2; got != want {
		t.Fatalf("len(KafkaBrokers) = %d, want %d", got, want)
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
	if got, want := cfg.SlackAPIBaseURL, "http://127.0.0.1:18091"; got != want {
		t.Fatalf("SlackAPIBaseURL = %q, want %q", got, want)
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
