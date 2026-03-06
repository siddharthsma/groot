package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/stream"
	"groot/internal/tenant"
)

var (
	ErrInvalidType   = errors.New("type is required")
	ErrInvalidSource = errors.New("source is required")
)

type Request struct {
	TenantID tenant.ID
	Type     string
	Source   string
	Payload  json.RawMessage
}

type Publisher interface {
	PublishEvent(context.Context, stream.Event) error
}

type EventStore interface {
	SaveEvent(context.Context, stream.Event) error
}

type Logger interface {
	Info(string, ...any)
	Error(string, ...any)
}

type Metrics interface {
	IncEventsPublished()
	IncEventsRecorded()
}

type Service struct {
	publisher Publisher
	store     EventStore
	log       Logger
	metrics   Metrics
	now       func() time.Time
}

func NewService(publisher Publisher, store EventStore, logger Logger, metrics Metrics) *Service {
	return &Service{
		publisher: publisher,
		store:     store,
		log:       logger,
		metrics:   metrics,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Ingest(ctx context.Context, req Request) (stream.Event, error) {
	eventType := strings.TrimSpace(req.Type)
	if eventType == "" {
		return stream.Event{}, ErrInvalidType
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		return stream.Event{}, ErrInvalidSource
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}

	event := stream.Event{
		EventID:   uuid.New(),
		TenantID:  req.TenantID,
		Type:      eventType,
		Source:    source,
		Timestamp: s.now(),
		Payload:   payload,
	}

	if err := s.publisher.PublishEvent(ctx, event); err != nil {
		if s.log != nil {
			s.log.Error("event_publish_failed",
				"event_id", event.EventID.String(),
				"tenant_id", event.TenantID.String(),
				"event_type", event.Type,
				"error", err.Error(),
			)
		}
		return stream.Event{}, fmt.Errorf("publish event: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncEventsPublished()
	}

	if err := s.store.SaveEvent(ctx, event); err != nil {
		return stream.Event{}, fmt.Errorf("save event: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncEventsRecorded()
	}

	if s.log != nil {
		s.log.Info("event_published",
			"event_id", event.EventID.String(),
			"tenant_id", event.TenantID.String(),
			"event_type", event.Type,
		)
	}

	return event, nil
}
