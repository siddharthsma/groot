package notion

type createPageParams struct {
	ParentDatabaseID string         `json:"parent_database_id"`
	Properties       map[string]any `json:"properties"`
}

type appendBlockParams struct {
	BlockID  string `json:"block_id"`
	Children []any  `json:"children"`
}
