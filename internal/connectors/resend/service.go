package resend

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	svix "github.com/svix/svix-webhooks/go"

	"groot/internal/config"
	"groot/internal/connectors"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/stream"
	"groot/internal/tenant"
)

const (
	ConnectorName              = "resend"
	EventTypeEmailReceived     = "resend.email.received"
	EventSourceResend          = "resend"
	systemSettingWebhookID     = "resend_webhook_id"
	systemSettingSigningSecret = "resend_webhook_signing_secret"
)

var _ connectors.Inbound = (*Service)(nil)

type Store interface {
	EnsureConnectorInstance(context.Context, tenant.ID, string, time.Time) error
	GetInboundRouteByTenant(context.Context, string, tenant.ID) (inboundroute.Route, error)
	CreateInboundRoute(context.Context, inboundroute.Record) (inboundroute.Route, error)
	GetInboundRoute(context.Context, string, string) (inboundroute.Route, error)
	GetSystemSetting(context.Context, string) (string, error)
	UpsertSystemSetting(context.Context, string, string) error
}

type EventIngestor interface {
	Ingest(context.Context, ingest.Request) (stream.Event, error)
}

type Metrics interface {
	IncResendWebhooksReceived()
	IncResendWebhooksVerified()
	IncResendWebhooksVerificationFailed()
	IncResendUnroutable()
	IncResendEventsPublished()
	IncInboundRoutes(string)
	IncInboundUnroutable(string)
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Service struct {
	cfg     config.ResendConfig
	store   Store
	ingest  EventIngestor
	logger  *slog.Logger
	metrics Metrics
	client  HTTPClient
	now     func() time.Time
}

type EnableResult struct {
	Address string `json:"address"`
}

type bootstrapRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"enabled_events"`
}

type bootstrapResponse struct {
	ID            string `json:"id"`
	SigningSecret string `json:"signing_secret"`
}

func NewService(cfg config.ResendConfig, store Store, ingestor EventIngestor, logger *slog.Logger, metrics Metrics, client HTTPClient) *Service {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Service{
		cfg:     cfg,
		store:   store,
		ingest:  ingestor,
		logger:  logger,
		metrics: metrics,
		client:  client,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Name() string {
	return ConnectorName
}

func (s *Service) Bootstrap(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.cfg.APIKey) == "" || strings.TrimSpace(s.cfg.WebhookPublicURL) == "" || strings.TrimSpace(s.cfg.ReceivingDomain) == "" {
		return "", errors.New("resend configuration is incomplete")
	}

	webhookID, err := s.store.GetSystemSetting(ctx, systemSettingWebhookID)
	if err == nil && strings.TrimSpace(webhookID) != "" {
		signingSecret, secretErr := s.store.GetSystemSetting(ctx, systemSettingSigningSecret)
		if secretErr == nil && strings.TrimSpace(signingSecret) != "" {
			return "already_bootstrapped", nil
		}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("get webhook id: %w", err)
	}

	body, err := json.Marshal(bootstrapRequest{
		URL:    s.cfg.WebhookPublicURL,
		Events: s.cfg.WebhookEvents,
	})
	if err != nil {
		return "", fmt.Errorf("marshal bootstrap request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.APIBaseURL, "/")+"/webhooks", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build bootstrap request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send bootstrap request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("bootstrap request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var created bootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode bootstrap response: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" || strings.TrimSpace(created.SigningSecret) == "" {
		return "", errors.New("bootstrap response missing id or signing_secret")
	}

	if err := s.store.UpsertSystemSetting(ctx, systemSettingWebhookID, created.ID); err != nil {
		return "", fmt.Errorf("store webhook id: %w", err)
	}
	if err := s.store.UpsertSystemSetting(ctx, systemSettingSigningSecret, created.SigningSecret); err != nil {
		return "", fmt.Errorf("store signing secret: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("resend_bootstrap_completed", slog.String("webhook_id", created.ID))
	}

	return "bootstrapped", nil
}

func (s *Service) Enable(ctx context.Context, tenantID tenant.ID) (EnableResult, error) {
	if err := s.store.EnsureConnectorInstance(ctx, tenantID, ConnectorName, s.now()); err != nil {
		return EnableResult{}, fmt.Errorf("ensure connector instance: %w", err)
	}

	route, err := s.store.GetInboundRouteByTenant(ctx, ConnectorName, tenantID)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		token := uuid.NewString()
		route, err = s.store.CreateInboundRoute(ctx, inboundroute.Record{
			ID:            uuid.New(),
			ConnectorName: ConnectorName,
			RouteKey:      token,
			TenantID:      tenantID,
			CreatedAt:     s.now(),
		})
		if err != nil {
			return EnableResult{}, fmt.Errorf("create inbound route: %w", err)
		}
		if s.metrics != nil {
			s.metrics.IncInboundRoutes(ConnectorName)
		}
		if s.logger != nil {
			s.logger.Info("inbound_route_created", slog.String("connector_name", ConnectorName), slog.String("route_key", route.RouteKey), slog.String("tenant_id", tenantID.String()))
		}
	default:
		return EnableResult{}, fmt.Errorf("get inbound route: %w", err)
	}

	address := fmt.Sprintf("inbound+%s@%s", route.RouteKey, s.cfg.ReceivingDomain)
	if s.logger != nil {
		s.logger.Info("resend_connector_enabled", slog.String("tenant_id", tenantID.String()))
	}
	return EnableResult{Address: address}, nil
}

func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, headers http.Header) error {
	if s.metrics != nil {
		s.metrics.IncResendWebhooksReceived()
	}

	signingSecret, err := s.store.GetSystemSetting(ctx, systemSettingSigningSecret)
	if err != nil {
		if s.logger != nil {
			s.logger.Info("resend_webhook_verification_failed", slog.String("error", err.Error()))
		}
		if s.metrics != nil {
			s.metrics.IncResendWebhooksVerificationFailed()
		}
		return nil
	}

	webhook, err := svix.NewWebhook(signingSecret)
	if err != nil {
		return fmt.Errorf("create svix verifier: %w", err)
	}
	if err := webhook.Verify(rawBody, headers); err != nil {
		if s.logger != nil {
			s.logger.Info("resend_webhook_verification_failed", slog.String("error", err.Error()))
		}
		if s.metrics != nil {
			s.metrics.IncResendWebhooksVerificationFailed()
		}
		return nil
	}
	if s.metrics != nil {
		s.metrics.IncResendWebhooksVerified()
	}
	if s.logger != nil {
		s.logger.Info("resend_webhook_verified")
	}

	token, err := extractRouteToken(rawBody)
	if err != nil {
		if s.logger != nil {
			s.logger.Info("resend_unroutable", slog.String("error", err.Error()))
			s.logger.Info("inbound_route_missing", slog.String("connector_name", ConnectorName))
		}
		if s.metrics != nil {
			s.metrics.IncResendUnroutable()
			s.metrics.IncInboundUnroutable(ConnectorName)
		}
		return nil
	}

	route, err := s.store.GetInboundRoute(ctx, ConnectorName, token)
	if err != nil {
		if s.logger != nil {
			s.logger.Info("resend_unroutable", slog.String("token", token))
			s.logger.Info("inbound_route_missing", slog.String("connector_name", ConnectorName), slog.String("route_key", token))
		}
		if s.metrics != nil {
			s.metrics.IncResendUnroutable()
			s.metrics.IncInboundUnroutable(ConnectorName)
		}
		return nil
	}
	if s.logger != nil {
		s.logger.Info("inbound_route_resolved", slog.String("connector_name", ConnectorName), slog.String("route_key", token), slog.String("tenant_id", route.TenantID.String()))
	}

	event, err := s.ingest.Ingest(ctx, ingest.Request{
		TenantID: tenant.ID(route.TenantID),
		Type:     EventTypeEmailReceived,
		Source:   EventSourceResend,
		Payload:  json.RawMessage(rawBody),
	})
	if err != nil {
		return fmt.Errorf("ingest resend webhook: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncResendEventsPublished()
	}
	if s.logger != nil {
		resolvedTenantID := tenant.ID(route.TenantID)
		s.logger.Info("resend_event_published",
			slog.String("tenant_id", resolvedTenantID.String()),
			slog.String("event_id", event.EventID.String()),
		)
	}
	return nil
}

func extractRouteToken(rawBody []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return "", fmt.Errorf("decode webhook payload: %w", err)
	}

	recipients := extractRecipientList(payload)
	for _, recipient := range recipients {
		address, err := mail.ParseAddress(recipient)
		if err == nil {
			recipient = address.Address
		}
		localPart, _, ok := strings.Cut(strings.ToLower(strings.TrimSpace(recipient)), "@")
		if !ok {
			continue
		}
		if strings.HasPrefix(localPart, "inbound+") {
			return strings.TrimPrefix(localPart, "inbound+"), nil
		}
	}
	return "", errors.New("no routable inbound recipient found")
}

func extractRecipientList(payload map[string]any) []string {
	var recipients []string
	collectRecipients(&recipients, payload["to"])
	if len(recipients) > 0 {
		return recipients
	}
	if data, ok := payload["data"].(map[string]any); ok {
		collectRecipients(&recipients, data["to"])
	}
	return recipients
}

func collectRecipients(out *[]string, value any) {
	switch typed := value.(type) {
	case string:
		*out = append(*out, typed)
	case []any:
		for _, item := range typed {
			switch v := item.(type) {
			case string:
				*out = append(*out, v)
			case map[string]any:
				if address, ok := v["email"].(string); ok {
					*out = append(*out, address)
				}
				if address, ok := v["address"].(string); ok {
					*out = append(*out, address)
				}
			}
		}
	}
}
