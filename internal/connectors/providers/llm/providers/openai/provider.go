package openai

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
	return "openai"
}

func (p *Provider) Generate(ctx context.Context, prompt string, params providers.Params) (providers.Response, error) {
	body, err := json.Marshal(map[string]any{
		"model": coalesce(params.Model, "gpt-4o-mini"),
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": optionalFloat(params.Temperature),
		"max_tokens":  optionalInt(params.MaxTokens),
	})
	if err != nil {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("marshal openai request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("build openai request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("perform openai request: %w", err), Provider: p.Name(), Model: params.Model}
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("read openai response: %w", err), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("openai returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("openai returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providers.Response{}, outbound.PermanentError{Err: fmt.Errorf("openai returned status %d", resp.StatusCode), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return providers.Response{}, outbound.RetryableError{Err: fmt.Errorf("decode openai response: %w", err), StatusCode: resp.StatusCode, Provider: p.Name(), Model: params.Model}
	}
	text := ""
	if len(payload.Choices) > 0 {
		text = payload.Choices[0].Message.Content
	}
	return providers.Response{
		Text:       text,
		StatusCode: resp.StatusCode,
		Model:      coalesce(params.Model, "gpt-4o-mini"),
		Usage: providers.Usage{
			PromptTokens:     payload.Usage.PromptTokens,
			CompletionTokens: payload.Usage.CompletionTokens,
			TotalTokens:      payload.Usage.TotalTokens,
		},
	}, nil
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func coalesce(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
