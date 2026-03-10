package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"groot/internal/subscription"
	"groot/internal/tenant"
	"groot/internal/workflow"
)

func (d *DB) ListWorkflowEntryBindings(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, versionID *uuid.UUID) ([]workflow.EntryBinding, error) {
	query := `
		SELECT b.id, b.workflow_id, b.workflow_version_id, b.workflow_node_id, b.integration, b.event_type, b.connection_id, b.filter_json, b.status, b.created_at, b.superseded_at
		FROM workflow_entry_bindings b
		INNER JOIN workflows w ON w.id = b.workflow_id
		WHERE b.workflow_id = $1 AND w.tenant_id = $2
	`
	args := []any{workflowID, tenantID}
	if versionID != nil {
		query += " AND b.workflow_version_id = $3"
		args = append(args, *versionID)
	}
	query += " ORDER BY b.created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workflow entry bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]workflow.EntryBinding, 0)
	for rows.Next() {
		var current workflow.EntryBinding
		if err := scanWorkflowEntryBinding(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow entry binding: %w", err)
		}
		out = append(out, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow entry bindings: %w", err)
	}
	return out, nil
}

func (d *DB) ListWorkflowSubscriptionArtifacts(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, versionID *uuid.UUID) ([]workflow.SubscriptionArtifact, error) {
	query := `
		SELECT id, workflow_id, workflow_version_id, workflow_node_id, destination_type, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, workflow_artifact_status, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1 AND managed_by_workflow = TRUE AND workflow_id = $2
	`
	args := []any{tenantID, workflowID}
	if versionID != nil {
		query += " AND workflow_version_id = $3"
		args = append(args, *versionID)
	}
	query += " ORDER BY created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workflow subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]workflow.SubscriptionArtifact, 0)
	for rows.Next() {
		var current workflow.SubscriptionArtifact
		if err := scanWorkflowSubscriptionArtifact(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow subscription: %w", err)
		}
		out = append(out, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow subscriptions: %w", err)
	}
	return out, nil
}

func (d *DB) ApplyWorkflowPublish(ctx context.Context, tenantID tenant.ID, wf workflow.Workflow, version workflow.Version, entryBindings []workflow.EntryBinding, subscriptionsToCreate []subscription.Record) (workflow.Workflow, workflow.Version, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Workflow{}, workflow.Version{}, fmt.Errorf("begin workflow publish tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := acquireWorkflowPublishLock(ctx, tx, wf.ID); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if err := lockWorkflow(ctx, tx, tenantID, wf.ID); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}

	now := time.Now().UTC()
	if err := supersedeWorkflowArtifacts(ctx, tx, wf.ID, now); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if err := insertWorkflowEntryBindings(ctx, tx, entryBindings); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if err := insertWorkflowSubscriptions(ctx, tx, subscriptionsToCreate); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if wf.PublishedVersionID != nil {
		if err := archiveWorkflowVersion(ctx, tx, *wf.PublishedVersionID, now); err != nil {
			return workflow.Workflow{}, workflow.Version{}, err
		}
	}
	if err := publishWorkflowVersion(ctx, tx, version.ID, now); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if err := activateWorkflow(ctx, tx, tenantID, wf.ID, version.ID, now); err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}

	updatedWorkflow, err := getWorkflowTx(ctx, tx, tenantID, wf.ID)
	if err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	updatedVersion, err := getWorkflowVersionTx(ctx, tx, tenantID, version.ID)
	if err != nil {
		return workflow.Workflow{}, workflow.Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return workflow.Workflow{}, workflow.Version{}, fmt.Errorf("commit workflow publish tx: %w", err)
	}
	return updatedWorkflow, updatedVersion, nil
}

func (d *DB) ApplyWorkflowUnpublish(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) (workflow.Workflow, *workflow.Version, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Workflow{}, nil, fmt.Errorf("begin workflow unpublish tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := acquireWorkflowPublishLock(ctx, tx, workflowID); err != nil {
		return workflow.Workflow{}, nil, err
	}
	currentWorkflow, err := getWorkflowTx(ctx, tx, tenantID, workflowID)
	if err != nil {
		return workflow.Workflow{}, nil, err
	}

	now := time.Now().UTC()
	if err := deactivateWorkflowArtifacts(ctx, tx, workflowID, now); err != nil {
		return workflow.Workflow{}, nil, err
	}

	var archived *workflow.Version
	if currentWorkflow.PublishedVersionID != nil {
		if err := archiveWorkflowVersion(ctx, tx, *currentWorkflow.PublishedVersionID, now); err != nil {
			return workflow.Workflow{}, nil, err
		}
		record, err := getWorkflowVersionTx(ctx, tx, tenantID, *currentWorkflow.PublishedVersionID)
		if err != nil {
			return workflow.Workflow{}, nil, err
		}
		archived = &record
	}
	if err := clearWorkflowPublication(ctx, tx, tenantID, workflowID, now); err != nil {
		return workflow.Workflow{}, nil, err
	}
	updatedWorkflow, err := getWorkflowTx(ctx, tx, tenantID, workflowID)
	if err != nil {
		return workflow.Workflow{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return workflow.Workflow{}, nil, fmt.Errorf("commit workflow unpublish tx: %w", err)
	}
	return updatedWorkflow, archived, nil
}

func acquireWorkflowPublishLock(ctx context.Context, tx *sql.Tx, workflowID uuid.UUID) error {
	const query = `SELECT pg_advisory_xact_lock(hashtext($1))`
	if _, err := tx.ExecContext(ctx, query, workflowID.String()); err != nil {
		return fmt.Errorf("acquire workflow publish lock: %w", err)
	}
	return nil
}

func supersedeWorkflowArtifacts(ctx context.Context, tx *sql.Tx, workflowID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_entry_bindings
		SET status = 'superseded',
		    superseded_at = $2
		WHERE workflow_id = $1
		  AND status = 'active'
	`, workflowID, now); err != nil {
		return fmt.Errorf("supersede workflow entry bindings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE subscriptions
		SET status = 'paused',
		    workflow_artifact_status = 'superseded'
		WHERE workflow_id = $1
		  AND managed_by_workflow = TRUE
		  AND workflow_artifact_status = 'active'
	`, workflowID); err != nil {
		return fmt.Errorf("supersede workflow subscriptions: %w", err)
	}
	return nil
}

func deactivateWorkflowArtifacts(ctx context.Context, tx *sql.Tx, workflowID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_entry_bindings
		SET status = 'inactive',
		    superseded_at = COALESCE(superseded_at, $2)
		WHERE workflow_id = $1
		  AND status = 'active'
	`, workflowID, now); err != nil {
		return fmt.Errorf("deactivate workflow entry bindings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE subscriptions
		SET status = 'paused',
		    workflow_artifact_status = 'inactive'
		WHERE workflow_id = $1
		  AND managed_by_workflow = TRUE
		  AND workflow_artifact_status = 'active'
	`, workflowID); err != nil {
		return fmt.Errorf("deactivate workflow subscriptions: %w", err)
	}
	return nil
}

func insertWorkflowEntryBindings(ctx context.Context, tx *sql.Tx, bindings []workflow.EntryBinding) error {
	const query = `
		INSERT INTO workflow_entry_bindings (id, workflow_id, workflow_version_id, workflow_node_id, integration, event_type, connection_id, filter_json, status, created_at, superseded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	for _, binding := range bindings {
		if _, err := tx.ExecContext(ctx, query, binding.ID, binding.WorkflowID, binding.WorkflowVersionID, binding.WorkflowNodeID, binding.Integration, binding.EventType, binding.ConnectionID, jsonBytes(binding.FilterJSON), binding.Status, binding.CreatedAt, binding.SupersededAt); err != nil {
			return fmt.Errorf("insert workflow entry binding: %w", err)
		}
	}
	return nil
}

func insertWorkflowSubscriptions(ctx context.Context, tx *sql.Tx, records []subscription.Record) error {
	const query = `
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, workflow_id, workflow_version_id, workflow_node_id, managed_by_workflow, workflow_artifact_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
	`
	for _, record := range records {
		if _, err := tx.ExecContext(ctx, query,
			record.ID,
			record.TenantID,
			record.ConnectedAppID,
			record.DestinationType,
			record.FunctionDestinationID,
			record.ConnectionID,
			record.AgentID,
			record.AgentVersionID,
			nullableString(optionalStringValue(record.SessionKeyTemplate)),
			record.SessionCreateIfMissing,
			record.Operation,
			[]byte(record.OperationParams),
			jsonBytes(record.Filter),
			record.EventType,
			record.EventSource,
			record.EmitSuccessEvent,
			record.EmitFailureEvent,
			record.Status,
			record.CreatedAt,
			record.WorkflowID,
			record.WorkflowVersionID,
			nullableString(record.WorkflowNodeID),
			record.ManagedByWorkflow,
			nullableString(record.WorkflowArtifactStatus),
		); err != nil {
			return fmt.Errorf("insert workflow subscription: %w", err)
		}
	}
	return nil
}

func archiveWorkflowVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_versions
		SET status = 'archived',
		    superseded_at = $2
		WHERE id = $1
	`, versionID, now); err != nil {
		return fmt.Errorf("archive workflow version: %w", err)
	}
	return nil
}

func publishWorkflowVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_versions
		SET status = 'published',
		    published_at = $2,
		    superseded_at = NULL
		WHERE id = $1
	`, versionID, now); err != nil {
		return fmt.Errorf("publish workflow version: %w", err)
	}
	return nil
}

func activateWorkflow(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, workflowID, versionID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflows
		SET status = 'published',
		    published_version_id = $3,
		    published_at = $4,
		    last_publish_error = NULL,
		    updated_at = $4
		WHERE id = $1 AND tenant_id = $2
	`, workflowID, tenantID, versionID, now); err != nil {
		return fmt.Errorf("activate workflow: %w", err)
	}
	return nil
}

func clearWorkflowPublication(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, workflowID uuid.UUID, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflows
		SET status = 'draft',
		    published_version_id = NULL,
		    published_at = NULL,
		    updated_at = $3
		WHERE id = $1 AND tenant_id = $2
	`, workflowID, tenantID, now); err != nil {
		return fmt.Errorf("clear workflow publication: %w", err)
	}
	return nil
}

func getWorkflowTx(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, workflowID uuid.UUID) (workflow.Workflow, error) {
	const query = `
		SELECT id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at
		FROM workflows
		WHERE id = $1 AND tenant_id = $2
	`
	var current workflow.Workflow
	if err := scanWorkflow(tx.QueryRowContext(ctx, query, workflowID, tenantID), &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Workflow{}, workflow.ErrWorkflowNotFound
		}
		return workflow.Workflow{}, fmt.Errorf("get workflow in tx: %w", err)
	}
	return current, nil
}

func getWorkflowVersionTx(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, versionID uuid.UUID) (workflow.Version, error) {
	const query = `
		SELECT v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
		FROM workflow_versions v
		INNER JOIN workflows w ON w.id = v.workflow_id
		WHERE v.id = $1 AND w.tenant_id = $2
	`
	var current workflow.Version
	if err := scanWorkflowVersion(tx.QueryRowContext(ctx, query, versionID, tenantID), &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("get workflow version in tx: %w", err)
	}
	return current, nil
}

func scanWorkflowEntryBinding(row scanner, current *workflow.EntryBinding) error {
	var connectionID sql.NullString
	var filter []byte
	var supersededAt sql.NullTime
	if err := row.Scan(
		&current.ID,
		&current.WorkflowID,
		&current.WorkflowVersionID,
		&current.WorkflowNodeID,
		&current.Integration,
		&current.EventType,
		&connectionID,
		&filter,
		&current.Status,
		&current.CreatedAt,
		&supersededAt,
	); err != nil {
		return err
	}
	current.ConnectionID = parseOptionalUUID(connectionID)
	current.FilterJSON = json.RawMessage(filter)
	if supersededAt.Valid {
		value := supersededAt.Time
		current.SupersededAt = &value
	}
	return nil
}

func scanWorkflowSubscriptionArtifact(row scanner, current *workflow.SubscriptionArtifact) error {
	var connectionID sql.NullString
	var agentID sql.NullString
	var agentVersionID sql.NullString
	var sessionKeyTemplate sql.NullString
	var operation sql.NullString
	var operationParams []byte
	var filterJSON []byte
	var eventSource sql.NullString
	if err := row.Scan(
		&current.SubscriptionID,
		&current.WorkflowID,
		&current.WorkflowVersionID,
		&current.WorkflowNodeID,
		&current.DestinationType,
		&connectionID,
		&agentID,
		&agentVersionID,
		&sessionKeyTemplate,
		&current.SessionCreateIfMissing,
		&operation,
		&operationParams,
		&filterJSON,
		&current.EventType,
		&eventSource,
		&current.ArtifactStatus,
		&current.Status,
		&current.CreatedAt,
	); err != nil {
		return err
	}
	current.ConnectionID = parseOptionalUUID(connectionID)
	current.AgentID = parseOptionalUUID(agentID)
	current.AgentVersionID = parseOptionalUUID(agentVersionID)
	if sessionKeyTemplate.Valid {
		value := sessionKeyTemplate.String
		current.SessionKeyTemplate = &value
	}
	if operation.Valid {
		value := operation.String
		current.Operation = &value
	}
	current.OperationParams = json.RawMessage(operationParams)
	current.FilterJSON = json.RawMessage(filterJSON)
	if eventSource.Valid {
		value := eventSource.String
		current.EventSource = &value
	}
	return nil
}
