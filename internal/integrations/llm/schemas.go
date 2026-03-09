package llm

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return []integration.SchemaSpec{
		{
			EventType:  "llm.generate.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("llm", "generate", true, integration.ObjectSchema(map[string]any{
				"text":        integration.StringSchema(),
				"integration": integration.StringSchema(),
				"model":       integration.StringSchema(),
				"usage": integration.ObjectSchema(map[string]any{
					"prompt_tokens":     integration.IntegerSchema(),
					"completion_tokens": integration.IntegerSchema(),
					"total_tokens":      integration.IntegerSchema(),
				}, false),
			}, false)),
		},
		{EventType: "llm.generate.failed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "generate", false, nil)},
		{
			EventType:  "llm.summarize.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("llm", "summarize", true, integration.ObjectSchema(map[string]any{
				"text":        integration.StringSchema(),
				"integration": integration.StringSchema(),
				"model":       integration.StringSchema(),
				"usage": integration.ObjectSchema(map[string]any{
					"prompt_tokens":     integration.IntegerSchema(),
					"completion_tokens": integration.IntegerSchema(),
					"total_tokens":      integration.IntegerSchema(),
				}, false),
			}, false)),
		},
		{EventType: "llm.summarize.failed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "summarize", false, nil)},
		{
			EventType:  "llm.classify.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("llm", "classify", true, integration.ObjectSchema(map[string]any{
				"label": integration.StringSchema(),
			}, false)),
		},
		{EventType: "llm.classify.failed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "classify", false, nil)},
		{EventType: "llm.extract.completed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "extract", true, integration.ObjectSchema(map[string]any{}, true))},
		{EventType: "llm.extract.failed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "extract", false, nil)},
		{EventType: "llm.agent.completed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "agent", true, integration.ObjectSchema(map[string]any{}, true))},
		{EventType: "llm.agent.failed", Version: 1, SourceKind: "internal", SchemaJSON: integration.ResultEventSchema("llm", "agent", false, nil)},
	}
}
