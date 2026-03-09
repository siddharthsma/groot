package stripe

import (
	"strings"

	"groot/internal/connection"
	"groot/internal/integrations"
)

func validateConfig(config map[string]any) error {
	var cfg connection.StripeConfig
	if err := integration.DecodeInto(config, &cfg); err != nil {
		return connection.ErrInvalidConfig
	}
	cfg.StripeAccountID = strings.TrimSpace(cfg.StripeAccountID)
	cfg.WebhookSecret = strings.TrimSpace(cfg.WebhookSecret)
	if cfg.StripeAccountID == "" {
		return connection.ErrMissingStripeAccount
	}
	if cfg.WebhookSecret == "" {
		return connection.ErrMissingWebhookSecret
	}
	return integration.RewriteConfig(config, cfg)
}
