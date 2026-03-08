package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/config"
	"groot/internal/event"
	"groot/internal/inboundroute"
	"groot/internal/ingest"
)

type stubStore struct {
	getInboundRouteFn func(context.Context, string, string) (inboundroute.Route, error)
}

func (s stubStore) GetInboundRoute(ctx context.Context, connectorName, routeKey string) (inboundroute.Route, error) {
	return s.getInboundRouteFn(ctx, connectorName, routeKey)
}

type stubIngestor struct {
	ingestFn func(context.Context, ingest.Request) (event.Event, error)
}

func (s stubIngestor) Ingest(ctx context.Context, req ingest.Request) (event.Event, error) {
	return s.ingestFn(ctx, req)
}

type stubMetrics struct{ received int }

func (s *stubMetrics) IncSlackEventsReceived()     { s.received++ }
func (s *stubMetrics) IncInboundUnroutable(string) {}

func TestHandleEventsPublishesCanonicalEvent(t *testing.T) {
	secret := "slack-secret"
	now := time.Unix(1710000000, 0).UTC()
	rawBody := []byte(`{"team_id":"T123","event":{"type":"app_mention","user":"U1","channel":"C1","text":"hello","ts":"123.45"}}`)
	headers := signedHeaders(secret, now, rawBody)

	called := false
	svc := NewService(config.SlackConfig{SigningSecret: secret}, stubStore{
		getInboundRouteFn: func(context.Context, string, string) (inboundroute.Route, error) {
			return inboundroute.Route{TenantID: uuid.New()}, nil
		},
	}, stubIngestor{
		ingestFn: func(_ context.Context, req ingest.Request) (event.Event, error) {
			called = true
			if got, want := req.Type, "slack.app_mentioned.v1"; got != want {
				t.Fatalf("Type = %q, want %q", got, want)
			}
			if got, want := req.Source, EventSource; got != want {
				t.Fatalf("Source = %q, want %q", got, want)
			}
			if got, want := req.SourceKind, event.SourceKindExternal; got != want {
				t.Fatalf("SourceKind = %q, want %q", got, want)
			}
			return event.Event{}, nil
		},
	}, slog.Default(), &stubMetrics{})
	svc.now = func() time.Time { return now }

	result, err := svc.HandleEvents(context.Background(), rawBody, headers)
	if err != nil {
		t.Fatalf("HandleEvents() error = %v", err)
	}
	if result.IsChallenge {
		t.Fatal("unexpected challenge response")
	}
	if !called {
		t.Fatal("expected ingest call")
	}
}

func TestHandleEventsURLVerification(t *testing.T) {
	secret := "slack-secret"
	now := time.Unix(1710000000, 0).UTC()
	rawBody := []byte(`{"type":"url_verification","challenge":"abc123","team_id":"T123"}`)
	headers := signedHeaders(secret, now, rawBody)

	svc := NewService(config.SlackConfig{SigningSecret: secret}, stubStore{}, stubIngestor{}, slog.Default(), &stubMetrics{})
	svc.now = func() time.Time { return now }

	result, err := svc.HandleEvents(context.Background(), rawBody, headers)
	if err != nil {
		t.Fatalf("HandleEvents() error = %v", err)
	}
	if !result.IsChallenge || result.Challenge != "abc123" {
		t.Fatalf("result = %+v", result)
	}
}

func signedHeaders(secret string, now time.Time, rawBody []byte) http.Header {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + strconv.FormatInt(now.Unix(), 10) + ":"))
	_, _ = mac.Write(rawBody)
	return http.Header{
		"X-Slack-Request-Timestamp": []string{strconv.FormatInt(now.Unix(), 10)},
		"X-Slack-Signature":         []string{"v0=" + hex.EncodeToString(mac.Sum(nil))},
	}
}
