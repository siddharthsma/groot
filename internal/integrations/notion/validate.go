package notion

import (
	"encoding/json"
	"strings"

	"groot/internal/connection"
	"groot/internal/integrations"
)

func validateConfig(config map[string]any) error {
	var cfg connection.NotionConfig
	if err := integration.DecodeInto(config, &cfg); err != nil {
		return connection.ErrInvalidConfig
	}
	cfg.IntegrationToken = strings.TrimSpace(cfg.IntegrationToken)
	if cfg.IntegrationToken == "" {
		return connection.ErrMissingNotionToken
	}
	return integration.RewriteConfig(config, cfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
