package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"groot/internal/subscription"
	"groot/internal/tenant"
)

func (d *DB) CreateSubscription(ctx context.Context, record subscription.Record) (subscription.Subscription, error) {
	const query = `
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $19, $20, $21)
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectorInstanceID, record.AgentID, nullableString(optionalStringValue(record.SessionKeyTemplate)), record.SessionCreateIfMissing, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, record.CreatedAt, actor.Type, actor.ID, actor.Email),
		&sub,
	)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) UpdateSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, record subscription.Record) (subscription.Subscription, error) {
	const query = `
		UPDATE subscriptions
		SET connected_app_id = $3,
		    destination_type = $4,
		    function_destination_id = $5,
		    connector_instance_id = $6,
		    agent_id = $7,
		    session_key_template = $8,
		    session_create_if_missing = $9,
		    operation = $10,
		    operation_params = $11,
		    filter_json = $12,
		    event_type = $13,
		    event_source = $14,
		    emit_success_event = $15,
		    emit_failure_event = $16,
		    status = $17,
		    updated_by_actor_type = $18,
		    updated_by_actor_id = $19,
		    updated_by_actor_email = $20
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectorInstanceID, record.AgentID, nullableString(optionalStringValue(record.SessionKeyTemplate)), record.SessionCreateIfMissing, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, actor.Type, actor.ID, actor.Email), &sub)
	if err != nil {
		if err == sql.ErrNoRows {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) GetSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE id = $1 AND tenant_id = $2
	`
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID), &sub)
	if err != nil {
		if err == sql.ErrNoRows {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		  AND event_type = $2
		  AND status = 'active'
		  AND (event_source IS NULL OR event_source = $3)
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventType, eventSource)
	if err != nil {
		return nil, fmt.Errorf("query matching subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan matching subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matching subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) SetSubscriptionStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (subscription.Subscription, error) {
	const query = `
		UPDATE subscriptions
		SET status = $3,
		    updated_by_actor_type = $4,
		    updated_by_actor_id = $5,
		    updated_by_actor_email = $6
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, status, actor.Type, actor.ID, actor.Email), &sub)
	if err != nil {
		if err == sql.ErrNoRows {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("set subscription status: %w", err)
	}
	return sub, nil
}

func (d *DB) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE id = $1
	`
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID), &sub)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("get subscription by id: %w", err)
	}
	return sub, nil
}

func scanSubscription(row scanner, sub *subscription.Subscription) error {
	var connectedAppID sql.NullString
	var functionDestinationID sql.NullString
	var connectorInstanceID sql.NullString
	var agentID sql.NullString
	var sessionKeyTemplate sql.NullString
	var operation sql.NullString
	var operationParams []byte
	var filter []byte
	if err := row.Scan(
		&sub.ID,
		&sub.TenantID,
		&connectedAppID,
		&sub.DestinationType,
		&functionDestinationID,
		&connectorInstanceID,
		&agentID,
		&sessionKeyTemplate,
		&sub.SessionCreateIfMissing,
		&operation,
		&operationParams,
		&filter,
		&sub.EventType,
		&sub.EventSource,
		&sub.EmitSuccessEvent,
		&sub.EmitFailureEvent,
		&sub.Status,
		&sub.CreatedAt,
	); err != nil {
		return err
	}

	sub.ConnectedAppID = parseOptionalUUID(connectedAppID)
	sub.FunctionDestinationID = parseOptionalUUID(functionDestinationID)
	sub.ConnectorInstanceID = parseOptionalUUID(connectorInstanceID)
	sub.AgentID = parseOptionalUUID(agentID)
	if sessionKeyTemplate.Valid {
		value := sessionKeyTemplate.String
		sub.SessionKeyTemplate = &value
	}
	if operation.Valid {
		value := operation.String
		sub.Operation = &value
	}
	sub.OperationParams = json.RawMessage(operationParams)
	sub.Filter = json.RawMessage(filter)
	return nil
}
