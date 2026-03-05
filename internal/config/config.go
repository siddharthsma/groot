package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr          string
	PostgresDSN       string
	KafkaBrokers      []string
	TemporalAddress   string
	TemporalNamespace string
}

func Load() (Config, error) {
	var cfg Config

	var err error
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

	return cfg, nil
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
