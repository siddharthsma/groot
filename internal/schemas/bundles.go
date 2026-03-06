package schemas

import (
	"encoding/json"

	"groot/internal/stream"
)

func DefaultBundles() []Bundle {
	return []Bundle{
		{
			Name: "resend",
			Schemas: []Spec{
				spec("resend.email.received", 1, "resend", stream.SourceKindExternal, objectSchema(map[string]any{}, true)),
				resultSpec("resend", "send_email", 1, true, objectSchema(map[string]any{
					"email_id": stringSchema(),
				}, false)),
				resultSpec("resend", "send_email", 1, false, nil),
			},
		},
		{
			Name: "slack",
			Schemas: []Spec{
				spec("slack.message.created", 1, "slack", stream.SourceKindExternal, objectSchema(map[string]any{
					"user":    stringSchema(),
					"channel": stringSchema(),
					"text":    stringSchema(),
					"ts":      stringSchema(),
				}, false)),
				spec("slack.app_mentioned", 1, "slack", stream.SourceKindExternal, objectSchema(map[string]any{
					"user":    stringSchema(),
					"channel": stringSchema(),
					"text":    stringSchema(),
					"ts":      stringSchema(),
				}, false)),
				spec("slack.reaction.added", 1, "slack", stream.SourceKindExternal, objectSchema(map[string]any{
					"user":    stringSchema(),
					"channel": stringSchema(),
					"text":    stringSchema(),
					"ts":      stringSchema(),
				}, false)),
				resultSpec("slack", "post_message", 1, true, objectSchema(map[string]any{
					"channel": stringSchema(),
					"ts":      stringSchema(),
				}, false)),
				resultSpec("slack", "post_message", 1, false, nil),
				resultSpec("slack", "create_thread_reply", 1, true, objectSchema(map[string]any{
					"channel": stringSchema(),
					"ts":      stringSchema(),
				}, false)),
				resultSpec("slack", "create_thread_reply", 1, false, nil),
			},
		},
		{
			Name: "llm",
			Schemas: []Spec{
				resultSpec("llm", "generate", 1, true, objectSchema(map[string]any{
					"text":     stringSchema(),
					"provider": stringSchema(),
					"model":    stringSchema(),
					"usage": objectSchema(map[string]any{
						"prompt_tokens":     integerSchema(),
						"completion_tokens": integerSchema(),
						"total_tokens":      integerSchema(),
					}, false),
				}, false)),
				resultSpec("llm", "generate", 1, false, nil),
				resultSpec("llm", "summarize", 1, true, objectSchema(map[string]any{
					"text":     stringSchema(),
					"provider": stringSchema(),
					"model":    stringSchema(),
					"usage": objectSchema(map[string]any{
						"prompt_tokens":     integerSchema(),
						"completion_tokens": integerSchema(),
						"total_tokens":      integerSchema(),
					}, false),
				}, false)),
				resultSpec("llm", "summarize", 1, false, nil),
				resultSpec("llm", "classify", 1, true, objectSchema(map[string]any{
					"label": stringSchema(),
				}, false)),
				resultSpec("llm", "classify", 1, false, nil),
				resultSpec("llm", "extract", 1, true, objectSchema(map[string]any{}, true)),
				resultSpec("llm", "extract", 1, false, nil),
				resultSpec("llm", "agent", 1, true, objectSchema(map[string]any{
					"output": objectSchema(map[string]any{}, true),
					"tool_calls": map[string]any{
						"type": "array",
						"items": objectSchema(map[string]any{
							"tool": stringSchema(),
							"ok":   map[string]any{"type": "boolean"},
						}, false),
					},
				}, false)),
				resultSpec("llm", "agent", 1, false, nil),
			},
		},
		{
			Name: "notion",
			Schemas: []Spec{
				resultSpec("notion", "create_page", 1, true, objectSchema(map[string]any{
					"page_id": stringSchema(),
				}, false)),
				resultSpec("notion", "create_page", 1, false, nil),
				resultSpec("notion", "append_block", 1, true, objectSchema(map[string]any{
					"block_id": stringSchema(),
				}, false)),
				resultSpec("notion", "append_block", 1, false, nil),
			},
		},
		{
			Name: "function",
			Schemas: []Spec{
				resultSpec("function", "invoke", 1, true, objectSchema(map[string]any{
					"response_status":      integerSchema(),
					"response_body_sha256": stringSchema(),
				}, false)),
				resultSpec("function", "invoke", 1, false, nil),
			},
		},
	}
}

func spec(eventType string, version int, source string, sourceKind string, schema map[string]any) Spec {
	body, _ := json.Marshal(schema)
	return Spec{
		EventType:  eventType,
		Version:    version,
		Source:     source,
		SourceKind: sourceKind,
		SchemaJSON: body,
	}
}

func resultSpec(connector, operation string, version int, success bool, outputSchema map[string]any) Spec {
	eventType := connector + "." + operation + ".failed"
	statusValue := "failed"
	properties := map[string]any{
		"input_event_id":   stringSchema(),
		"subscription_id":  stringSchema(),
		"delivery_job_id":  stringSchema(),
		"connector_name":   stringSchema(),
		"operation":        stringSchema(),
		"status":           enumStringSchema(statusValue),
		"external_id":      nullableStringSchema(),
		"http_status_code": nullableIntegerSchema(),
		"output":           objectSchema(map[string]any{}, false),
	}
	required := []string{"input_event_id", "subscription_id", "delivery_job_id", "connector_name", "operation", "status", "output"}
	if success {
		eventType = connector + "." + operation + ".completed"
		statusValue = "succeeded"
		properties["status"] = enumStringSchema(statusValue)
		if outputSchema != nil {
			properties["output"] = outputSchema
		}
	} else {
		properties["error"] = objectSchema(map[string]any{
			"message": stringSchema(),
			"type":    stringSchema(),
		}, false)
		required = append(required, "error")
	}
	body, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	})
	return Spec{
		EventType:  eventType,
		Version:    version,
		Source:     connector,
		SourceKind: stream.SourceKindInternal,
		SchemaJSON: body,
	}
}

func objectSchema(properties map[string]any, allowAdditional bool) map[string]any {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": allowAdditional,
		"properties":           properties,
		"required":             required,
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func integerSchema() map[string]any {
	return map[string]any{"type": "integer"}
}

func nullableStringSchema() map[string]any {
	return map[string]any{"type": []string{"string", "null"}}
}

func nullableIntegerSchema() map[string]any {
	return map[string]any{"type": []string{"integer", "null"}}
}

func enumStringSchema(value string) map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []string{value},
	}
}
