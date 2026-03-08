package stripe

import (
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/provider"
)

func validateConfig(config map[string]any) error {
	var cfg connectorinstance.StripeConfig
	if err := provider.DecodeInto(config, &cfg); err != nil {
		return connectorinstance.ErrInvalidConfig
	}
	cfg.StripeAccountID = strings.TrimSpace(cfg.StripeAccountID)
	cfg.WebhookSecret = strings.TrimSpace(cfg.WebhookSecret)
	if cfg.StripeAccountID == "" {
		return connectorinstance.ErrMissingStripeAccount
	}
	if cfg.WebhookSecret == "" {
		return connectorinstance.ErrMissingWebhookSecret
	}
	return provider.RewriteConfig(config, cfg)
}
