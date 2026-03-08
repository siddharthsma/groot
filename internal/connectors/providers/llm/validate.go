package llm

import (
	"encoding/json"
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/provider"
)

func validateConfig(config map[string]any) error {
	var llmCfg connectorinstance.LLMConfig
	if err := provider.DecodeInto(config, &llmCfg); err != nil {
		return connectorinstance.ErrInvalidConfig
	}
	llmCfg.DefaultProvider = strings.TrimSpace(llmCfg.DefaultProvider)
	if llmCfg.Providers == nil {
		llmCfg.Providers = map[string]connectorinstance.LLMProviderConfig{}
	}
	if len(llmCfg.Providers) == 0 {
		return connectorinstance.ErrMissingLLMProviders
	}
	for name, cfg := range llmCfg.Providers {
		if !isSupportedLLMProvider(name) {
			return connectorinstance.ErrInvalidConfig
		}
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		if cfg.APIKey == "" {
			return connectorinstance.ErrMissingLLMAPIKey
		}
		llmCfg.Providers[name] = cfg
	}
	if _, ok := llmCfg.Providers[llmCfg.DefaultProvider]; !ok {
		return connectorinstance.ErrInvalidLLMProvider
	}
	return provider.RewriteConfig(config, llmCfg)
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}

func isSupportedLLMProvider(name string) bool {
	switch strings.TrimSpace(name) {
	case "openai", "anthropic":
		return true
	default:
		return false
	}
}
