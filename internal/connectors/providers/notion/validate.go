package notion

import (
	"encoding/json"
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/provider"
)

func validateConfig(config map[string]any) error {
	var cfg connectorinstance.NotionConfig
	if err := provider.DecodeInto(config, &cfg); err != nil {
		return connectorinstance.ErrInvalidConfig
	}
	cfg.IntegrationToken = strings.TrimSpace(cfg.IntegrationToken)
	if cfg.IntegrationToken == "" {
		return connectorinstance.ErrMissingNotionToken
	}
	return provider.RewriteConfig(config, cfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
