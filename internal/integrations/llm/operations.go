package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"groot/internal/config"
	"groot/internal/connection"
	"groot/internal/connectors/outbound"
	"groot/internal/event"
	"groot/internal/integrations/llm/clients"
	anthropicclient "groot/internal/integrations/llm/clients/anthropic"
	openaiclient "groot/internal/integrations/llm/clients/openai"
)

const (
	IntegrationName    = "llm"
	OperationGenerate  = "generate"
	OperationSummarize = "summarize"
	OperationClassify  = "classify"
	OperationExtract   = "extract"
	OperationAgent     = "agent"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	cfg        config.LLMConfig
	httpClient HTTPClient
}

func New(cfg config.LLMConfig, httpClient HTTPClient) *Client {
	return &Client{cfg: cfg, httpClient: httpClient}
}

func (c *Client) Name() string {
	return IntegrationName
}

func (c *Client) Execute(ctx context.Context, operation string, instanceConfig, params json.RawMessage, _ event.Event) (outbound.Result, error) {
	llmConfig, err := c.parseConfig(instanceConfig)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: err}
	}
	parsedParams, prompt, err := c.parseOperation(operation, params)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: err}
	}
	integrationName := strings.TrimSpace(parsedParams.Integration)
	if integrationName == "" {
		integrationName = strings.TrimSpace(llmConfig.DefaultIntegration)
	}
	integrationConfig, ok := llmConfig.Integrations[integrationName]
	if !ok {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("unknown llm integration: %s", integrationName), Integration: integrationName, Model: parsedParams.Model}
	}
	apiKey, err := c.resolveAPIKey(integrationConfig.APIKey)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: err, Integration: integrationName, Model: parsedParams.Model}
	}
	integration := c.integration(integrationName, apiKey)
	if integration == nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("unknown llm integration: %s", integrationName), Integration: integrationName, Model: parsedParams.Model}
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	response, err := integration.Generate(runCtx, prompt, clients.Params{
		Model:       parsedParams.Model,
		Temperature: parsedParams.Temperature,
		MaxTokens:   parsedParams.MaxTokens,
	})
	if err != nil {
		return outbound.Result{}, err
	}
	result := outbound.Result{
		StatusCode:  response.StatusCode,
		Text:        response.Text,
		Integration: integrationName,
		Model:       response.Model,
		Usage: outbound.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}
	if output, err := buildOutput(operation, response.Text); err == nil {
		result.Output = output
	} else {
		return outbound.Result{}, outbound.PermanentError{Err: err, Integration: integrationName, Model: response.Model}
	}
	return result, nil
}

func (c *Client) parseConfig(raw json.RawMessage) (connection.LLMConfig, error) {
	var parsed connection.LLMConfig
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return connection.LLMConfig{}, fmt.Errorf("decode llm connection config: %w", err)
	}
	if strings.TrimSpace(parsed.DefaultIntegration) == "" {
		parsed.DefaultIntegration = c.cfg.DefaultIntegration
	}
	return parsed, nil
}

func (c *Client) parseOperation(operation string, params json.RawMessage) (operationParams, string, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	var parsed operationParams
	if err := json.Unmarshal(params, &parsed); err != nil {
		return operationParams{}, "", fmt.Errorf("decode llm params: %w", err)
	}
	switch strings.TrimSpace(operation) {
	case OperationGenerate:
		if strings.TrimSpace(parsed.Prompt) == "" {
			return operationParams{}, "", fmt.Errorf("prompt is required")
		}
		return parsed, parsed.Prompt, nil
	case OperationSummarize:
		if strings.TrimSpace(parsed.Text) == "" {
			return operationParams{}, "", fmt.Errorf("text is required")
		}
		return parsed, "Summarize the following text:\n\n" + parsed.Text, nil
	case OperationClassify:
		if strings.TrimSpace(parsed.Text) == "" || len(parsed.Labels) == 0 {
			return operationParams{}, "", fmt.Errorf("text and labels are required")
		}
		if strings.TrimSpace(parsed.Model) == "" {
			parsed.Model = c.cfg.DefaultClassifyModel
		}
		return parsed, fmt.Sprintf("Classify the following text into one of the labels.\n\nLabels:\n%s\n\nText:\n%s\n\nReturn only the label.", strings.Join(parsed.Labels, "\n"), parsed.Text), nil
	case OperationExtract:
		if strings.TrimSpace(parsed.Text) == "" || parsed.Schema == nil {
			return operationParams{}, "", fmt.Errorf("text and schema are required")
		}
		schema, err := json.Marshal(parsed.Schema)
		if err != nil {
			return operationParams{}, "", fmt.Errorf("marshal schema: %w", err)
		}
		if strings.TrimSpace(parsed.Model) == "" {
			parsed.Model = c.cfg.DefaultExtractModel
		}
		return parsed, fmt.Sprintf("Extract structured data matching this schema:\n\n%s\n\nText:\n%s\n\nReturn valid JSON only.", schema, parsed.Text), nil
	default:
		return operationParams{}, "", outbound.ErrUnsupportedOperation
	}
}

func buildOutput(operation string, text string) (json.RawMessage, error) {
	switch strings.TrimSpace(operation) {
	case OperationGenerate, OperationSummarize:
		return json.Marshal(map[string]any{"text": text})
	case OperationClassify:
		label := strings.TrimSpace(strings.Trim(text, "\""))
		if label == "" {
			return nil, fmt.Errorf("classify result is empty")
		}
		return json.Marshal(map[string]any{"label": label})
	case OperationExtract:
		var payload any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			return nil, fmt.Errorf("extract result is not valid json: %w", err)
		}
		return json.Marshal(payload)
	default:
		return json.Marshal(map[string]any{})
	}
}

func (c *Client) resolveAPIKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "env:") {
		envName := strings.TrimSpace(strings.TrimPrefix(value, "env:"))
		if envName == "" {
			return "", fmt.Errorf("invalid llm api key reference")
		}
		switch envName {
		case "OPENAI_API_KEY":
			if c.cfg.OpenAIAPIKey != "" {
				return c.cfg.OpenAIAPIKey, nil
			}
		case "ANTHROPIC_API_KEY":
			if c.cfg.AnthropicAPIKey != "" {
				return c.cfg.AnthropicAPIKey, nil
			}
		}
		resolved := strings.TrimSpace(os.Getenv(envName))
		if resolved == "" {
			return "", fmt.Errorf("llm api key env var is empty: %s", envName)
		}
		return resolved, nil
	}
	if value == "" {
		return "", fmt.Errorf("llm api key is required")
	}
	return value, nil
}

func (c *Client) integration(name, apiKey string) clients.Integration {
	switch name {
	case "openai":
		return openaiclient.New(c.cfg.OpenAIAPIBaseURL, apiKey, c.httpClient)
	case "anthropic":
		return anthropicclient.New(c.cfg.AnthropicAPIBaseURL, apiKey, c.httpClient)
	default:
		return nil
	}
}
