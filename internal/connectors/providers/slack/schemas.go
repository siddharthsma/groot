package slack

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
	return []provider.SchemaSpec{
		{
			EventType:  "slack.message.created",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: provider.MarshalSchema(provider.ObjectSchema(map[string]any{
				"user":    provider.StringSchema(),
				"channel": provider.StringSchema(),
				"text":    provider.StringSchema(),
				"ts":      provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.app_mentioned",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: provider.MarshalSchema(provider.ObjectSchema(map[string]any{
				"user":    provider.StringSchema(),
				"channel": provider.StringSchema(),
				"text":    provider.StringSchema(),
				"ts":      provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.reaction.added",
			Version:    1,
			SourceKind: "external",
			SchemaJSON: provider.MarshalSchema(provider.ObjectSchema(map[string]any{
				"user":    provider.StringSchema(),
				"channel": provider.StringSchema(),
				"text":    provider.StringSchema(),
				"ts":      provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.post_message.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("slack", "post_message", true, provider.ObjectSchema(map[string]any{
				"channel": provider.StringSchema(),
				"ts":      provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.post_message.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("slack", "post_message", false, nil),
		},
		{
			EventType:  "slack.create_thread_reply.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("slack", "create_thread_reply", true, provider.ObjectSchema(map[string]any{
				"channel": provider.StringSchema(),
				"ts":      provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "slack.create_thread_reply.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("slack", "create_thread_reply", false, nil),
		},
	}
}
