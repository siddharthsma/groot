package event

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func BuildTemplateReplacements(event Event) map[string]string {
	replacements := map[string]string{
		"{{event_id}}":                   event.EventID.String(),
		"{{tenant_id}}":                  event.TenantID.String(),
		"{{workflow_run_id}}":            optionalUUID(event.WorkflowRunID),
		"{{workflow_node_id}}":           event.WorkflowNodeID,
		"{{type}}":                       event.Type,
		"{{source}}":                     event.SourceIntegration(),
		"{{source.kind}}":                event.Source.Kind,
		"{{source.integration}}":         event.SourceIntegration(),
		"{{source.connection_id}}":       optionalUUID(event.Source.ConnectionID),
		"{{source.connection_name}}":     event.Source.ConnectionName,
		"{{source.external_account_id}}": event.Source.ExternalAccountID,
		"{{timestamp}}":                  event.Timestamp.UTC().Format(time.RFC3339),
	}
	if event.Lineage != nil {
		replacements["{{lineage.integration}}"] = event.Lineage.Integration
		replacements["{{lineage.connection_id}}"] = optionalUUID(event.Lineage.ConnectionID)
		replacements["{{lineage.connection_name}}"] = event.Lineage.ConnectionName
		replacements["{{lineage.external_account_id}}"] = event.Lineage.ExternalAccountID
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

func optionalUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
