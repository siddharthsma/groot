package llm

import (
	"encoding/json"
	"strings"

	"groot/internal/connection"
	"groot/internal/integrations"
)

func validateConfig(config map[string]any) error {
	var llmCfg connection.LLMConfig
	if err := integration.DecodeInto(config, &llmCfg); err != nil {
		return connection.ErrInvalidConfig
	}
	llmCfg.DefaultIntegration = strings.TrimSpace(llmCfg.DefaultIntegration)
	if llmCfg.Integrations == nil {
		llmCfg.Integrations = map[string]connection.LLMIntegrationConfig{}
	}
	if len(llmCfg.Integrations) == 0 {
		return connection.ErrMissingLLMIntegrations
	}
	for name, cfg := range llmCfg.Integrations {
		if !isSupportedLLMIntegration(name) {
			return connection.ErrInvalidConfig
		}
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		if cfg.APIKey == "" {
			return connection.ErrMissingLLMAPIKey
		}
		llmCfg.Integrations[name] = cfg
	}
	if _, ok := llmCfg.Integrations[llmCfg.DefaultIntegration]; !ok {
		return connection.ErrInvalidLLMIntegration
	}
	return integration.RewriteConfig(config, llmCfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}

func isSupportedLLMIntegration(name string) bool {
	switch strings.TrimSpace(name) {
	case "openai", "anthropic":
		return true
	default:
		return false
	}
}
