package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	PostgresDSN       string
	KafkaBrokers      []string
	TemporalAddress   string
	TemporalNamespace string
	DeliveryRetry     DeliveryRetryConfig
	SystemAPIKey      string
	Resend            ResendConfig
	SlackAPIBaseURL   string
}

type DeliveryRetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
}

type ResendConfig struct {
	APIKey           string
	APIBaseURL       string
	WebhookPublicURL string
	ReceivingDomain  string
	WebhookEvents    []string
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
	if cfg.SystemAPIKey, err = requiredEnv("GROOT_SYSTEM_API_KEY"); err != nil {
		return Config{}, err
	}
	if cfg.DeliveryRetry, err = loadDeliveryRetryConfig(); err != nil {
		return Config{}, err
	}
	if cfg.Resend, err = loadResendConfig(); err != nil {
		return Config{}, err
	}
	cfg.SlackAPIBaseURL = strings.TrimSpace(os.Getenv("SLACK_API_BASE_URL"))
	if cfg.SlackAPIBaseURL == "" {
		cfg.SlackAPIBaseURL = "https://slack.com/api"
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
