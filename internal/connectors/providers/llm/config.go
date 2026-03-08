package llm

type operationParams struct {
	Prompt      string   `json:"prompt"`
	Text        string   `json:"text"`
	Labels      []string `json:"labels"`
	Schema      any      `json:"schema"`
	Model       string   `json:"model"`
	Provider    string   `json:"provider"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
}
