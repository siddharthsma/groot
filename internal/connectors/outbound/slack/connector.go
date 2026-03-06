package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/outbound"
	"groot/internal/stream"
)

const (
	ConnectorName              = "slack"
	OperationPostMessage       = "post_message"
	OperationCreateThreadReply = "create_thread_reply"
	defaultSlackAPIBaseURL     = "https://slack.com/api"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Connector struct {
	apiBaseURL string
	httpClient HTTPClient
}

type Config struct {
	BotToken       string `json:"bot_token"`
	DefaultChannel string `json:"default_channel,omitempty"`
}

type Params struct {
	Channel  string          `json:"channel,omitempty"`
	Text     string          `json:"text,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	ThreadTS string          `json:"thread_ts,omitempty"`
}

type postMessageRequest struct {
	Channel  string          `json:"channel"`
	Text     string          `json:"text,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	ThreadTS string          `json:"thread_ts,omitempty"`
}

type postMessageResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	TS    string `json:"ts,omitempty"`
}

func New(apiBaseURL string, httpClient HTTPClient) *Connector {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = defaultSlackAPIBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Connector{apiBaseURL: strings.TrimRight(apiBaseURL, "/"), httpClient: httpClient}
}

func (c *Connector) Name() string {
	return ConnectorName
}

func (c *Connector) Execute(ctx context.Context, operation string, instanceConfig, params json.RawMessage, _ stream.Event) (outbound.Result, error) {
	op := strings.TrimSpace(operation)
	if op != OperationPostMessage && op != OperationCreateThreadReply {
		return outbound.Result{}, outbound.PermanentError{Err: outbound.ErrUnsupportedOperation}
	}

	cfg, reqBody, err := buildRequest(op, instanceConfig, params)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: err}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("marshal slack request: %w", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("build slack request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+cfg.BotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("perform slack request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("read slack response: %w", err), StatusCode: resp.StatusCode}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("slack returned status %d", resp.StatusCode)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return outbound.Result{}, outbound.PermanentError{Err: err, StatusCode: resp.StatusCode}
		}
		return outbound.Result{}, outbound.RetryableError{Err: err, StatusCode: resp.StatusCode}
	}

	var payload postMessageResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("decode slack response: %w", err), StatusCode: resp.StatusCode}
	}
	if !payload.OK {
		err := fmt.Errorf("slack error: %s", payload.Error)
		switch payload.Error {
		case "ratelimited":
			return outbound.Result{}, outbound.RetryableError{Err: err, StatusCode: resp.StatusCode}
		case "invalid_auth", "account_inactive", "token_revoked":
			return outbound.Result{}, outbound.PermanentError{Err: err, StatusCode: resp.StatusCode}
		default:
			return outbound.Result{}, outbound.PermanentError{Err: err, StatusCode: resp.StatusCode}
		}
	}
	output, err := json.Marshal(map[string]any{"channel": reqBody.Channel, "ts": payload.TS})
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("marshal slack output: %w", err), StatusCode: resp.StatusCode}
	}
	return outbound.Result{ExternalID: payload.TS, StatusCode: resp.StatusCode, Channel: reqBody.Channel, Output: output}, nil
}

func buildRequest(operation string, instanceConfig, params json.RawMessage) (Config, postMessageRequest, error) {
	var cfg connectorinstance.SlackConfig
	if err := json.Unmarshal(instanceConfig, &cfg); err != nil {
		return Config{}, postMessageRequest{}, fmt.Errorf("decode connector config: %w", err)
	}
	if strings.TrimSpace(cfg.BotToken) == "" {
		return Config{}, postMessageRequest{}, connectorinstance.ErrMissingBotToken
	}

	var reqParams Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		return Config{}, postMessageRequest{}, fmt.Errorf("decode operation params: %w", err)
	}

	channel := strings.TrimSpace(reqParams.Channel)
	if channel == "" {
		channel = strings.TrimSpace(cfg.DefaultChannel)
	}
	if channel == "" {
		return Config{}, postMessageRequest{}, errors.New("channel is required")
	}
	if operation == OperationCreateThreadReply && strings.TrimSpace(reqParams.ThreadTS) == "" {
		return Config{}, postMessageRequest{}, errors.New("thread_ts is required")
	}
	if strings.TrimSpace(reqParams.Text) == "" && len(reqParams.Blocks) == 0 {
		return Config{}, postMessageRequest{}, errors.New("text is required unless blocks are provided")
	}

	return Config{BotToken: cfg.BotToken, DefaultChannel: cfg.DefaultChannel}, postMessageRequest{
		Channel:  channel,
		Text:     reqParams.Text,
		Blocks:   reqParams.Blocks,
		ThreadTS: strings.TrimSpace(reqParams.ThreadTS),
	}, nil
}
