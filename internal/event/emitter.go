package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/schema"
)

const (
	ResultStatusSucceeded = "succeeded"
	ResultStatusFailed    = "failed"
)

type Publisher interface {
	PublishEvent(context.Context, Event) error
}

type Store interface {
	SaveResultEvent(context.Context, uuid.UUID, Event) (bool, error)
}

type Metrics interface {
	IncResultEventsEmitted(string, string, string)
	IncResultEventEmitFailures()
}

type Emitter struct {
	publisher     Publisher
	store         Store
	logger        *slog.Logger
	metrics       Metrics
	maxChainDepth int
	now           func() time.Time
	schemas       SchemaResolver
}

type EmitRequest struct {
	SubscriptionID      uuid.UUID
	DeliveryJobID       uuid.UUID
	ExistingResultEvent *uuid.UUID
	InputEvent          Event
	Integration         string
	Operation           string
	Status              string
	Output              map[string]any
	ToolCalls           []map[string]any
	Error               *ResultError
	ExternalID          *string
	HTTPStatus          *int
	AgentID             *uuid.UUID
	AgentSessionID      *uuid.UUID
	SessionKey          string
}

type ResultError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type SchemaResolver interface {
	GetLatest(context.Context, string) (schema.Schema, error)
	ValidateEvent(context.Context, string, string, string, json.RawMessage) (schema.Schema, error)
}

type Option func(*Emitter)

func WithSchemaResolver(resolver SchemaResolver) Option {
	return func(e *Emitter) {
		e.schemas = resolver
	}
}

func NewEmitter(publisher Publisher, store Store, logger *slog.Logger, metrics Metrics, maxChainDepth int, options ...Option) *Emitter {
	emitter := &Emitter{
		publisher:     publisher,
		store:         store,
		logger:        logger,
		metrics:       metrics,
		maxChainDepth: maxChainDepth,
		now:           func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(emitter)
	}
	return emitter
}

func (e *Emitter) EmitResultEvent(ctx context.Context, req EmitRequest) error {
	if req.ExistingResultEvent != nil {
		return nil
	}
	if req.InputEvent.ChainDepth >= e.maxChainDepth {
		if e.logger != nil {
			e.logger.Info("chain_depth_exceeded",
				slog.String("tenant_id", req.InputEvent.TenantID.String()),
				slog.String("event_id", req.InputEvent.EventID.String()),
				slog.Int("chain_depth", req.InputEvent.ChainDepth),
				slog.Int("max_chain_depth", e.maxChainDepth),
			)
		}
		return nil
	}

	payload, err := buildPayload(req)
	if err != nil {
		e.fail(req.Integration, req.Operation, fmt.Errorf("build result event payload: %w", err))
		return nil
	}

	eventType := resultEventType(req.Integration, req.Operation, req.Status)
	fullName := schema.FullName(eventType, 1)
	schemaVersion := 1
	if e.schemas != nil {
		latest, latestErr := e.schemas.GetLatest(ctx, eventType)
		switch {
		case latestErr == nil:
			fullName = latest.FullName
			schemaVersion = latest.Version
		case latestErr != nil && !errors.Is(latestErr, sql.ErrNoRows):
			e.fail(req.Integration, req.Operation, fmt.Errorf("get latest schema: %w", latestErr))
			return nil
		}
		resolvedSchema, validationErr := e.schemas.ValidateEvent(ctx, fullName, req.Integration, SourceKindInternal, payload)
		if validationErr != nil {
			e.fail(req.Integration, req.Operation, fmt.Errorf("validate result event: %w", validationErr))
			return nil
		}
		if resolvedSchema.FullName != "" {
			fullName = resolvedSchema.FullName
			schemaVersion = resolvedSchema.Version
		}
	}

	evt := Event{
		EventID:        uuid.New(),
		TenantID:       req.InputEvent.TenantID,
		Type:           fullName,
		Source:         NormalizeSource(Source{Kind: SourceKindInternal, Integration: req.Integration}, SourceKindInternal),
		SourceKind:     SourceKindInternal,
		Lineage:        inheritedLineage(req.InputEvent),
		ChainDepth:     req.InputEvent.ChainDepth + 1,
		Timestamp:      e.now(),
		Payload:        payload,
		SchemaFullName: fullName,
		SchemaVersion:  schemaVersion,
	}

	if e.logger != nil {
		e.logger.Info("result_event_emit_started",
			slog.String("delivery_job_id", req.DeliveryJobID.String()),
			slog.String("tenant_id", evt.TenantID.String()),
			slog.String("event_type", evt.Type),
		)
	}

	if err := e.publisher.PublishEvent(ctx, evt); err != nil {
		e.fail(req.Integration, req.Operation, fmt.Errorf("publish result event: %w", err))
		return nil
	}
	linked, err := e.store.SaveResultEvent(ctx, req.DeliveryJobID, evt)
	if err != nil {
		e.fail(req.Integration, req.Operation, fmt.Errorf("store result event: %w", err))
		return nil
	}
	if !linked {
		return nil
	}

	if e.metrics != nil {
		e.metrics.IncResultEventsEmitted(req.Integration, req.Operation, req.Status)
	}
	if e.logger != nil {
		e.logger.Info("result_event_emit_succeeded",
			slog.String("delivery_job_id", req.DeliveryJobID.String()),
			slog.String("tenant_id", evt.TenantID.String()),
			slog.String("result_event_id", evt.EventID.String()),
			slog.String("event_type", evt.Type),
		)
	}
	return nil
}

