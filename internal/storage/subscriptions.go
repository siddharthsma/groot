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
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $25, $26, $27)
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectionID, record.AgentID, record.AgentVersionID, nullableString(optionalStringValue(record.SessionKeyTemplate)), record.SessionCreateIfMissing, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, record.CreatedAt, record.WorkflowID, record.WorkflowVersionID, nullableString(record.WorkflowNodeID), record.ManagedByWorkflow, nullableString(record.WorkflowArtifactStatus), actor.Type, actor.ID, actor.Email),
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
		    agent_version_id = $8,
		    session_key_template = $9,
		    session_create_if_missing = $10,
		    operation = $11,
		    operation_params = $12,
		    filter_json = $13,
		    event_type = $14,
		    event_source = $15,
		    emit_success_event = $16,
		    emit_failure_event = $17,
		    status = $18,
		    workflow_id = $19,
		    workflow_version_id = $20,
		    workflow_node_id = $21,
		    managed_by_workflow = $22,
		    workflow_artifact_status = $23,
		    updated_by_actor_type = $24,
		    updated_by_actor_id = $25,
		    updated_by_actor_email = $26
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectionID, record.AgentID, record.AgentVersionID, nullableString(optionalStringValue(record.SessionKeyTemplate)), record.SessionCreateIfMissing, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, record.WorkflowID, record.WorkflowVersionID, nullableString(record.WorkflowNodeID), record.ManagedByWorkflow, nullableString(record.WorkflowArtifactStatus), actor.Type, actor.ID, actor.Email), &sub)
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
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
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
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
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
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
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
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
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
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
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
	var agentVersionID sql.NullString
	var sessionKeyTemplate sql.NullString
	var operation sql.NullString
	var operationParams []byte
	var filter []byte
	var workflowID sql.NullString
	var workflowVersionID sql.NullString
	var workflowNodeID sql.NullString
	var workflowArtifactStatus sql.NullString
	if err := row.Scan(
		&sub.ID,
		&sub.TenantID,
		&connectedAppID,
		&sub.DestinationType,
		&functionDestinationID,
		&connectorInstanceID,
		&agentID,
		&agentVersionID,
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
		&workflowID,
		&workflowVersionID,
		&workflowNodeID,
		&sub.ManagedByWorkflow,
		&workflowArtifactStatus,
	); err != nil {
		return err
	}

	sub.ConnectedAppID = parseOptionalUUID(connectedAppID)
	sub.FunctionDestinationID = parseOptionalUUID(functionDestinationID)
	sub.ConnectionID = parseOptionalUUID(connectorInstanceID)
	sub.AgentID = parseOptionalUUID(agentID)
	sub.AgentVersionID = parseOptionalUUID(agentVersionID)
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
	sub.WorkflowID = parseOptionalUUID(workflowID)
	sub.WorkflowVersionID = parseOptionalUUID(workflowVersionID)
	sub.WorkflowNodeID = nullableStringValue(workflowNodeID)
	sub.WorkflowArtifactStatus = nullableStringValue(workflowArtifactStatus)
	return nil
}
