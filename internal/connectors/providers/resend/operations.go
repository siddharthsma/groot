package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"groot/internal/config"
	"groot/internal/connectors/outbound"
	"groot/internal/event"
)

const (
	ConnectorName           = "resend"
	OperationSendEmail      = "send_email"
	defaultResendAPIBaseURL = "https://api.resend.com"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Connector struct {
	apiBaseURL string
	apiKey     string
	httpClient HTTPClient
}

func New(cfg config.ResendConfig, httpClient HTTPClient) *Connector {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	apiBaseURL := strings.TrimSpace(cfg.APIBaseURL)
	if apiBaseURL == "" {
		apiBaseURL = defaultResendAPIBaseURL
	}
	return &Connector{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: httpClient,
	}
}

func (c *Connector) Name() string {
	return ConnectorName
}

func (c *Connector) Execute(ctx context.Context, operation string, _ json.RawMessage, params json.RawMessage, _ event.Event) (outbound.Result, error) {
	if strings.TrimSpace(operation) != OperationSendEmail {
		return outbound.Result{}, outbound.PermanentError{Err: outbound.ErrUnsupportedOperation}
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("resend api key is required")}
	}

	var reqBody sendEmailParams
	if err := json.Unmarshal(params, &reqBody); err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("decode resend params: %w", err)}
	}
	reqBody.To = strings.TrimSpace(reqBody.To)
	reqBody.Subject = strings.TrimSpace(reqBody.Subject)
	if reqBody.To == "" || reqBody.Subject == "" {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("to and subject are required")}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("marshal resend request: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("build resend request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("perform resend request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("read resend response: %w", err), StatusCode: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("resend returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("resend returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("resend returned status %d", resp.StatusCode), StatusCode: resp.StatusCode}
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return outbound.Result{}, outbound.RetryableError{Err: fmt.Errorf("decode resend response: %w", err), StatusCode: resp.StatusCode}
	}
	output, err := json.Marshal(map[string]any{"email_id": strings.TrimSpace(payload.ID)})
	if err != nil {
		return outbound.Result{}, outbound.PermanentError{Err: fmt.Errorf("marshal resend output: %w", err)}
	}
	return outbound.Result{
		ExternalID: strings.TrimSpace(payload.ID),
		StatusCode: resp.StatusCode,
		Output:     output,
	}, nil
}
