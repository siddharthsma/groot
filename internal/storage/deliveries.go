package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"groot/internal/delivery"
	eventpkg "groot/internal/event"
	"groot/internal/tenant"
)

func (d *DB) CreateDeliveryJob(ctx context.Context, record delivery.JobRecord) (bool, error) {
	const query = `
		INSERT INTO delivery_jobs (id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT DO NOTHING
	`
	result, err := d.db.ExecContext(ctx, query, record.ID, record.TenantID, record.SubscriptionID, record.EventID, record.WorkflowRunID, nullableString(record.WorkflowNodeID), record.IsReplay, record.ReplayOfEventID, record.Status, record.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert delivery job: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delivery job rows affected: %w", err)
	}
	return rows == 1, nil
}

func (d *DB) ClaimPendingJobs(ctx context.Context, limit int) ([]delivery.Job, error) {
	const query = `
		WITH claimed AS (
			SELECT id
			FROM delivery_jobs
			WHERE status = 'pending'
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE delivery_jobs dj
		SET status = 'in_progress'
		FROM claimed
		WHERE dj.id = claimed.id
		RETURNING dj.id, dj.tenant_id, dj.subscription_id, dj.event_id, dj.workflow_run_id, dj.workflow_node_id, dj.status, dj.attempts, dj.last_error, dj.completed_at, dj.created_at
	`
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim jobs tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		var workflowRunID sql.NullString
		var workflowNodeID sql.NullString
		if err := rows.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &workflowRunID, &workflowNodeID, &job.Status, &job.Attempts, &job.LastError, &job.CompletedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan claimed delivery job: %w", err)
		}
		job.WorkflowRunID = parseOptionalUUID(workflowRunID)
		job.WorkflowNodeID = nullableStringValue(workflowNodeID)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed delivery jobs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim jobs tx: %w", err)
	}
	return jobs, nil
}

func (d *DB) RequeueJob(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("requeue delivery job: %w", err)
	}
	return nil
}

func (d *DB) GetDeliveryJob(ctx context.Context, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, fmt.Errorf("get delivery job: %w", sql.ErrNoRows)
		}
		return delivery.Job{}, fmt.Errorf("get delivery job: %w", err)
	}
	return job, nil
}

func (d *DB) ListDeliveryJobs(ctx context.Context, tenantID tenant.ID, status string, subscriptionID, eventID *uuid.UUID, limit int) ([]delivery.Job, error) {
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
	if subscriptionID != nil {
		query += fmt.Sprintf(" AND subscription_id = $%d", nextArg)
		args = append(args, *subscriptionID)
		nextArg++
	}
	if eventID != nil {
		query += fmt.Sprintf(" AND event_id = $%d", nextArg)
		args = append(args, *eventID)
		nextArg++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", nextArg)
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delivery jobs: %w", err)
	}
	return jobs, nil
}

func (d *DB) ListDeliveryJobsForEvent(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID, limit int) ([]delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE tenant_id = $1 AND event_id = $2
		ORDER BY created_at ASC
		LIMIT $3
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventID, limit)
	if err != nil {
		return nil, fmt.Errorf("query delivery jobs for event: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan delivery job for event: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (d *DB) GetDeliveryJobForTenant(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1 AND tenant_id = $2
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID, tenantID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, delivery.ErrJobNotFound
		}
		return delivery.Job{}, fmt.Errorf("get delivery job for tenant: %w", err)
	}
	return job, nil
}

func (d *DB) ResetDeliveryJob(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', attempts = 0, last_error = NULL, completed_at = NULL
		WHERE id = $1 AND tenant_id = $2 AND status IN ('dead_letter', 'failed')
		RETURNING id, tenant_id, subscription_id, event_id, workflow_run_id, workflow_node_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID, tenantID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, delivery.ErrRetryNotAllowed
		}
		return delivery.Job{}, fmt.Errorf("reset delivery job: %w", err)
	}
	return job, nil
}

func (d *DB) SetDeliveryJobAttempt(ctx context.Context, jobID uuid.UUID, attempt int) error {
	const query = `
		UPDATE delivery_jobs
		SET attempts = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, attempt); err != nil {
		return fmt.Errorf("set delivery job attempt: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobSucceeded(ctx context.Context, jobID uuid.UUID, completedAt time.Time, externalID *string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'succeeded', last_error = NULL, external_id = $3, last_status_code = $4, completed_at = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, completedAt, externalID, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobRetryableFailure(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'in_progress', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job retryable failure: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobDeadLetter(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'dead_letter', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job dead letter: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobFailed(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'failed', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job failed: %w", err)
	}
	return nil
}

func (d *DB) SaveResultEvent(ctx context.Context, jobID uuid.UUID, event eventpkg.Event) (bool, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin result event tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const insertEvent = `
		INSERT INTO events (
			event_id, tenant_id, type, source, source_kind, source_connection_id, source_connection_name, source_external_account_id,
			lineage_integration, lineage_connection_id, lineage_connection_name, lineage_external_account_id,
			chain_depth, timestamp, payload, schema_full_name, schema_version, workflow_run_id, workflow_node_id, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
	`
	var schemaVersion any
	if event.SchemaVersion > 0 {
		schemaVersion = event.SchemaVersion
	}
	if _, err := tx.ExecContext(ctx, insertEvent,
		event.EventID,
		event.TenantID,
		event.Type,
		event.Source.Integration,
		event.SourceKind,
		event.Source.ConnectionID,
		nullableString(event.Source.ConnectionName),
		nullableString(event.Source.ExternalAccountID),
		lineageIntegration(event.Lineage),
		lineageConnectionID(event.Lineage),
		lineageConnectionName(event.Lineage),
		lineageExternalAccountID(event.Lineage),
		event.ChainDepth,
		event.Timestamp,
		[]byte(event.Payload),
		nullableString(event.SchemaFullName),
		schemaVersion,
		event.WorkflowRunID,
		nullableString(event.WorkflowNodeID),
	); err != nil {
		return false, fmt.Errorf("insert result event: %w", err)
	}

	const updateJob = `
		UPDATE delivery_jobs
		SET result_event_id = $2
		WHERE id = $1 AND result_event_id IS NULL
	`
	result, err := tx.ExecContext(ctx, updateJob, jobID, event.EventID)
	if err != nil {
		return false, fmt.Errorf("link result event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("result event rows affected: %w", err)
	}
	if rows == 0 {
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit result event tx: %w", err)
	}
	return true, nil
}

func scanDeliveryJob(row scanner, job *delivery.Job) error {
	var workflowRunID sql.NullString
	var workflowNodeID sql.NullString
	var replayOfEventID sql.NullString
	var resultEventID sql.NullString
	if err := row.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &workflowRunID, &workflowNodeID, &job.IsReplay, &replayOfEventID, &resultEventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt); err != nil {
		return err
	}
	job.WorkflowRunID = parseOptionalUUID(workflowRunID)
	job.WorkflowNodeID = nullableStringValue(workflowNodeID)
	job.ReplayOfEventID = parseOptionalUUID(replayOfEventID)
	job.ResultEventID = parseOptionalUUID(resultEventID)
	return nil
}
