package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Enabled         bool
	BaseURL         string
	Timeout         time.Duration
	SharedSecret    string
	ToolEndpointURL string
}

type Client struct {
	baseURL         string
	sharedSecret    string
	toolEndpointURL string
	httpClient      *http.Client
}

type Event struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	SourceKind string          `json:"source_kind"`
	ChainDepth int             `json:"chain_depth"`
	Payload    json.RawMessage `json:"payload"`
}

type Request struct {
	TenantID       string          `json:"tenant_id"`
	AgentID        string          `json:"agent_id"`
	AgentName      string          `json:"agent_name"`
	AgentRunID     string          `json:"agent_run_id"`
	SessionID      string          `json:"session_id"`
	SessionKey     string          `json:"session_key"`
	Instructions   string          `json:"instructions"`
	Provider       string          `json:"provider,omitempty"`
	Model          string          `json:"model,omitempty"`
	AllowedTools   []string        `json:"allowed_tools"`
	ToolBindings   json.RawMessage `json:"tool_bindings"`
	Event          Event           `json:"event"`
	SessionSummary *string         `json:"session_summary,omitempty"`
	ToolEndpoint   string          `json:"tool_endpoint_url"`
	ToolAuthToken  string          `json:"tool_auth_token"`
}

type ToolCallSummary struct {
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	ExternalID string `json:"external_id,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	Status         string            `json:"status"`
	Output         json.RawMessage   `json:"output"`
	SessionSummary *string           `json:"session_summary,omitempty"`
	ToolCalls      []ToolCallSummary `json:"tool_calls"`
	Usage          Usage             `json:"usage"`
	Error          *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

func NewClient(cfg Config, client *http.Client) *Client {
	httpClient := client
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:         strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		sharedSecret:    strings.TrimSpace(cfg.SharedSecret),
		toolEndpointURL: strings.TrimSpace(cfg.ToolEndpointURL),
		httpClient:      httpClient,
	}
}

func (c *Client) RunAgentSession(ctx context.Context, req Request) (Response, error) {
	req.ToolEndpoint = c.toolEndpointURL
	req.ToolAuthToken = c.sharedSecret
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, PermanentError{Err: fmt.Errorf("marshal runtime request: %w", err)}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sessions/run", bytes.NewReader(body))
	if err != nil {
		return Response{}, PermanentError{Err: fmt.Errorf("build runtime request: %w", err)}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("call agent runtime: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("read agent runtime response: %w", err)
	}

	if resp.StatusCode >= 500 {
		return Response{}, fmt.Errorf("agent runtime status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, PermanentError{Err: fmt.Errorf("agent runtime rejected request with status %d", resp.StatusCode)}
	}

	var parsed Response
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, PermanentError{Err: fmt.Errorf("decode runtime response: %w", err)}
	}
	switch strings.TrimSpace(parsed.Status) {
	case "succeeded":
		return parsed, nil
	case "failed":
		message := "agent runtime failed"
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			message = strings.TrimSpace(parsed.Error.Message)
		}
		return Response{}, PermanentError{Err: fmt.Errorf(message)}
	default:
		return Response{}, PermanentError{Err: fmt.Errorf("agent runtime returned invalid status")}
	}
}
