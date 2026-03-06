package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/delivery"
	"groot/internal/stream"
	"groot/internal/subscription"
)

type stubPublisher struct {
	published []stream.Event
}

func (s *stubPublisher) PublishEvent(_ context.Context, event stream.Event) error {
	s.published = append(s.published, event)
	return nil
}

type stubStore struct {
	linkedJobID uuid.UUID
	linkedEvent stream.Event
	linked      bool
}

func (s *stubStore) SaveResultEvent(_ context.Context, jobID uuid.UUID, event stream.Event) (bool, error) {
	s.linkedJobID = jobID
	s.linkedEvent = event
	s.linked = true
	return true, nil
}

type stubMetrics struct {
	emitted  int
	failures int
}

func (s *stubMetrics) IncResultEventsEmitted(_, _, _ string) { s.emitted++ }
func (s *stubMetrics) IncResultEventEmitFailures()           { s.failures++ }

func TestEmitResultEventPublishesAndLinks(t *testing.T) {
	publisher := &stubPublisher{}
	store := &stubStore{}
	metrics := &stubMetrics{}
	emitter := NewEmitter(publisher, store, slog.Default(), metrics, 10)
	emitter.now = func() time.Time { return time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC) }

	inputEventID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	jobID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	subscriptionID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	externalID := "page_123"
	httpStatus := 200

	err := emitter.EmitResultEvent(context.Background(), EmitRequest{
		Subscription: subscription.Subscription{ID: subscriptionID},
		DeliveryJob:  delivery.Job{ID: jobID},
		InputEvent: stream.Event{
			EventID:    inputEventID,
			TenantID:   tenantID,
			Type:       "resend.email.received.v1",
			Source:     "resend",
			SourceKind: stream.SourceKindExternal,
			ChainDepth: 0,
			Timestamp:  time.Date(2026, 3, 6, 11, 0, 0, 0, time.UTC),
			Payload:    json.RawMessage(`{"text":"hello"}`),
		},
		Connector:  "llm",
		Operation:  "summarize",
		Status:     ResultStatusSucceeded,
		Output:     map[string]any{"text": "summary"},
		ExternalID: &externalID,
		HTTPStatus: &httpStatus,
	})
	if err != nil {
		t.Fatalf("EmitResultEvent() error = %v", err)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("published = %d, want 1", len(publisher.published))
	}
	if !store.linked {
		t.Fatal("expected result event to be linked")
	}
	if metrics.emitted != 1 {
		t.Fatalf("metrics emitted = %d", metrics.emitted)
	}
	event := publisher.published[0]
	if event.Type != "llm.summarize.completed.v1" {
		t.Fatalf("event.Type = %q", event.Type)
	}
	if event.SourceKind != stream.SourceKindInternal {
		t.Fatalf("event.SourceKind = %q", event.SourceKind)
	}
	if event.ChainDepth != 1 {
		t.Fatalf("event.ChainDepth = %d", event.ChainDepth)
	}
}

func TestEmitResultEventSkipsWhenChainDepthExceeded(t *testing.T) {
	publisher := &stubPublisher{}
	store := &stubStore{}
	metrics := &stubMetrics{}
	emitter := NewEmitter(publisher, store, slog.Default(), metrics, 1)

	err := emitter.EmitResultEvent(context.Background(), EmitRequest{
		Subscription: subscription.Subscription{ID: uuid.New()},
		DeliveryJob:  delivery.Job{ID: uuid.New()},
		InputEvent: stream.Event{
			EventID:    uuid.New(),
			TenantID:   uuid.New(),
			Type:       "llm.summarize.completed.v1",
			Source:     "llm",
			SourceKind: stream.SourceKindInternal,
			ChainDepth: 1,
			Timestamp:  time.Now().UTC(),
			Payload:    json.RawMessage(`{}`),
		},
		Connector: "llm",
		Operation: "summarize",
		Status:    ResultStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("EmitResultEvent() error = %v", err)
	}
	if len(publisher.published) != 0 {
		t.Fatalf("published = %d, want 0", len(publisher.published))
	}
	if store.linked {
		t.Fatal("did not expect result event to be linked")
	}
	if metrics.emitted != 0 || metrics.failures != 0 {
		t.Fatalf("metrics = %+v", metrics)
	}
}
