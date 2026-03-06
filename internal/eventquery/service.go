package eventquery

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	OccurredAt time.Time `json:"occurred_at"`
}

type Store interface {
	ListEvents(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]stream.Event, error)
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
			OccurredAt: event.Timestamp,
		})
	}
	return result, nil
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
