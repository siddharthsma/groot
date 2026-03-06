package activities

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/outbound"
	slackconnector "groot/internal/connectors/outbound/slack"
	"groot/internal/delivery"
	"groot/internal/functiondestination"
	"groot/internal/stream"
	"groot/internal/subscription"
)

const DeliverHTTPName = "deliver_http"
const InvokeFunctionName = "invoke_function"
const ExecuteConnectorName = "execute_connector"

type Dependencies struct {
	Store           Store
	HTTPClient      *http.Client
	SlackAPIBaseURL string
	Metrics         Metrics
	Logger          *slog.Logger
}

type Store interface {
	GetDeliveryJob(context.Context, uuid.UUID) (delivery.Job, error)
	GetSubscriptionByID(context.Context, uuid.UUID) (subscription.Subscription, error)
	GetConnectedApp(context.Context, uuid.UUID, uuid.UUID) (connectedapp.App, error)
	GetFunctionDestination(context.Context, uuid.UUID, uuid.UUID) (functiondestination.Destination, error)
	GetConnectorInstance(context.Context, uuid.UUID, uuid.UUID) (connectorinstance.Instance, error)
	GetEvent(context.Context, uuid.UUID) (stream.Event, error)
	SetDeliveryJobAttempt(context.Context, uuid.UUID, int) error
	MarkDeliveryJobSucceeded(context.Context, uuid.UUID, time.Time, *string, *int) error
	MarkDeliveryJobRetryableFailure(context.Context, uuid.UUID, string, *int) error
	MarkDeliveryJobDeadLetter(context.Context, uuid.UUID, string, *int) error
	MarkDeliveryJobFailed(context.Context, uuid.UUID, string, *int) error
}

type Activities struct {
	store      Store
	httpClient *http.Client
	connectors map[string]outbound.Connector
	metrics    Metrics
	logger     *slog.Logger
}

type Metrics interface {
	IncDeliverySucceeded()
	IncDeliveryFailed()
	IncDeliveryDeadLetter()
	IncFunctionInvocations()
	IncFunctionInvocationFailures()
	IncConnectorDeliveries(string, string)
	IncConnectorDeliveryFailures(string, string)
	IncConnectorDeliveryDeadLetter(string, string)
}

type DeliveryJob struct {
	ID             string
	TenantID       string
	SubscriptionID string
	EventID        string
}

type Subscription struct {
	ID                    string
	DestinationType       string
	ConnectedAppID        string
	FunctionDestinationID string
	ConnectorInstanceID   string
	Operation             string
	OperationParams       json.RawMessage
}

type ConnectedApp struct {
	ID             string
	DestinationURL string
}

type FunctionDestination struct {
	ID             string
	URL            string
	Secret         string
	TimeoutSeconds int
}

type ConnectorInstance struct {
	ID            string
	ConnectorName string
	Config        json.RawMessage
}

