package eventquery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/stream"
	"groot/internal/tenant"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

type Event struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Source     string    `json:"source"`
	SourceKind string    `json:"source_kind"`
	ChainDepth int       `json:"chain_depth"`
	OccurredAt time.Time `json:"occurred_at"`
}

type AdminEvent struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	SourceKind string          `json:"source_kind"`
	CreatedAt  time.Time       `json:"created_at"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

type Store interface {
	ListEvents(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]stream.Event, error)
	ListEventsAdmin(context.Context, tenant.ID, string, *time.Time, *time.Time, int) ([]stream.Event, error)
	GetEvent(context.Context, uuid.UUID) (stream.Event, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]Event, error) {
	normalizedLimit := normalizeLimit(limit)
	events, err := s.store.ListEvents(ctx, tenantID, strings.TrimSpace(eventType), strings.TrimSpace(source), from, to, normalizedLimit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	result := make([]Event, 0, len(events))
	for _, event := range events {
		result = append(result, Event{
			ID:         event.EventID.String(),
			Type:       event.Type,
			Source:     event.Source,
			SourceKind: event.SourceKind,
			ChainDepth: event.ChainDepth,
			OccurredAt: event.Timestamp,
		})
	}
	return result, nil
}

func (s *Service) AdminList(ctx context.Context, tenantID tenant.ID, eventType string, from, to *time.Time, limit int, includePayload bool) ([]AdminEvent, error) {
	normalizedLimit := normalizeLimit(limit)
	events, err := s.store.ListEventsAdmin(ctx, tenantID, strings.TrimSpace(eventType), from, to, normalizedLimit)
	if err != nil {
		return nil, fmt.Errorf("list admin events: %w", err)
	}
	result := make([]AdminEvent, 0, len(events))
	for _, event := range events {
		record := AdminEvent{
			ID:         event.EventID.String(),
			TenantID:   event.TenantID.String(),
			Type:       event.Type,
			Source:     event.Source,
			SourceKind: event.SourceKind,
			CreatedAt:  event.Timestamp,
		}
		if includePayload {
			record.Payload = event.Payload
		}
		result = append(result, record)
	}
	return result, nil
}

func (s *Service) AdminGet(ctx context.Context, eventID uuid.UUID, includePayload bool) (AdminEvent, error) {
	event, err := s.store.GetEvent(ctx, eventID)
	if err != nil {
		return AdminEvent{}, fmt.Errorf("get admin event: %w", err)
	}
	record := AdminEvent{
		ID:         event.EventID.String(),
		TenantID:   event.TenantID.String(),
		Type:       event.Type,
		Source:     event.Source,
		SourceKind: event.SourceKind,
		CreatedAt:  event.Timestamp,
	}
	if includePayload {
		record.Payload = event.Payload
	}
	return record, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}
