package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/config"
	"groot/internal/connectorinstance"
	"groot/internal/connectors"
	eventpkg "groot/internal/event"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/schema"
	"groot/internal/tenant"
)

const (
	ConnectorName = "stripe"
	EventSource   = "stripe"
)

var (
	_                   connectors.Inbound = (*Service)(nil)
	ErrUnauthorized                        = errors.New("stripe webhook signature verification failed")
	ErrRouteConflict                       = errors.New("stripe account is already connected to another tenant")
	ErrInvalidAccountID                    = errors.New("stripe_account_id is required")
	ErrInvalidSecret                       = errors.New("webhook_secret is required")
)

type Store interface {
	GetTenantConnectorInstanceByName(context.Context, tenant.ID, string) (connectorinstance.Instance, error)
	CreateConnectorInstance(context.Context, connectorinstance.Record) (connectorinstance.Instance, error)
	UpdateConnectorInstanceConfig(context.Context, tenant.ID, string, json.RawMessage) (connectorinstance.Instance, error)
	GetInboundRouteByTenant(context.Context, string, tenant.ID) (inboundroute.Route, error)
	CreateInboundRoute(context.Context, inboundroute.Record) (inboundroute.Route, error)
	UpdateInboundRouteByTenant(context.Context, string, tenant.ID, string, *uuid.UUID) (inboundroute.Route, error)
	GetInboundRoute(context.Context, string, string) (inboundroute.Route, error)
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error)
}

type EventIngestor interface {
	Ingest(context.Context, ingest.Request) (eventpkg.Event, error)
}

type Metrics interface {
	IncStripeWebhooks()
	IncStripeUnroutable()
	IncInboundRoutes(string)
	IncInboundUnroutable(string)
}

type Service struct {
	cfg     config.StripeConfig
	store   Store
	ingest  EventIngestor
	logger  *slog.Logger
	metrics Metrics
	now     func() time.Time
}

