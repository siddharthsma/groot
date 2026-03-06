package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/schemas"
	"groot/internal/stream"
	"groot/internal/tenant"
)

var (
	ErrInvalidType          = errors.New("type is required")
	ErrInvalidSource        = errors.New("source is required")
	ErrInvalidSourceKind    = errors.New("source_kind must be external or internal")
	ErrInvalidChainDepth    = errors.New("chain_depth must be at least 0")
	ErrInvalidVersionedType = errors.New("type must be versioned like <event>.v1")
)

type Request struct {
	TenantID   tenant.ID
	Type       string
	Source     string
	SourceKind string
	ChainDepth int
	Payload    json.RawMessage
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
	schemas   SchemaValidator
}

type SchemaValidator interface {
	ValidateEvent(context.Context, string, string, string, json.RawMessage) (schemas.Schema, error)
}

type Option func(*Service)

func WithSchemaValidator(validator SchemaValidator) Option {
	return func(s *Service) {
		s.schemas = validator
	}
}

func NewService(publisher Publisher, store EventStore, logger Logger, metrics Metrics, options ...Option) *Service {
	service := &Service{
		publisher: publisher,
		store:     store,
		log:       logger,
		metrics:   metrics,
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Ingest(ctx context.Context, req Request) (stream.Event, error) {
	eventType := strings.TrimSpace(req.Type)
	if eventType == "" {
		return stream.Event{}, ErrInvalidType
	}
	if _, _, ok := schemas.ParseFullName(eventType); !ok {
		return stream.Event{}, ErrInvalidVersionedType
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
		EventID:    uuid.New(),
		TenantID:   req.TenantID,
		Type:       eventType,
		Source:     source,
		SourceKind: normalizeSourceKind(req.SourceKind),
		ChainDepth: req.ChainDepth,
		Timestamp:  s.now(),
		Payload:    payload,
	}
	if event.SourceKind == "" {
		return stream.Event{}, ErrInvalidSourceKind
	}
	if event.ChainDepth < 0 {
		return stream.Event{}, ErrInvalidChainDepth
	}
	if s.schemas != nil {
		schema, err := s.schemas.ValidateEvent(ctx, event.Type, event.Source, event.SourceKind, event.Payload)
		if err != nil {
			return stream.Event{}, err
		}
		if schema.FullName != "" {
			event.SchemaFullName = schema.FullName
			event.SchemaVersion = schema.Version
		}
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

func normalizeSourceKind(value string) string {
	switch strings.TrimSpace(value) {
	case "", stream.SourceKindExternal:
		return stream.SourceKindExternal
	case stream.SourceKindInternal:
		return stream.SourceKindInternal
	default:
		return ""
	}
}
