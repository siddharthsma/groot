package replay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"groot/internal/config"
	"groot/internal/delivery"
	eventpkg "groot/internal/event"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

var (
	ErrEventNotFound        = errors.New("event not found")
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrSubscriptionInactive = errors.New("subscription is not active")
	ErrInvalidWindow        = errors.New("invalid replay window")
	ErrReplayLimitExceeded  = errors.New("replay request exceeds configured limits")
)

type SingleResult struct {
	EventID              uuid.UUID `json:"event_id"`
	MatchedSubscriptions int       `json:"matched_subscriptions"`
	JobsCreated          int       `json:"jobs_created"`
}

type QueryRequest struct {
	From           time.Time
	To             time.Time
	Type           string
	Source         string
	SubscriptionID *uuid.UUID
}

type QueryResult struct {
	EventsScanned int `json:"events_scanned"`
	JobsCreated   int `json:"jobs_created"`
}

type Store interface {
	GetEventForTenant(context.Context, tenant.ID, uuid.UUID) (eventpkg.Event, error)
	ListEvents(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]eventpkg.Event, error)
	ListSubscriptions(context.Context, tenant.ID) ([]subscription.Subscription, error)
	GetSubscriptionByID(context.Context, uuid.UUID) (subscription.Subscription, error)
	CreateDeliveryJob(context.Context, delivery.JobRecord) (bool, error)
}

type Metrics interface {
	IncReplayRequests()
	IncReplayJobsCreated(int)
}

type Service struct {
	store   Store
	cfg     config.ReplayConfig
	metrics Metrics
	now     func() time.Time
}

