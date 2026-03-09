package resend

import (
	"encoding/json"

	"groot/internal/integrations"
)

func validateConfig(config map[string]any) error {
	return integration.RewriteConfig(config, map[string]any{})
}

func mustMarshalConfig(config map[string]any) json.RawMessage {
	body, _ := json.Marshal(config)
	return body
}
