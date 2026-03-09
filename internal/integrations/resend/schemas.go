package resend

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return []integration.SchemaSpec{
		{
			EventType:  "resend.email.received",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: integration.MarshalSchema(integration.ObjectSchema(map[string]any{}, true)),
		},
		{
			EventType:  "resend.send_email.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("resend", "send_email", true, integration.ObjectSchema(map[string]any{
				"email_id": integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "resend.send_email.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("resend", "send_email", false, nil),
		},
	}
}
