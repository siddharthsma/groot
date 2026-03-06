package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/schemas"
	"groot/internal/stream"
)

type stubPublisher struct {
	publishFn func(context.Context, stream.Event) error
}

func (s stubPublisher) PublishEvent(ctx context.Context, event stream.Event) error {
	return s.publishFn(ctx, event)
}

type stubStore struct {
	saveFn func(context.Context, stream.Event) error
}

func (s stubStore) SaveEvent(ctx context.Context, event stream.Event) error {
	return s.saveFn(ctx, event)
}

type stubLogger struct{}

func (stubLogger) Info(string, ...any)  {}
func (stubLogger) Error(string, ...any) {}

type stubMetrics struct{}

func (stubMetrics) IncEventsPublished() {}
func (stubMetrics) IncEventsRecorded()  {}

type stubSchemaValidator struct {
	validateFn func(context.Context, string, string, string, json.RawMessage) (schemas.Schema, error)
}

func (s stubSchemaValidator) ValidateEvent(ctx context.Context, fullName, source, sourceKind string, payload json.RawMessage) (schemas.Schema, error) {
	return s.validateFn(ctx, fullName, source, sourceKind, payload)
}

func TestIngest(t *testing.T) {
	now := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	tenantID := uuid.New()
	svc := NewService(stubPublisher{
		publishFn: func(_ context.Context, event stream.Event) error {
			if event.TenantID != tenantID {
				t.Fatalf("event.TenantID = %s", event.TenantID)
			}
			if string(event.Payload) != `{"ok":true}` {
				t.Fatalf("event.Payload = %s", event.Payload)
			}
			return nil
		},
	}, stubStore{saveFn: func(context.Context, stream.Event) error { return nil }}, stubLogger{}, stubMetrics{})
	svc.now = func() time.Time { return now }

	event, err := svc.Ingest(context.Background(), Request{
		TenantID: tenantID,
		Type:     "example.event.v1",
		Source:   "manual",
		Payload:  json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if event.Type != "example.event.v1" {
		t.Fatalf("event.Type = %q", event.Type)
	}
}

func TestIngestValidation(t *testing.T) {
	svc := NewService(stubPublisher{publishFn: func(context.Context, stream.Event) error { return nil }}, stubStore{saveFn: func(context.Context, stream.Event) error { return nil }}, stubLogger{}, stubMetrics{})

	_, err := svc.Ingest(context.Background(), Request{Source: "manual"})
	if !errors.Is(err, ErrInvalidType) {
		t.Fatalf("Ingest() error = %v, want %v", err, ErrInvalidType)
	}
}

func TestIngestRejectsUnversionedType(t *testing.T) {
	svc := NewService(stubPublisher{publishFn: func(context.Context, stream.Event) error { return nil }}, stubStore{saveFn: func(context.Context, stream.Event) error { return nil }}, stubLogger{}, stubMetrics{})
	_, err := svc.Ingest(context.Background(), Request{Type: "example.event", Source: "manual"})
	if !errors.Is(err, ErrInvalidVersionedType) {
		t.Fatalf("Ingest() error = %v, want %v", err, ErrInvalidVersionedType)
	}
}

func TestIngestStoresSchemaMetadata(t *testing.T) {
	now := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	tenantID := uuid.New()
	svc := NewService(
		stubPublisher{publishFn: func(context.Context, stream.Event) error { return nil }},
		stubStore{saveFn: func(_ context.Context, event stream.Event) error {
			if got, want := event.SchemaFullName, "example.event.v1"; got != want {
				t.Fatalf("SchemaFullName = %q, want %q", got, want)
			}
			if got, want := event.SchemaVersion, 1; got != want {
				t.Fatalf("SchemaVersion = %d, want %d", got, want)
			}
			return nil
		}},
		stubLogger{},
		stubMetrics{},
		WithSchemaValidator(stubSchemaValidator{
			validateFn: func(context.Context, string, string, string, json.RawMessage) (schemas.Schema, error) {
				return schemas.Schema{FullName: "example.event.v1", Version: 1}, nil
			},
		}),
	)
	svc.now = func() time.Time { return now }

	if _, err := svc.Ingest(context.Background(), Request{
		TenantID: tenantID,
		Type:     "example.event.v1",
		Source:   "manual",
		Payload:  json.RawMessage(`{"ok":true}`),
	}); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
}
