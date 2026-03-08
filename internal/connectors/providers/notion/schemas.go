package notion

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
	return []provider.SchemaSpec{
		{
			EventType:  "notion.create_page.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("notion", "create_page", true, provider.ObjectSchema(map[string]any{
				"page_id": provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "notion.create_page.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("notion", "create_page", false, nil),
		},
		{
			EventType:  "notion.append_block.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("notion", "append_block", true, provider.ObjectSchema(map[string]any{
				"block_id": provider.StringSchema(),
			}, false)),
		},
		{
			EventType:  "notion.append_block.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: provider.ResultEventSchema("notion", "append_block", false, nil),
		},
	}
}
