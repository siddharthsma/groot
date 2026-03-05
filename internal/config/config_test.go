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
}

func TestLoadMissingEnv(t *testing.T) {
	t.Setenv("GROOT_HTTP_ADDR", "")
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("TEMPORAL_ADDRESS", "")
	t.Setenv("TEMPORAL_NAMESPACE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
