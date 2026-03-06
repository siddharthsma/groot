package replay

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/config"
	"groot/internal/delivery"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

type stubStore struct {
	getEventForTenantFn   func(context.Context, tenant.ID, uuid.UUID) (stream.Event, error)
	listEventsFn          func(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]stream.Event, error)
	listSubscriptionsFn   func(context.Context, tenant.ID) ([]subscription.Subscription, error)
	getSubscriptionByIDFn func(context.Context, uuid.UUID) (subscription.Subscription, error)
	createDeliveryJobFn   func(context.Context, delivery.JobRecord) (bool, error)
}

func (s stubStore) GetEventForTenant(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (stream.Event, error) {
	return s.getEventForTenantFn(ctx, tenantID, eventID)
}
func (s stubStore) ListEvents(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]stream.Event, error) {
	return s.listEventsFn(ctx, tenantID, eventType, source, from, to, limit)
}
func (s stubStore) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	return s.listSubscriptionsFn(ctx, tenantID)
}
func (s stubStore) GetSubscriptionByID(ctx context.Context, id uuid.UUID) (subscription.Subscription, error) {
	return s.getSubscriptionByIDFn(ctx, id)
}
func (s stubStore) CreateDeliveryJob(ctx context.Context, record delivery.JobRecord) (bool, error) {
	return s.createDeliveryJobFn(ctx, record)
}

type stubMetrics struct {
	requests uint64
	jobs     int
}

func (m *stubMetrics) IncReplayRequests()         { m.requests++ }
func (m *stubMetrics) IncReplayJobsCreated(n int) { m.jobs += n }

func TestReplayEventCreatesReplayJobs(t *testing.T) {
	eventID := uuid.New()
	tenantID := tenant.ID(uuid.New())
	subscriptionID := uuid.New()
	metrics := &stubMetrics{}
	called := false
	svc := NewService(stubStore{
		getEventForTenantFn: func(context.Context, tenant.ID, uuid.UUID) (stream.Event, error) {
			return stream.Event{EventID: eventID, TenantID: uuid.UUID(tenantID), Type: "example.event.v1", Source: "manual"}, nil
		},
		listSubscriptionsFn: func(context.Context, tenant.ID) ([]subscription.Subscription, error) {
			return []subscription.Subscription{{ID: subscriptionID, EventType: "example.event.v1", Status: subscription.StatusActive}}, nil
		},
		createDeliveryJobFn: func(_ context.Context, record delivery.JobRecord) (bool, error) {
			called = true
			if !record.IsReplay || record.ReplayOfEventID == nil || *record.ReplayOfEventID != eventID {
				t.Fatal("expected replay metadata")
			}
			return true, nil
		},
	}, config.ReplayConfig{MaxEvents: 1000, MaxWindowHours: 24}, metrics)

	result, err := svc.ReplayEvent(context.Background(), tenantID, eventID)
	if err != nil {
		t.Fatalf("ReplayEvent() error = %v", err)
	}
	if !called || result.JobsCreated != 1 || metrics.jobs != 1 {
		t.Fatalf("unexpected result: %+v jobs=%d", result, metrics.jobs)
	}
}

func TestReplayQueryRejectsInactiveSubscription(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	subID := uuid.New()
	from := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	svc := NewService(stubStore{
		listEventsFn: func(context.Context, tenant.ID, string, string, *time.Time, *time.Time, int) ([]stream.Event, error) {
			return []stream.Event{}, nil
		},
		getSubscriptionByIDFn: func(context.Context, uuid.UUID) (subscription.Subscription, error) {
			return subscription.Subscription{ID: subID, TenantID: uuid.UUID(tenantID), Status: subscription.StatusPaused}, nil
		},
	}, config.ReplayConfig{MaxEvents: 1000, MaxWindowHours: 24}, nil)

	_, err := svc.ReplayQuery(context.Background(), tenantID, QueryRequest{From: from, To: to, SubscriptionID: &subID})
	if !errors.Is(err, ErrSubscriptionInactive) {
		t.Fatalf("ReplayQuery() error = %v", err)
	}
}

func TestReplayEventNotFound(t *testing.T) {
	svc := NewService(stubStore{
		getEventForTenantFn: func(context.Context, tenant.ID, uuid.UUID) (stream.Event, error) {
			return stream.Event{}, sql.ErrNoRows
		},
	}, config.ReplayConfig{MaxEvents: 1000, MaxWindowHours: 24}, nil)
	_, err := svc.ReplayEvent(context.Background(), tenant.ID(uuid.New()), uuid.New())
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("ReplayEvent() error = %v", err)
	}
}