type ConnectorResult struct {
	ExternalID string
	StatusCode int
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
	connectors := map[string]outbound.Connector{
		slackconnector.ConnectorName: slackconnector.New(deps.SlackAPIBaseURL, client),
	}
	return &Activities{
		store:      deps.Store,
		httpClient: client,
		connectors: connectors,
		metrics:    deps.Metrics,
		logger:     deps.Logger,
	}
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
	return DeliveryJob{
		ID:             job.ID.String(),
		TenantID:       job.TenantID.String(),
		SubscriptionID: job.SubscriptionID.String(),
		EventID:        job.EventID.String(),
	}, nil
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
	return Subscription{
		ID:                    sub.ID.String(),
		DestinationType:       sub.DestinationType,
		ConnectedAppID:        optionalUUIDString(sub.ConnectedAppID),
		FunctionDestinationID: optionalUUIDString(sub.FunctionDestinationID),
		ConnectorInstanceID:   optionalUUIDString(sub.ConnectorInstanceID),
		Operation:             optionalString(sub.Operation),
		OperationParams:       sub.OperationParams,
	}, nil
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

func (a *Activities) LoadFunctionDestination(ctx context.Context, functionDestinationID string, tenantID string) (FunctionDestination, error) {
	id, err := uuid.Parse(functionDestinationID)
	if err != nil {
		return FunctionDestination{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return FunctionDestination{}, err
	}
	destination, err := a.store.GetFunctionDestination(ctx, tID, id)
	if err != nil {
		return FunctionDestination{}, err
	}
	return FunctionDestination{
		ID:             destination.ID.String(),
		URL:            destination.URL,
		Secret:         destination.Secret,
		TimeoutSeconds: destination.TimeoutSeconds,
	}, nil
}

func (a *Activities) LoadConnectorInstance(ctx context.Context, connectorInstanceID string, tenantID string) (ConnectorInstance, error) {
	id, err := uuid.Parse(connectorInstanceID)
	if err != nil {
		return ConnectorInstance{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return ConnectorInstance{}, err
	}
	instance, err := a.store.GetConnectorInstance(ctx, tID, id)
	if err != nil {
		return ConnectorInstance{}, err
	}
	return ConnectorInstance{
		ID:            instance.ID.String(),
		ConnectorName: instance.ConnectorName,
		Config:        instance.Config,
	}, nil
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

func (a *Activities) InvokeFunction(ctx context.Context, deliveryJobID string, functionDestinationID string, event Event, destinationURL string, secret string, timeoutSeconds int, attempt int) error {
	if a.logger != nil {
		a.logger.Info("function_invocation_started",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("function_destination_id", functionDestinationID),
			slog.String("event_id", event.EventID),
			slog.String("tenant_id", event.TenantID),
			slog.Int("attempt", attempt),
		)
	}
	if a.metrics != nil {
		a.metrics.IncFunctionInvocations()
	}

	body, err := json.Marshal(event)
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return fmt.Errorf("marshal event: %w", err)
	}

	invokeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(invokeCtx, http.MethodPost, destinationURL, bytes.NewReader(body))
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Groot-Event-Id", event.EventID)
	req.Header.Set("X-Groot-Tenant-Id", event.TenantID)
	req.Header.Set("X-Groot-Signature", computeSignature(secret, body))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		if a.logger != nil {
			a.logger.Info("function_invocation_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("function_destination_id", functionDestinationID),
				slog.String("event_id", event.EventID),
				slog.String("tenant_id", event.TenantID),
				slog.Int("attempt", attempt),
			)
		}
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return fmt.Errorf("perform request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if a.logger != nil {
			a.logger.Info("function_invocation_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("function_destination_id", functionDestinationID),
				slog.String("event_id", event.EventID),
				slog.String("tenant_id", event.TenantID),
				slog.Int("attempt", attempt),
			)
		}
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if a.logger != nil {
		a.logger.Info("function_invocation_succeeded",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("function_destination_id", functionDestinationID),
			slog.String("event_id", event.EventID),
			slog.String("tenant_id", event.TenantID),
			slog.Int("attempt", attempt),
		)
	}
	return nil
}

func (a *Activities) ExecuteConnector(ctx context.Context, deliveryJobID string, tenantID string, event Event, connectorInstance ConnectorInstance, operation string, operationParams json.RawMessage, attempt int) (ConnectorResult, error) {
	connector, ok := a.connectors[connectorInstance.ConnectorName]
	if !ok {
		return ConnectorResult{}, outbound.PermanentError{Err: fmt.Errorf("unsupported connector: %s", connectorInstance.ConnectorName)}
	}
	if a.logger != nil {
		a.logger.Info("connector_delivery_started",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorInstance.ConnectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", event.EventID),
			slog.Int("attempt", attempt),
		)
	}
	if a.metrics != nil {
		a.metrics.IncConnectorDeliveries(connectorInstance.ConnectorName, operation)
	}

	eventID, err := uuid.Parse(event.EventID)
	if err != nil {
		return ConnectorResult{}, outbound.PermanentError{Err: err}
	}
	parsedTenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return ConnectorResult{}, outbound.PermanentError{Err: err}
	}

	result, err := connector.Execute(ctx, operation, connectorInstance.Config, renderOperationParams(operationParams, event), stream.Event{
		EventID:   eventID,
		TenantID:  parsedTenantID,
		Type:      event.Type,
		Source:    event.Source,
		Timestamp: event.Timestamp,
		Payload:   event.Payload,
	})
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncConnectorDeliveryFailures(connectorInstance.ConnectorName, operation)
		}
		if a.logger != nil {
			a.logger.Info("connector_delivery_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("connector_name", connectorInstance.ConnectorName),
				slog.String("operation", operation),
				slog.String("tenant_id", tenantID),
				slog.String("event_id", event.EventID),
				slog.Int("attempt", attempt),
			)
		}
		return ConnectorResult{}, err
	}

	if a.logger != nil {
		a.logger.Info("connector_delivery_succeeded",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorInstance.ConnectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", event.EventID),
			slog.Int("attempt", attempt),
		)
	}
	return ConnectorResult{ExternalID: result.ExternalID, StatusCode: result.StatusCode}, nil
}

func (a *Activities) MarkSucceeded(ctx context.Context, deliveryJobID string, externalID *string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobSucceeded(ctx, id, time.Now().UTC(), externalID, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliverySucceeded()
	}
	return nil
}

func (a *Activities) MarkRetryableFailure(ctx context.Context, deliveryJobID string, lastError string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobRetryableFailure(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryFailed()
	}
	return nil
}

func (a *Activities) MarkDeadLetter(ctx context.Context, deliveryJobID string, lastError string, connectorName string, operation string, tenantID string, eventID string, attempt int, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobDeadLetter(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryDeadLetter()
		if connectorName != "" && operation != "" {
			a.metrics.IncConnectorDeliveryDeadLetter(connectorName, operation)
		}
	}
	if connectorName != "" && a.logger != nil {
		a.logger.Info("connector_delivery_dead_letter",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", eventID),
			slog.Int("attempt", attempt),
		)
	}
	return nil
}

func (a *Activities) MarkFailed(ctx context.Context, deliveryJobID string, lastError string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobFailed(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryFailed()
	}
	return nil
}

func computeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func optionalUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func renderOperationParams(raw json.RawMessage, event Event) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	replacements := map[string]string{
		"{{event_id}}":  event.EventID,
		"{{tenant_id}}": event.TenantID,
		"{{type}}":      event.Type,
		"{{source}}":    event.Source,
		"{{timestamp}}": event.Timestamp.UTC().Format(time.RFC3339),
	}
	rendered := string(raw)
	for token, value := range replacements {
		rendered = strings.ReplaceAll(rendered, token, value)
	}
	return json.RawMessage(rendered)
}

func ConnectorStatusCode(err error) *int {
	var retryable outbound.RetryableError
	if errors.As(err, &retryable) {
		if retryable.StatusCode == 0 {
			return nil
		}
		statusCode := retryable.StatusCode
		return &statusCode
	}
	var permanent outbound.PermanentError
	if errors.As(err, &permanent) {
		if permanent.StatusCode == 0 {
			return nil
		}
		statusCode := permanent.StatusCode
		return &statusCode
	}
	return nil
}
