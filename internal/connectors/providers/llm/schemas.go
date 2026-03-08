package llm

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
	return []provider.SchemaSpec{
		{
			EventType:  "llm.generate.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("llm", "generate", true, provider.ObjectSchema(map[string]any{
				"text":     provider.StringSchema(),
				"provider": provider.StringSchema(),
				"model":    provider.StringSchema(),
				"usage": provider.ObjectSchema(map[string]any{
					"prompt_tokens":     provider.IntegerSchema(),
					"completion_tokens": provider.IntegerSchema(),
					"total_tokens":      provider.IntegerSchema(),
				}, false),
			}, false)),
		},
		{EventType: "llm.generate.failed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "generate", false, nil)},
		{
			EventType:  "llm.summarize.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("llm", "summarize", true, provider.ObjectSchema(map[string]any{
				"text":     provider.StringSchema(),
				"provider": provider.StringSchema(),
				"model":    provider.StringSchema(),
				"usage": provider.ObjectSchema(map[string]any{
					"prompt_tokens":     provider.IntegerSchema(),
					"completion_tokens": provider.IntegerSchema(),
					"total_tokens":      provider.IntegerSchema(),
				}, false),
			}, false)),
		},
		{EventType: "llm.summarize.failed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "summarize", false, nil)},
		{
			EventType:  "llm.classify.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("llm", "classify", true, provider.ObjectSchema(map[string]any{
				"label": provider.StringSchema(),
			}, false)),
		},
		{EventType: "llm.classify.failed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "classify", false, nil)},
		{EventType: "llm.extract.completed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "extract", true, provider.ObjectSchema(map[string]any{}, true))},
		{EventType: "llm.extract.failed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "extract", false, nil)},
		{EventType: "llm.agent.completed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "agent", true, provider.ObjectSchema(map[string]any{}, true))},
		{EventType: "llm.agent.failed", Version: 1, SourceKind: "internal", SchemaJSON: provider.ResultEventSchema("llm", "agent", false, nil)},
	}
}
