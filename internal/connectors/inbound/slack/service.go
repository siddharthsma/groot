package slack

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

	"groot/internal/config"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
	"groot/internal/schemas"
	"groot/internal/stream"
	"groot/internal/tenant"
)

const (
	ConnectorName = "slack"
	EventSource   = "slack"
)

type Store interface {
	GetInboundRoute(context.Context, string, string) (inboundroute.Route, error)
}

type EventIngestor interface {
	Ingest(context.Context, ingest.Request) (stream.Event, error)
}

type Metrics interface {
	IncSlackEventsReceived()
	IncInboundUnroutable(string)
}

type Service struct {
	cfg     config.SlackConfig
	store   Store
	ingest  EventIngestor
	logger  *slog.Logger
	metrics Metrics
	now     func() time.Time
}

type Result struct {
	IsChallenge bool
	Challenge   string
}

func NewService(cfg config.SlackConfig, store Store, ingestor EventIngestor, logger *slog.Logger, metrics Metrics) *Service {
	return &Service{
		cfg:     cfg,
		store:   store,
		ingest:  ingestor,
		logger:  logger,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) HandleEvents(ctx context.Context, rawBody []byte, headers http.Header) (Result, error) {
	if s.metrics != nil {
		s.metrics.IncSlackEventsReceived()
	}
	if err := s.verify(headers, rawBody); err != nil {
		return Result{}, err
	}

	payload, err := parseWebhook(rawBody)
	if err != nil {
		return Result{}, fmt.Errorf("parse slack webhook: %w", err)
	}
	if strings.TrimSpace(payload.Type) == "url_verification" {
		return Result{IsChallenge: true, Challenge: payload.Challenge}, nil
	}
	if s.logger != nil {
		s.logger.Info("slack_event_received", slog.String("team_id", payload.TeamID), slog.String("event_type", payload.Event.Type))
	}

	route, err := s.store.GetInboundRoute(ctx, ConnectorName, payload.TeamID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return Result{}, fmt.Errorf("get slack inbound route: %w", err)
		}
		if s.logger != nil {
			s.logger.Info("slack_unroutable", slog.String("team_id", payload.TeamID))
		}
		if s.metrics != nil {
			s.metrics.IncInboundUnroutable(ConnectorName)
		}
		return Result{}, nil
	}

	eventType, mappedPayload, ok := canonicalEvent(payload.Event)
	if !ok {
		return Result{}, nil
	}
	if _, err := s.ingest.Ingest(ctx, ingest.Request{
		TenantID:   tenant.ID(route.TenantID),
		Type:       eventType,
		Source:     EventSource,
		SourceKind: stream.SourceKindExternal,
		Payload:    mappedPayload,
	}); err != nil {
		return Result{}, fmt.Errorf("ingest slack event: %w", err)
	}
	return Result{}, nil
}

type webhookPayload struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	TeamID    string     `json:"team_id"`
	Event     slackEvent `json:"event"`
}

type slackEvent struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Text    string `json:"text"`
	TS      string `json:"ts"`
}

func parseWebhook(rawBody []byte) (webhookPayload, error) {
	var payload webhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return webhookPayload{}, err
	}
	payload.Type = strings.TrimSpace(payload.Type)
	payload.Challenge = strings.TrimSpace(payload.Challenge)
	payload.TeamID = strings.TrimSpace(payload.TeamID)
	payload.Event.Type = strings.TrimSpace(payload.Event.Type)
	return payload, nil
}

func canonicalEvent(event slackEvent) (string, json.RawMessage, bool) {
	var eventType string
	switch event.Type {
	case "message.channels":
		eventType = schemas.FullName("slack.message.created", 1)
	case "app_mention":
		eventType = schemas.FullName("slack.app_mentioned", 1)
	case "reaction_added":
		eventType = schemas.FullName("slack.reaction.added", 1)
	default:
		return "", nil, false
	}
	body, err := json.Marshal(map[string]any{
		"user":    strings.TrimSpace(event.User),
		"channel": strings.TrimSpace(event.Channel),
		"text":    event.Text,
		"ts":      strings.TrimSpace(event.TS),
	})
	if err != nil {
		return "", nil, false
	}
	return eventType, json.RawMessage(body), true
}

func (s *Service) verify(headers http.Header, rawBody []byte) error {
	timestamp := strings.TrimSpace(headers.Get("X-Slack-Request-Timestamp"))
	signature := strings.TrimSpace(headers.Get("X-Slack-Signature"))
	if timestamp == "" || signature == "" || strings.TrimSpace(s.cfg.SigningSecret) == "" {
		return errors.New("slack webhook verification failed")
	}
	parsedTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return err
	}
	if delta := s.now().Unix() - parsedTimestamp; delta > 300 || delta < -300 {
		return errors.New("slack webhook timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.SigningSecret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":"))
	_, _ = mac.Write(rawBody)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("slack webhook signature mismatch")
	}
	return nil
}
