package notion

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return []integration.SchemaSpec{
		{
			EventType:  "notion.create_page.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("notion", "create_page", true, integration.ObjectSchema(map[string]any{
				"page_id": integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "notion.create_page.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("notion", "create_page", false, nil),
		},
		{
			EventType:  "notion.append_block.completed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("notion", "append_block", true, integration.ObjectSchema(map[string]any{
				"block_id": integration.StringSchema(),
			}, false)),
		},
		{
			EventType:  "notion.append_block.failed",
			Version:    1,
			SourceKind: "internal",
			SchemaJSON: integration.ResultEventSchema("notion", "append_block", false, nil),
		},
	}
}
