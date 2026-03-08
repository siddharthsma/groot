package stripe

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
	return []provider.SchemaSpec{
		{
			EventType:  "stripe.payment_intent.succeeded",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: provider.MarshalSchema(provider.ObjectSchema(map[string]any{}, true)),
		},
	}
}
