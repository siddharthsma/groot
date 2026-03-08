package agent

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	eventpkg "groot/internal/event"
)

func ResolveSessionKey(template string, event eventpkg.Event) string {
	rendered := strings.TrimSpace(template)
	for token, replacement := range buildTemplateReplacements(event) {
		rendered = strings.ReplaceAll(rendered, token, replacement)
	}
	return strings.TrimSpace(rendered)
}

func buildTemplateReplacements(event eventpkg.Event) map[string]string {
	replacements := map[string]string{
		"{{event_id}}":  event.EventID.String(),
		"{{tenant_id}}": event.TenantID.String(),
		"{{type}}":      event.Type,
		"{{source}}":    event.Source,
		"{{timestamp}}": event.Timestamp.UTC().Format(time.RFC3339),
	}
	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err == nil {
		collectPayloadReplacements(replacements, "payload", payload)
	}
	return replacements
}

func collectPayloadReplacements(replacements map[string]string, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			collectPayloadReplacements(replacements, prefix+"."+key, nested)
		}
	case []any:
		for i, nested := range typed {
			collectPayloadReplacements(replacements, prefix+"."+strconv.Itoa(i), nested)
		}
	case string:
		replacements["{{"+prefix+"}}"] = typed
	case bool:
		replacements["{{"+prefix+"}}"] = strconv.FormatBool(typed)
	case float64:
		replacements["{{"+prefix+"}}"] = strconv.FormatFloat(typed, 'f', -1, 64)
	}
}
