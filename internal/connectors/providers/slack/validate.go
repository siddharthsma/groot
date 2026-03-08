package slack

import (
	"encoding/json"
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/provider"
)

func validateConfig(config map[string]any) error {
	var cfg connectorinstance.SlackConfig
	if err := provider.DecodeInto(config, &cfg); err != nil {
		return connectorinstance.ErrInvalidConfig
	}
	cfg.BotToken = strings.TrimSpace(cfg.BotToken)
	cfg.DefaultChannel = strings.TrimSpace(cfg.DefaultChannel)
	if cfg.BotToken == "" {
		return connectorinstance.ErrMissingBotToken
	}
	return provider.RewriteConfig(config, cfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
