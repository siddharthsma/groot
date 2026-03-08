package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"groot/internal/connectors/outbound"
	"groot/internal/connectors/providers/llm/providers"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Provider struct {
	apiBaseURL string
	apiKey     string
	httpClient HTTPClient
}

func New(apiBaseURL, apiKey string, httpClient HTTPClient) *Provider {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Provider{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) Generate(ctx context.Context, prompt string, params providers.Params) (providers.Response, error) {
	body, err := json.Marshal(map[string]any{
		"model":      coalesce(params.Model, "claude-3-5-haiku-latest"),
		"max_tokens": optionalInt(params.MaxTokens, 256),
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": optionalFloat(params.Temperature),
	})
	if err != nil {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("marshal anthropic request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("build anthropic request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("perform anthropic request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("read anthropic response: %w", err), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("anthropic returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("anthropic returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("anthropic returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}

	var payload struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("decode anthropic response: %w", err), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	text := ""
	if len(payload.Content) > 0 {
		text = payload.Content[0].Text
	}
	model := payload.Model
	if strings.TrimSpace(model) == "" {
		model = coalesce(params.Model, "claude-3-5-haiku-latest")
	}
	return providers.Response{
		Text:       text,
		StatusCode: resp.StatusCode,
		Model:      model,
		Usage: providers.Usage{
			PromptTokens:     payload.Usage.InputTokens,
			CompletionTokens: payload.Usage.OutputTokens,
			TotalTokens:      payload.Usage.InputTokens + payload.Usage.OutputTokens,
		},
	}, nil
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
