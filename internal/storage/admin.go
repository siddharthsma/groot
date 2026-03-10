package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"groot/internal/connection"
	"groot/internal/delivery"
	"groot/internal/event"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

func (d *DB) ListConnectionsAdmin(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]connection.Instance, error) {
	query := `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE 1=1
	`
	args := []any{}
	nextArg := 1
	if tenantID != nil {
		query += fmt.Sprintf(" AND tenant_id = $%d", nextArg)
		args = append(args, *tenantID)
		nextArg++
	}
	if strings.TrimSpace(connectorName) != "" {
		query += fmt.Sprintf(" AND connector_name = $%d", nextArg)
		args = append(args, strings.TrimSpace(connectorName))
		nextArg++
	}
	if strings.TrimSpace(scope) != "" {
		query += fmt.Sprintf(" AND scope = $%d", nextArg)
		args = append(args, strings.TrimSpace(scope))
		nextArg++
	}
	query += " ORDER BY created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var instances []connection.Instance
	for rows.Next() {
		var instance connection.Instance
		if err := scanConnection(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan admin connection: %w", err)
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (d *DB) ListSubscriptionsAdmin(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]subscription.Subscription, error) {
	query := `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status
		FROM subscriptions
		WHERE 1=1
	`
	args := []any{}
	nextArg := 1
	if tenantID != nil {
		query += fmt.Sprintf(" AND tenant_id = $%d", nextArg)
		args = append(args, *tenantID)
		nextArg++
	}
	if strings.TrimSpace(eventType) != "" {
		query += fmt.Sprintf(" AND event_type = $%d", nextArg)
		args = append(args, strings.TrimSpace(eventType))
		nextArg++
	}
	if strings.TrimSpace(destinationType) != "" {
		query += fmt.Sprintf(" AND destination_type = $%d", nextArg)
		args = append(args, strings.TrimSpace(destinationType))
		nextArg++
	}
	query += " ORDER BY created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan admin subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (d *DB) ListEventsAdmin(ctx context.Context, tenantID tenant.ID, eventType string, from, to *time.Time, limit int) ([]event.Event, error) {
	query := `
		SELECT event_id, tenant_id, workflow_run_id, workflow_node_id, type, source, source_kind, source_connection_id, source_connection_name, source_external_account_id,
		       lineage_integration, lineage_connection_id, lineage_connection_name, lineage_external_account_id,
		       chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	nextArg := 2
	if eventType != "" {
		query += fmt.Sprintf(" AND type = $%d", nextArg)
		args = append(args, eventType)
		nextArg++
	}
	if from != nil {
		query += fmt.Sprintf(" AND timestamp >= $%d", nextArg)
		args = append(args, *from)
		nextArg++
	}
	if to != nil {
		query += fmt.Sprintf(" AND timestamp <= $%d", nextArg)
		args = append(args, *to)
		nextArg++
	}
	query += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", nextArg)
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var events []event.Event
	for rows.Next() {
		var event event.Event
		if err := scanEvent(rows, &event); err != nil {
			return nil, fmt.Errorf("scan admin event: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (d *DB) ListDeliveryJobsAdmin(ctx context.Context, tenantID tenant.ID, status string, from, to *time.Time, limit int) ([]delivery.Job, error) {
	query := `
		SELECT id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	nextArg := 2
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", nextArg)
		args = append(args, status)
		nextArg++
	}
	if from != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", nextArg)
		args = append(args, *from)
		nextArg++
	}
	if to != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", nextArg)
		args = append(args, *to)
		nextArg++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", nextArg)
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan admin delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
