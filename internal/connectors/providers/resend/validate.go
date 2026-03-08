package resend

import (
	"encoding/json"

	"groot/internal/connectors/provider"
)

func validateConfig(config map[string]any) error {
	return provider.RewriteConfig(config, map[string]any{})
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
