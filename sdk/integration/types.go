package integration

import "encoding/json"

func MarshalSchema(schema map[string]any) json.RawMessage {
	body, _ := json.Marshal(schema)
	return body
}

func ObjectSchema(properties map[string]any, allowAdditional bool) map[string]any {
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

func StringSchema() map[string]any         { return map[string]any{"type": "string"} }
func IntegerSchema() map[string]any        { return map[string]any{"type": "integer"} }
func BooleanSchema() map[string]any        { return map[string]any{"type": "boolean"} }
func NullableStringSchema() map[string]any { return map[string]any{"type": []string{"string", "null"}} }
func NullableIntegerSchema() map[string]any {
	return map[string]any{"type": []string{"integer", "null"}}
}

func EnumStringSchema(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}

func ResultEventSchema(connector, operation string, success bool, outputSchema map[string]any) json.RawMessage {
	statusValue := "failed"
	properties := map[string]any{
		"input_event_id":   StringSchema(),
		"subscription_id":  StringSchema(),
		"delivery_job_id":  StringSchema(),
		"integration_name":   StringSchema(),
		"operation":        StringSchema(),
		"status":           EnumStringSchema(statusValue),
		"external_id":      NullableStringSchema(),
		"http_status_code": NullableIntegerSchema(),
		"output":           ObjectSchema(map[string]any{}, false),
	}
	required := []string{"input_event_id", "subscription_id", "delivery_job_id", "integration_name", "operation", "status", "output"}
	if success {
		properties["status"] = EnumStringSchema("succeeded")
		if outputSchema != nil {
			properties["output"] = outputSchema
		}
	} else {
		properties["error"] = ObjectSchema(map[string]any{
			"message": StringSchema(),
			"type":    StringSchema(),
		}, false)
		required = append(required, "error")
	}
	return MarshalSchema(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	})
}