func NewService(store Store, cfg config.ReplayConfig, metrics Metrics) *Service {
	return &Service{
		store:   store,
		cfg:     cfg,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ReplayEvent(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (SingleResult, error) {
	event, err := s.store.GetEventForTenant(ctx, tenantID, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SingleResult{}, ErrEventNotFound
		}
		return SingleResult{}, fmt.Errorf("get event: %w", err)
	}
	subs, err := s.store.ListSubscriptions(ctx, tenantID)
	if err != nil {
		return SingleResult{}, fmt.Errorf("list subscriptions: %w", err)
	}
	matches := matchingSubscriptions(event, subs, nil)
	if s.metrics != nil {
		s.metrics.IncReplayRequests()
	}
	jobsCreated, err := s.createReplayJobs(ctx, event, matches)
	if err != nil {
		return SingleResult{}, err
	}
	return SingleResult{EventID: event.EventID, MatchedSubscriptions: len(matches), JobsCreated: jobsCreated}, nil
}

func (s *Service) ReplayQuery(ctx context.Context, tenantID tenant.ID, req QueryRequest) (QueryResult, error) {
	if !req.To.After(req.From) {
		return QueryResult{}, ErrInvalidWindow
	}
	if req.To.Sub(req.From) > time.Duration(s.cfg.MaxWindowHours)*time.Hour {
		return QueryResult{}, ErrReplayLimitExceeded
	}
	events, err := s.store.ListEvents(ctx, tenantID, req.Type, req.Source, &req.From, &req.To, s.cfg.MaxEvents+1)
	if err != nil {
		return QueryResult{}, fmt.Errorf("list events: %w", err)
	}
	if len(events) > s.cfg.MaxEvents {
		return QueryResult{}, ErrReplayLimitExceeded
	}

	var scopedSubscription *subscription.Subscription
	if req.SubscriptionID != nil {
		sub, err := s.store.GetSubscriptionByID(ctx, *req.SubscriptionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return QueryResult{}, ErrSubscriptionNotFound
			}
			return QueryResult{}, fmt.Errorf("get subscription: %w", err)
		}
		if sub.TenantID != uuid.UUID(tenantID) {
			return QueryResult{}, ErrSubscriptionNotFound
		}
		if sub.Status != subscription.StatusActive {
			return QueryResult{}, ErrSubscriptionInactive
		}
		scopedSubscription = &sub
	}

	if s.metrics != nil {
		s.metrics.IncReplayRequests()
	}

	eventsScanned := len(events)
	plannedJobs := 0
	seen := make(map[string]struct{})
	for _, event := range events {
		var matches []subscription.Subscription
		if scopedSubscription != nil {
			matches = matchingSubscriptions(event, []subscription.Subscription{*scopedSubscription}, nil)
		} else {
			subs, err := s.store.ListSubscriptions(ctx, tenantID)
			if err != nil {
				return QueryResult{}, fmt.Errorf("list subscriptions: %w", err)
			}
			matches = matchingSubscriptions(event, subs, nil)
		}
		for _, sub := range matches {
			key := event.EventID.String() + ":" + sub.ID.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			plannedJobs++
		}
	}
	if plannedJobs > s.cfg.MaxEvents {
		return QueryResult{}, ErrReplayLimitExceeded
	}

	jobsCreated := 0
	seen = make(map[string]struct{})
	var subscriptionsCache []subscription.Subscription
	if scopedSubscription == nil {
		subscriptionsCache, err = s.store.ListSubscriptions(ctx, tenantID)
		if err != nil {
			return QueryResult{}, fmt.Errorf("list subscriptions: %w", err)
		}
	}
	for _, event := range events {
		var matches []subscription.Subscription
		if scopedSubscription != nil {
			matches = matchingSubscriptions(event, []subscription.Subscription{*scopedSubscription}, nil)
		} else {
			matches = matchingSubscriptions(event, subscriptionsCache, nil)
		}
		for _, sub := range matches {
			key := event.EventID.String() + ":" + sub.ID.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			created, err := s.store.CreateDeliveryJob(ctx, delivery.JobRecord{
				ID:              uuid.New(),
				TenantID:        event.TenantID,
				SubscriptionID:  sub.ID,
				EventID:         event.EventID,
				IsReplay:        true,
				ReplayOfEventID: &event.EventID,
				Status:          delivery.StatusPending,
				CreatedAt:       s.now(),
			})
			if err != nil {
				return QueryResult{}, fmt.Errorf("create replay delivery job: %w", err)
			}
			if created {
				jobsCreated++
			}
		}
	}
	if s.metrics != nil {
		s.metrics.IncReplayJobsCreated(jobsCreated)
	}
	return QueryResult{EventsScanned: eventsScanned, JobsCreated: jobsCreated}, nil
}

func (s *Service) createReplayJobs(ctx context.Context, event eventpkg.Event, matches []subscription.Subscription) (int, error) {
	jobsCreated := 0
	for _, sub := range matches {
		created, err := s.store.CreateDeliveryJob(ctx, delivery.JobRecord{
			ID:              uuid.New(),
			TenantID:        event.TenantID,
			SubscriptionID:  sub.ID,
			EventID:         event.EventID,
			IsReplay:        true,
			ReplayOfEventID: &event.EventID,
			Status:          delivery.StatusPending,
			CreatedAt:       s.now(),
		})
		if err != nil {
			return 0, fmt.Errorf("create replay delivery job: %w", err)
		}
		if created {
			jobsCreated++
		}
	}
	if s.metrics != nil {
		s.metrics.IncReplayJobsCreated(jobsCreated)
	}
	return jobsCreated, nil
}

func matchingSubscriptions(event eventpkg.Event, subs []subscription.Subscription, only *uuid.UUID) []subscription.Subscription {
	result := make([]subscription.Subscription, 0, len(subs))
	for _, sub := range subs {
		if only != nil && sub.ID != *only {
			continue
		}
		if sub.Status != subscription.StatusActive {
			continue
		}
		if sub.EventType != event.Type {
			continue
		}
		if sub.EventSource != nil && *sub.EventSource != event.Source {
			continue
		}
		result = append(result, sub)
	}
	return result
}
