package providers

import "context"

type Params struct {
	Model       string
	Temperature *float64
	MaxTokens   *int
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Response struct {
	Text       string
	Usage      Usage
	StatusCode int
	Model      string
}

type Provider interface {
	Name() string
	Generate(context.Context, string, Params) (Response, error)
}
