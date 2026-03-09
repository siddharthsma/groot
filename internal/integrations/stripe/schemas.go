package stripe

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return []integration.SchemaSpec{
		{
			EventType:  "stripe.payment_intent.succeeded",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: integration.MarshalSchema(integration.ObjectSchema(map[string]any{}, true)),
		},
	}
}
