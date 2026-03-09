package router

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"groot/internal/delivery"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type stubStore struct {
	matchFn  func(context.Context, tenant.ID, string, string) ([]subscription.Subscription, error)
	createFn func(context.Context, delivery.JobRecord) (bool, error)
}

func (s stubStore) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]subscription.Subscription, error) {
	return s.matchFn(ctx, tenantID, eventType, eventSource)
}

func (s stubStore) CreateDeliveryJob(ctx context.Context, record delivery.JobRecord) (bool, error) {
	return s.createFn(ctx, record)
}

type stubMetrics struct {
	evaluations int
	matches     int
	rejections  int
}

func (m *stubMetrics) IncRouterEventsConsumed()          {}
func (m *stubMetrics) IncRouterMatches()                 {}
func (m *stubMetrics) IncSubscriptionFilterEvaluations() { m.evaluations++ }
func (m *stubMetrics) IncSubscriptionFilterMatches()     { m.matches++ }
func (m *stubMetrics) IncSubscriptionFilterRejections()  { m.rejections++ }

func TestProcessMessageAppliesFilter(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	m := &stubMetrics{}
	created := 0
	consumer := &Consumer{
		store: stubStore{
			matchFn: func(context.Context, tenant.ID, string, string) ([]subscription.Subscription, error) {
				return []subscription.Subscription{
					{ID: uuid.New(), Filter: []byte(`{"path":"payload.currency","op":"==","value":"usd"}`)},
					{ID: uuid.New(), Filter: []byte(`{"path":"payload.amount","op":">=","value":1000}`)},
				}, nil
			},
			createFn: func(context.Context, delivery.JobRecord) (bool, error) {
				created++
				return true, nil
			},
		},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics: m,
		now:     func() time.Time { return time.Unix(0, 0).UTC() },
	}

	msg := kafka.Message{Value: []byte(`{"event_id":"11111111-1111-1111-1111-111111111111","tenant_id":"` + tenantID.String() + `","type":"example.event.v1","source":{"kind":"external","integration":"manual","connection_id":"11111111-1111-1111-1111-111111111111"},"source_kind":"external","timestamp":"2026-03-06T00:00:00Z","payload":{"currency":"usd","amount":120}}`)}
	if err := consumer.processMessage(context.Background(), msg); err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if got, want := created, 1; got != want {
		t.Fatalf("created = %d, want %d", got, want)
	}
	if got, want := m.evaluations, 2; got != want {
		t.Fatalf("evaluations = %d, want %d", got, want)
	}
	if got, want := m.matches, 1; got != want {
		t.Fatalf("filter matches = %d, want %d", got, want)
	}
}
