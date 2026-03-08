package schema

import "encoding/json"

func CoreBundles() []Bundle {
	return []Bundle{
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

func resultSpec(connector, operation string, version int, success bool, outputSchema map[string]any) Spec {
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
		properties["status"] = enumStringSchema("succeeded")
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
		EventType:  connector + "." + operation + map[bool]string{true: ".completed", false: ".failed"}[success],
		Version:    version,
		Source:     connector,
		SourceKind: "internal",
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

func stringSchema() map[string]any         { return map[string]any{"type": "string"} }
func integerSchema() map[string]any        { return map[string]any{"type": "integer"} }
func nullableStringSchema() map[string]any { return map[string]any{"type": []string{"string", "null"}} }
func nullableIntegerSchema() map[string]any {
	return map[string]any{"type": []string{"integer", "null"}}
}
func enumStringSchema(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}
