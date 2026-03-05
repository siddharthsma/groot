package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/delivery"
	"groot/internal/stream"
	"groot/internal/subscription"
)

const DeliverHTTPName = "deliver_http"

type Dependencies struct {
	Store      Store
	HTTPClient *http.Client
}

type Store interface {
	GetDeliveryJob(context.Context, uuid.UUID) (delivery.Job, error)
	GetSubscriptionByID(context.Context, uuid.UUID) (subscription.Subscription, error)
	GetConnectedApp(context.Context, uuid.UUID, uuid.UUID) (connectedapp.App, error)
	GetEvent(context.Context, uuid.UUID) (stream.Event, error)
	SetDeliveryJobAttempt(context.Context, uuid.UUID, int) error
	MarkDeliveryJobSucceeded(context.Context, uuid.UUID, time.Time) error
	MarkDeliveryJobRetryableFailure(context.Context, uuid.UUID, string) error
	MarkDeliveryJobDeadLetter(context.Context, uuid.UUID, string) error
	MarkDeliveryJobFailed(context.Context, uuid.UUID, string) error
}

type Activities struct {
	store      Store
	httpClient *http.Client
}

type DeliveryJob struct {
	ID             string
	TenantID       string
	SubscriptionID string
	EventID        string
}

type Subscription struct {
	ID             string
	ConnectedAppID string
}

type ConnectedApp struct {
	ID             string
	DestinationURL string
}

type Event struct {
	EventID   string          `json:"event_id"`
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func New(deps Dependencies) *Activities {
	client := deps.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Activities{store: deps.Store, httpClient: client}
}

func (a *Activities) LoadDeliveryJob(ctx context.Context, deliveryJobID string) (DeliveryJob, error) {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return DeliveryJob{}, err
	}
	job, err := a.store.GetDeliveryJob(ctx, id)
	if err != nil {
		return DeliveryJob{}, err
	}
	return DeliveryJob{ID: job.ID.String(), TenantID: job.TenantID.String(), SubscriptionID: job.SubscriptionID.String(), EventID: job.EventID.String()}, nil
}

func (a *Activities) LoadSubscription(ctx context.Context, subscriptionID string) (Subscription, error) {
	id, err := uuid.Parse(subscriptionID)
	if err != nil {
		return Subscription{}, err
	}
	sub, err := a.store.GetSubscriptionByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	return Subscription{ID: sub.ID.String(), ConnectedAppID: sub.ConnectedAppID.String()}, nil
}

func (a *Activities) LoadConnectedApp(ctx context.Context, connectedAppID string, tenantID string) (ConnectedApp, error) {
	appID, err := uuid.Parse(connectedAppID)
	if err != nil {
		return ConnectedApp{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return ConnectedApp{}, err
	}
	app, err := a.store.GetConnectedApp(ctx, tID, appID)
	if err != nil {
		return ConnectedApp{}, err
	}
	return ConnectedApp{ID: app.ID.String(), DestinationURL: app.DestinationURL}, nil
}

func (a *Activities) LoadEvent(ctx context.Context, eventID string) (Event, error) {
	id, err := uuid.Parse(eventID)
	if err != nil {
		return Event{}, err
	}
	event, err := a.store.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	return Event{
		EventID:   event.EventID.String(),
		TenantID:  event.TenantID.String(),
		Type:      event.Type,
		Source:    event.Source,
		Timestamp: event.Timestamp,
		Payload:   event.Payload,
	}, nil
}

func (a *Activities) RecordAttempt(ctx context.Context, deliveryJobID string, attempt int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.SetDeliveryJobAttempt(ctx, id, attempt)
}

func (a *Activities) DeliverHTTP(ctx context.Context, destinationURL string, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destinationURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (a *Activities) MarkSucceeded(ctx context.Context, deliveryJobID string) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.MarkDeliveryJobSucceeded(ctx, id, time.Now().UTC())
}

func (a *Activities) MarkRetryableFailure(ctx context.Context, deliveryJobID string, lastError string) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.MarkDeliveryJobRetryableFailure(ctx, id, lastError)
}

func (a *Activities) MarkDeadLetter(ctx context.Context, deliveryJobID string, lastError string) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.MarkDeliveryJobDeadLetter(ctx, id, lastError)
}

func (a *Activities) MarkFailed(ctx context.Context, deliveryJobID string, lastError string) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.MarkDeliveryJobFailed(ctx, id, lastError)
}
