package event

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

type ListEvent struct {
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

type QueryStore interface {
	ListEvents(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]Event, error)
	ListEventsAdmin(context.Context, tenant.ID, string, *time.Time, *time.Time, int) ([]Event, error)
	GetEvent(context.Context, uuid.UUID) (Event, error)
}

type QueryService struct {
	store QueryStore
}

func NewQueryService(store QueryStore) *QueryService {
	return &QueryService{store: store}
}

func (s *QueryService) List(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]ListEvent, error) {
	normalizedLimit := normalizeListLimit(limit)
	events, err := s.store.ListEvents(ctx, tenantID, strings.TrimSpace(eventType), strings.TrimSpace(source), from, to, normalizedLimit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	result := make([]ListEvent, 0, len(events))
	for _, evt := range events {
		result = append(result, ListEvent{
			ID:         evt.EventID.String(),
			Type:       evt.Type,
			Source:     evt.Source,
			SourceKind: evt.SourceKind,
			ChainDepth: evt.ChainDepth,
			OccurredAt: evt.Timestamp,
		})
	}
	return result, nil
}

func (s *QueryService) AdminList(ctx context.Context, tenantID tenant.ID, eventType string, from, to *time.Time, limit int, includePayload bool) ([]AdminEvent, error) {
	normalizedLimit := normalizeListLimit(limit)
	events, err := s.store.ListEventsAdmin(ctx, tenantID, strings.TrimSpace(eventType), from, to, normalizedLimit)
	if err != nil {
		return nil, fmt.Errorf("list admin events: %w", err)
	}
	result := make([]AdminEvent, 0, len(events))
	for _, evt := range events {
		record := AdminEvent{
			ID:         evt.EventID.String(),
			TenantID:   evt.TenantID.String(),
			Type:       evt.Type,
			Source:     evt.Source,
			SourceKind: evt.SourceKind,
			CreatedAt:  evt.Timestamp,
		}
		if includePayload {
			record.Payload = evt.Payload
		}
		result = append(result, record)
	}
	return result, nil
}

func (s *QueryService) AdminGet(ctx context.Context, eventID uuid.UUID, includePayload bool) (AdminEvent, error) {
	evt, err := s.store.GetEvent(ctx, eventID)
	if err != nil {
		return AdminEvent{}, fmt.Errorf("get admin event: %w", err)
	}
	record := AdminEvent{
		ID:         evt.EventID.String(),
		TenantID:   evt.TenantID.String(),
		Type:       evt.Type,
		Source:     evt.Source,
		SourceKind: evt.SourceKind,
		CreatedAt:  evt.Timestamp,
	}
	if includePayload {
		record.Payload = evt.Payload
	}
	return record, nil
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}
