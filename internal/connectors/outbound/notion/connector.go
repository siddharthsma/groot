package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"groot/internal/connectorinstance"
	"groot/internal/connectors/outbound"
	"groot/internal/stream"
)

const (
	ConnectorName           = "notion"
	OperationCreatePage     = "create_page"
	OperationAppendBlock    = "append_block"
	defaultNotionAPIBaseURL = "https://api.notion.com/v1"
	defaultNotionAPIVersion = "2022-06-28"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Connector struct {
	apiBaseURL string
	apiVersion string
	httpClient HTTPClient
}

type createPageParams struct {
	ParentDatabaseID string         `json:"parent_database_id"`
	Properties       map[string]any `json:"properties"`
}

type appendBlockParams struct {
	BlockID  string `json:"block_id"`
	Children []any  `json:"children"`
}

func New(apiBaseURL, apiVersion string, httpClient HTTPClient) *Connector {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = defaultNotionAPIBaseURL
	}
	if strings.TrimSpace(apiVersion) == "" {
		apiVersion = defaultNotionAPIVersion
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Connector{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		apiVersion: apiVersion,
		httpClient: httpClient,
	}
}

func (c *Connector) Name() string {
	return ConnectorName
}

func (c *Connector) Execute(ctx context.Context, operation string, instanceConfig, params json.RawMessage, _ stream.Event) (outbound.Result, error) {
	var cfg connectorinstance.NotionConfig
	if err := json.Unmarshal(instanceConfig, &cfg); err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("decode connector config: %w", err)}
	}
	if strings.TrimSpace(cfg.IntegrationToken) == "" {
		return outbound.Result{}, outbound.PermanentError{Err: connectorinstance.ErrMissingNotionToken}
	}

	switch strings.TrimSpace(operation) {
	case OperationCreatePage:
		body, err := buildCreatePageRequest(params)
		if err != nil {
			return outbound.Result{}, outbound.PermanentError{Err: err}
		}
		return c.execute(ctx, http.MethodPost, c.apiBaseURL+"/pages", cfg.IntegrationToken, body, extractTopLevelID)
	case OperationAppendBlock:
		blockID, body, err := buildAppendBlockRequest(params)
		if err != nil {
			return outbound.Result{}, outbound.PermanentError{Err: err}
		}
		return c.execute(ctx, http.MethodPatch, c.apiBaseURL+"/blocks/"+blockID+"/children", cfg.IntegrationToken, body, extractFirstResultID)
	default:
		return outbound.Result{}, outbound.PermanentError{Err: outbound.ErrUnsupportedOperation}
	}
}

func buildCreatePageRequest(params json.RawMessage) ([]byte, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	var parsed createPageParams
	if err := json.Unmarshal(params, &parsed); err != nil {
		return nil, fmt.Errorf("decode operation params: %w", err)
	}
	if strings.TrimSpace(parsed.ParentDatabaseID) == "" {
		return nil, fmt.Errorf("parent_database_id is required")
	}
	if len(parsed.Properties) == 0 {
		return nil, fmt.Errorf("properties is required")
	}
	body, err := json.Marshal(map[string]any{
		"parent": map[string]any{
			"database_id": parsed.ParentDatabaseID,
		},
		"properties": parsed.Properties,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal notion request: %w", err)
	}
	return body, nil
}

func buildAppendBlockRequest(params json.RawMessage) (string, []byte, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	var parsed appendBlockParams
	if err := json.Unmarshal(params, &parsed); err != nil {
		return "", nil, fmt.Errorf("decode operation params: %w", err)
	}
	blockID := strings.TrimSpace(parsed.BlockID)
	if blockID == "" {
		return "", nil, fmt.Errorf("block_id is required")
	}
	if len(parsed.Children) == 0 {
		return "", nil, fmt.Errorf("children is required")
	}
	body, err := json.Marshal(map[string]any{"children": parsed.Children})
	if err != nil {
		return "", nil, fmt.Errorf("marshal notion request: %w", err)
	}
	return blockID, body, nil
}

func (c *Connector) execute(ctx context.Context, method, url, token string, body []byte, idExtractor func([]byte) (string, error)) (outbound.Result, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("build notion request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", c.apiVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("perform notion request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("read notion response: %w", err), StatusCode: resp.StatusCode}
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("notion returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("notion returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("notion returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}

	externalID, err := idExtractor(responseBody)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: err, StatusCode: resp.StatusCode}
	}
	return outbound.Result{ExternalID: externalID, StatusCode: resp.StatusCode}, nil
}

func extractTopLevelID(body []byte) (string, error) {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode notion response: %w", err)
	}
	return strings.TrimSpace(payload.ID), nil
}

func extractFirstResultID(body []byte) (string, error) {
	var payload struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode notion response: %w", err)
	}
	if len(payload.Results) == 0 {
		return "", nil
	}
	return strings.TrimSpace(payload.Results[0].ID), nil
}