func NewService(cfg config.StripeConfig, store Store, ingestor EventIngestor, logger *slog.Logger, metrics Metrics) *Service {
	return &Service{
		cfg:     cfg,
		store:   store,
		ingest:  ingestor,
		logger:  logger,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Name() string {
	return ConnectorName
}

func (s *Service) Enable(ctx context.Context, tenantID tenant.ID, stripeAccountID string, webhookSecret string) (uuid.UUID, error) {
	accountID := strings.TrimSpace(stripeAccountID)
	if accountID == "" {
		return uuid.Nil, ErrInvalidAccountID
	}
	secret := strings.TrimSpace(webhookSecret)
	if secret == "" {
		return uuid.Nil, ErrInvalidSecret
	}
	configJSON, err := json.Marshal(connectorinstance.StripeConfig{
		StripeAccountID: accountID,
		WebhookSecret:   secret,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal stripe connector config: %w", err)
	}

	instance, err := s.store.GetTenantConnectorInstanceByName(ctx, tenantID, ConnectorName)
	switch {
	case err == nil:
		instance, err = s.store.UpdateConnectorInstanceConfig(ctx, tenantID, ConnectorName, configJSON)
		if err != nil {
			return uuid.Nil, fmt.Errorf("update stripe connector instance: %w", err)
		}
	case errors.Is(err, connectorinstance.ErrNotFound):
		instance, err = s.store.CreateConnectorInstance(ctx, connectorinstance.Record{
			ID:            uuid.New(),
			TenantID:      tenantID,
			OwnerTenantID: &tenantID,
			ConnectorName: ConnectorName,
			Scope:         connectorinstance.ScopeTenant,
			Status:        "enabled",
			Config:        configJSON,
			CreatedAt:     s.now(),
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("create stripe connector instance: %w", err)
		}
	default:
		return uuid.Nil, fmt.Errorf("get stripe connector instance: %w", err)
	}

	_, routeErr := s.store.GetInboundRouteByTenant(ctx, ConnectorName, tenantID)
	switch {
	case routeErr == nil:
		if _, err := s.store.UpdateInboundRouteByTenant(ctx, ConnectorName, tenantID, accountID, &instance.ID); err != nil {
			if errors.Is(err, inboundroute.ErrDuplicateRoute) {
				return uuid.Nil, ErrRouteConflict
			}
			return uuid.Nil, fmt.Errorf("update stripe inbound route: %w", err)
		}
	case errors.Is(routeErr, sql.ErrNoRows):
		if _, err := s.store.CreateInboundRoute(ctx, inboundroute.Record{
			ID:                  uuid.New(),
			ConnectorName:       ConnectorName,
			RouteKey:            accountID,
			TenantID:            tenantID,
			ConnectorInstanceID: &instance.ID,
			CreatedAt:           s.now(),
		}); err != nil {
			if errors.Is(err, inboundroute.ErrDuplicateRoute) {
				return uuid.Nil, ErrRouteConflict
			}
			return uuid.Nil, fmt.Errorf("create stripe inbound route: %w", err)
		}
		if s.metrics != nil {
			s.metrics.IncInboundRoutes(ConnectorName)
		}
	default:
		return uuid.Nil, fmt.Errorf("get stripe inbound route: %w", routeErr)
	}

	return instance.ID, nil
}

func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, headers http.Header) error {
	if s.metrics != nil {
		s.metrics.IncStripeWebhooks()
	}
	if s.logger != nil {
		s.logger.Info("stripe_webhook_received", slog.String("connector_name", ConnectorName))
	}

	payload, err := parseWebhook(rawBody)
	if err != nil {
		return fmt.Errorf("parse stripe webhook: %w", err)
	}

	route, err := s.store.GetInboundRoute(ctx, ConnectorName, payload.Account)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get stripe inbound route: %w", err)
		}
		if s.logger != nil {
			s.logger.Info("stripe_unroutable", slog.String("connector_name", ConnectorName))
		}
		if s.metrics != nil {
			s.metrics.IncStripeUnroutable()
			s.metrics.IncInboundUnroutable(ConnectorName)
		}
		return nil
	}

	secret, err := s.loadWebhookSecret(ctx, tenant.ID(route.TenantID), route.ConnectorInstanceID)
	if err != nil {
		return fmt.Errorf("load stripe webhook secret: %w", err)
	}
	if err := verifySignature(headers.Get("Stripe-Signature"), rawBody, secret, s.cfg.WebhookToleranceSeconds, s.now()); err != nil {
		return ErrUnauthorized
	}

	event, err := s.ingest.Ingest(ctx, ingest.Request{
		TenantID: tenant.ID(route.TenantID),
		Type:     schema.FullName("stripe."+payload.Type, 1),
		Source:   EventSource,
		Payload:  json.RawMessage(rawBody),
	})
	if err != nil {
		return fmt.Errorf("ingest stripe webhook: %w", err)
	}
	if s.logger != nil {
		s.logger.Info("stripe_event_published",
			slog.String("tenant_id", route.TenantID.String()),
			slog.String("connector_name", ConnectorName),
			slog.String("event_id", event.EventID.String()),
		)
	}
	return nil
}

func parseWebhook(rawBody []byte) (webhookPayload, error) {
	var payload webhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return webhookPayload{}, err
	}
	payload.Type = strings.TrimSpace(payload.Type)
	payload.Account = strings.TrimSpace(payload.Account)
	if payload.Type == "" || payload.Account == "" {
		return webhookPayload{}, errors.New("stripe webhook payload missing type or account")
	}
	return payload, nil
}

func (s *Service) loadWebhookSecret(ctx context.Context, tenantID tenant.ID, connectorInstanceID *uuid.UUID) (string, error) {
	if connectorInstanceID == nil {
		return "", errors.New("stripe route missing connector instance")
	}
	instance, err := s.store.GetConnectorInstance(ctx, tenantID, *connectorInstanceID)
	if err != nil {
		return "", err
	}
	var cfg connectorinstance.StripeConfig
	if err := json.Unmarshal(instance.Config, &cfg); err != nil {
		return "", err
	}
	return strings.TrimSpace(cfg.WebhookSecret), nil
}

func verifySignature(signatureHeader string, rawBody []byte, secret string, toleranceSeconds int, now time.Time) error {
	parts := strings.Split(signatureHeader, ",")
	var timestamp string
	var signatures []string
	for _, part := range parts {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return errors.New("missing stripe signature components")
	}
	parsedTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return err
	}
	if delta := now.Unix() - parsedTimestamp; delta > int64(toleranceSeconds) || delta < -int64(toleranceSeconds) {
		return errors.New("stripe signature timestamp outside tolerance")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	for _, candidate := range signatures {
		if hmac.Equal([]byte(expected), []byte(candidate)) {
			return nil
		}
	}
	return errors.New("stripe signature mismatch")
}
