package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"groot/internal/delivery"
	eventpkg "groot/internal/event"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/subscriptionfilter"
	"groot/internal/tenant"
)

type Store interface {
	ListMatchingSubscriptions(context.Context, tenant.ID, string, string) ([]subscription.Subscription, error)
	CreateDeliveryJob(context.Context, delivery.JobRecord) (bool, error)
}

type Consumer struct {
	reader  *kafka.Reader
	store   Store
	logger  *slog.Logger
	metrics Metrics
	now     func() time.Time
}

type Metrics interface {
	IncRouterEventsConsumed()
	IncRouterMatches()
	IncSubscriptionFilterEvaluations()
	IncSubscriptionFilterMatches()
	IncSubscriptionFilterRejections()
}

func NewConsumer(brokers []string, groupID string, store Store, logger *slog.Logger, metrics Metrics) *Consumer {
	if groupID == "" {
		groupID = "groot-router"
	}
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers,
			Topic:   stream.EventsTopic,
			GroupID: groupID,
		}),
		store:   store,
		logger:  logger,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	defer func() {
		_ = c.reader.Close()
	}()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("fetch kafka message: %w", err)
		}

		if err := c.processMessage(ctx, msg); err != nil {
			c.logger.Error("router_process_failed", slog.String("error", err.Error()))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			return fmt.Errorf("commit kafka message: %w", err)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	var event eventpkg.Event
	if err := jsonUnmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("decode event message: %w", err)
	}

	c.logger.Info("event_consumed",
		slog.String("event_id", event.EventID.String()),
		slog.String("tenant_id", event.TenantID.String()),
	)
	if c.metrics != nil {
		c.metrics.IncRouterEventsConsumed()
	}

	matches, err := c.store.ListMatchingSubscriptions(ctx, event.TenantID, event.Type, event.Source)
	if err != nil {
		return fmt.Errorf("list matching subscriptions: %w", err)
	}

	for _, sub := range matches {
		if len(sub.Filter) > 0 {
			if c.metrics != nil {
				c.metrics.IncSubscriptionFilterEvaluations()
			}
			matched, err := subscriptionfilter.Evaluate(sub.Filter, event.Payload)
			if err != nil {
				if c.metrics != nil {
					c.metrics.IncSubscriptionFilterRejections()
				}
				c.logger.Info("subscription_filter_invalid",
					slog.String("subscription_id", sub.ID.String()),
					slog.String("tenant_id", event.TenantID.String()),
					slog.String("event_id", event.EventID.String()),
					slog.String("error", err.Error()),
				)
				continue
			}
			c.logger.Info("subscription_filter_evaluated",
				slog.String("subscription_id", sub.ID.String()),
				slog.String("tenant_id", event.TenantID.String()),
				slog.String("event_id", event.EventID.String()),
				slog.Bool("matched", matched),
			)
			if !matched {
				continue
			}
			if c.metrics != nil {
				c.metrics.IncSubscriptionFilterMatches()
			}
		}
		if c.metrics != nil {
			c.metrics.IncRouterMatches()
		}
		c.logger.Info("subscription_matched",
			slog.String("event_id", event.EventID.String()),
			slog.String("tenant_id", event.TenantID.String()),
			slog.String("subscription_id", sub.ID.String()),
		)

		created, err := c.store.CreateDeliveryJob(ctx, delivery.JobRecord{
			ID:             uuid.New(),
			TenantID:       event.TenantID,
			SubscriptionID: sub.ID,
			EventID:        event.EventID,
			Status:         delivery.StatusPending,
			CreatedAt:      c.now(),
		})
		if err != nil {
			return fmt.Errorf("create delivery job: %w", err)
		}
		if created {
			c.logger.Info("delivery_job_created",
				slog.String("event_id", event.EventID.String()),
				slog.String("tenant_id", event.TenantID.String()),
				slog.String("subscription_id", sub.ID.String()),
			)
		}
	}
	return nil
}

func jsonUnmarshal(data []byte, event *eventpkg.Event) error {
	return eventpkg.UnmarshalEvent(data, event)
}
