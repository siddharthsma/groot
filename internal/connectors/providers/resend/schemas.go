package resend

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
	return []provider.SchemaSpec{
		{
			EventType:  "resend.email.received",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: provider.MarshalSchema(provider.ObjectSchema(map[string]any{}, true)),
		},
		{
			EventType:  "resend.send_email.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("resend", "send_email", true, provider.ObjectSchema(map[string]any{
				"email_id": provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "resend.send_email.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("resend", "send_email", false, nil),
		},
	}
}
