package agent

import (
	"strings"

	eventpkg "groot/internal/event"
)

func ResolveSessionKey(template string, event eventpkg.Event) string {
	rendered := strings.TrimSpace(template)
	for token, replacement := range eventpkg.BuildTemplateReplacements(event) {
		rendered = strings.ReplaceAll(rendered, token, replacement)
	}
	return strings.TrimSpace(rendered)
}