func inheritedLineage(input Event) *Lineage {
	if input.Lineage != nil {
		return NormalizeLineage(input.Lineage)
	}
	if input.Source.Kind != SourceKindExternal {
		return nil
	}
	return NormalizeLineage(&Lineage{
		Integration:       input.Source.Integration,
		ConnectionID:      input.Source.ConnectionID,
		ConnectionName:    input.Source.ConnectionName,
		ExternalAccountID: input.Source.ExternalAccountID,
	})
}

func (e *Emitter) fail(connector, operation string, err error) {
	if e.metrics != nil {
		e.metrics.IncResultEventEmitFailures()
	}
	if e.logger != nil {
		e.logger.Error("result_event_emit_failed",
			slog.String("integration_name", connector),
			slog.String("operation", operation),
			slog.String("error", err.Error()),
		)
	}
}

func buildPayload(req EmitRequest) (json.RawMessage, error) {
	payload := map[string]any{
		"input_event_id":   req.InputEvent.EventID.String(),
		"subscription_id":  req.SubscriptionID.String(),
		"delivery_job_id":  req.DeliveryJobID.String(),
		"integration_name": req.Integration,
		"operation":        req.Operation,
		"status":           req.Status,
		"external_id":      req.ExternalID,
		"http_status_code": req.HTTPStatus,
		"output":           emptyOutput(req.Output),
	}
	if req.Integration == "llm" && req.Operation == "agent" {
		payload["tool_calls"] = emptyToolCalls(req.ToolCalls)
		if req.AgentID != nil {
			payload["agent_id"] = req.AgentID.String()
		} else {
			payload["agent_id"] = nil
		}
		if req.AgentSessionID != nil {
			payload["agent_session_id"] = req.AgentSessionID.String()
		} else {
			payload["agent_session_id"] = nil
		}
		if strings.TrimSpace(req.SessionKey) != "" {
			payload["session_key"] = strings.TrimSpace(req.SessionKey)
		} else {
			payload["session_key"] = nil
		}
	}
	if req.Error != nil {
		payload["error"] = req.Error
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func emptyOutput(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func emptyToolCalls(value []map[string]any) []map[string]any {
	if value == nil {
		return []map[string]any{}
	}
	return value
}

func resultEventType(connector, operation, status string) string {
	suffix := "completed"
	if status != ResultStatusSucceeded {
		suffix = "failed"
	}
	return fmt.Sprintf("%s.%s.%s", connector, operation, suffix)
}
