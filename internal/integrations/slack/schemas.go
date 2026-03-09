package slack

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return []integration.SchemaSpec{
		{
			EventType:  "slack.message.created",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: integration.MarshalSchema(integration.ObjectSchema(map[string]any{
				"user":    integration.StringSchema(),
				"channel": integration.StringSchema(),
				"text":    integration.StringSchema(),
				"ts":      integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.app_mentioned",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: integration.MarshalSchema(integration.ObjectSchema(map[string]any{
				"user":    integration.StringSchema(),
				"channel": integration.StringSchema(),
				"text":    integration.StringSchema(),
				"ts":      integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.reaction.added",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: integration.MarshalSchema(integration.ObjectSchema(map[string]any{
				"user":    integration.StringSchema(),
				"channel": integration.StringSchema(),
				"text":    integration.StringSchema(),
				"ts":      integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.post_message.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("slack", "post_message", true, integration.ObjectSchema(map[string]any{
				"channel": integration.StringSchema(),
				"ts":      integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.post_message.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("slack", "post_message", false, nil),
		},
		{
			EventType:  "slack.create_thread_reply.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("slack", "create_thread_reply", true, integration.ObjectSchema(map[string]any{
				"channel": integration.StringSchema(),
				"ts":      integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.create_thread_reply.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("slack", "create_thread_reply", false, nil),
		},
	}
}
