package slack

import (
	"encoding/json"
	"strings"

	"groot/internal/connection"
	"groot/internal/integrations"
)

func validateConfig(config map[string]any) error {
	var cfg connection.SlackConfig
	if err := integration.DecodeInto(config, &cfg); err != nil {
		return connection.ErrInvalidConfig
	}
	cfg.BotToken = strings.TrimSpace(cfg.BotToken)
	cfg.DefaultChannel = strings.TrimSpace(cfg.DefaultChannel)
	if cfg.BotToken == "" {
		return connection.ErrMissingBotToken
	}
	return integration.RewriteConfig(config, cfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
