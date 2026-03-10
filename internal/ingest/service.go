package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	eventpkg "groot/internal/event"
	"groot/internal/schema"
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
	TenantID       tenant.ID
	WorkflowRunID  *uuid.UUID
	WorkflowNodeID string
	Type           string
	Source         string
	SourceInfo     eventpkg.Source
	SourceKind     string
	Lineage        *eventpkg.Lineage
	ChainDepth     int
	Payload        json.RawMessage
}

type Publisher interface {
	PublishEvent(context.Context, eventpkg.Event) error
}

type EventStore interface {
	SaveEvent(context.Context, eventpkg.Event) error
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
	ValidateEvent(context.Context, string, string, string, json.RawMessage) (schema.Schema, error)
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

func (s *Service) Ingest(ctx context.Context, req Request) (eventpkg.Event, error) {
	eventType := strings.TrimSpace(req.Type)
	if eventType == "" {
		return eventpkg.Event{}, ErrInvalidType
	}
	if _, _, ok := schema.ParseFullName(eventType); !ok {
		return eventpkg.Event{}, ErrInvalidVersionedType
	}

	source := eventpkg.NormalizeSource(req.SourceInfo, normalizeSourceKind(req.SourceKind))
	if source.Integration == "" {
		source.Integration = strings.TrimSpace(req.Source)
	}
	if source.Integration == "" {
		return eventpkg.Event{}, ErrInvalidSource
	}
	if source.Kind == "" {
		return eventpkg.Event{}, ErrInvalidSourceKind
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}

	evt := eventpkg.Event{
		EventID:        uuid.New(),
		TenantID:       req.TenantID,
		WorkflowRunID:  req.WorkflowRunID,
		WorkflowNodeID: strings.TrimSpace(req.WorkflowNodeID),
		Type:           eventType,
		Source:         source,
		SourceKind:     source.Kind,
		Lineage:        eventpkg.NormalizeLineage(req.Lineage),
		ChainDepth:     req.ChainDepth,
		Timestamp:      s.now(),
		Payload:        payload,
	}
	if evt.ChainDepth < 0 {
		return eventpkg.Event{}, ErrInvalidChainDepth
	}
	if s.schemas != nil {
		schema, err := s.schemas.ValidateEvent(ctx, evt.Type, evt.SourceIntegration(), evt.SourceKind, evt.Payload)
		if err != nil {
			return eventpkg.Event{}, err
		}
		if schema.FullName != "" {
			evt.SchemaFullName = schema.FullName
			evt.SchemaVersion = schema.Version
		}
	}

	if err := s.publisher.PublishEvent(ctx, evt); err != nil {
		if s.log != nil {
			s.log.Error("event_publish_failed",
				"event_id", evt.EventID.String(),
				"tenant_id", evt.TenantID.String(),
				"event_type", evt.Type,
				"error", err.Error(),
			)
		}
		return eventpkg.Event{}, fmt.Errorf("publish event: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncEventsPublished()
	}

	if err := s.store.SaveEvent(ctx, evt); err != nil {
		return eventpkg.Event{}, fmt.Errorf("save event: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncEventsRecorded()
	}

	if s.log != nil {
		s.log.Info("event_published",
			"event_id", evt.EventID.String(),
			"tenant_id", evt.TenantID.String(),
			"event_type", evt.Type,
		)
	}

	return evt, nil
}

func normalizeSourceKind(value string) string {
	switch strings.TrimSpace(value) {
	case "", eventpkg.SourceKindExternal:
		return eventpkg.SourceKindExternal
	case eventpkg.SourceKindInternal:
		return eventpkg.SourceKindInternal
	default:
		return ""
	}
}
