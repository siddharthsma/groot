package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	eventpkg "groot/internal/event"
	"groot/internal/tenant"
	"groot/internal/workflow"
)

func (d *DB) ListActiveWorkflowEntryBindingsByEvent(ctx context.Context, tenantID tenant.ID, eventType, integration string) ([]workflow.EntryBinding, error) {
	const query = `
		SELECT b.id, b.workflow_id, b.workflow_version_id, b.workflow_node_id, b.integration, b.event_type, b.connection_id, b.filter_json, b.status, b.created_at, b.superseded_at
		FROM workflow_entry_bindings b
		INNER JOIN workflows w ON w.id = b.workflow_id
		WHERE w.tenant_id = $1
		  AND b.status = 'active'
		  AND b.event_type = $2
		  AND b.integration = $3
		ORDER BY b.created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventType, integration)
	if err != nil {
		return nil, fmt.Errorf("query active workflow entry bindings: %w", err)
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

func (d *DB) GetActiveWorkflowSubscriptionArtifactByNode(ctx context.Context, tenantID tenant.ID, workflowVersionID uuid.UUID, workflowNodeID string) (workflow.SubscriptionArtifact, error) {
	const query = `
		SELECT id, workflow_id, workflow_version_id, workflow_node_id, destination_type, connector_instance_id, agent_id, agent_version_id, session_key_template, session_create_if_missing, operation, operation_params, filter_json, event_type, event_source, workflow_artifact_status, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		  AND managed_by_workflow = TRUE
		  AND workflow_version_id = $2
		  AND workflow_node_id = $3
		  AND workflow_artifact_status = 'active'
		LIMIT 1
	`
	var current workflow.SubscriptionArtifact
	if err := scanWorkflowSubscriptionArtifact(d.db.QueryRowContext(ctx, query, tenantID, workflowVersionID, workflowNodeID), &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.SubscriptionArtifact{}, sql.ErrNoRows
		}
		return workflow.SubscriptionArtifact{}, fmt.Errorf("get workflow subscription artifact by node: %w", err)
	}
	return current, nil
}

func (d *DB) CreateWorkflowRun(ctx context.Context, run workflow.RunRecord, triggerStep workflow.RunStepRecord) (workflow.Run, workflow.RunStep, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Run{}, workflow.RunStep{}, fmt.Errorf("begin workflow run tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	currentRun, err := insertWorkflowRun(ctx, tx, run)
	if err != nil {
		return workflow.Run{}, workflow.RunStep{}, err
	}
	triggerStep.WorkflowRunID = currentRun.ID
	currentStep, err := insertWorkflowRunStep(ctx, tx, triggerStep)
	if err != nil {
		return workflow.Run{}, workflow.RunStep{}, err
	}
	if err := tx.Commit(); err != nil {
		return workflow.Run{}, workflow.RunStep{}, fmt.Errorf("commit workflow run tx: %w", err)
	}
	return currentRun, currentStep, nil
}

func (d *DB) CreateWorkflowRunStep(ctx context.Context, record workflow.RunStepRecord) (workflow.RunStep, error) {
	step, err := insertWorkflowRunStep(ctx, d.db, record)
	if err != nil {
		return workflow.RunStep{}, err
	}
	return step, nil
}

func (d *DB) CompleteWorkflowRunStep(ctx context.Context, stepID uuid.UUID, status string, completedAt time.Time, outputEventID *uuid.UUID, deliveryJobID *uuid.UUID, agentRunID *uuid.UUID, errorJSON, outputSummary json.RawMessage) error {
	const query = `
		UPDATE workflow_run_steps
		SET status = $2,
		    completed_at = $3,
		    output_event_id = COALESCE($4, output_event_id),
		    delivery_job_id = COALESCE($5, delivery_job_id),
		    agent_run_id = COALESCE($6, agent_run_id),
		    error_json = $7,
		    output_summary_json = $8
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, stepID, status, completedAt, outputEventID, deliveryJobID, agentRunID, jsonBytes(errorJSON), jsonBytes(outputSummary)); err != nil {
		return fmt.Errorf("complete workflow run step: %w", err)
	}
	return nil
}

func (d *DB) AttachWorkflowRunStepDelivery(ctx context.Context, stepID uuid.UUID, deliveryJobID uuid.UUID) error {
	const query = `UPDATE workflow_run_steps SET delivery_job_id = $2 WHERE id = $1`
	if _, err := d.db.ExecContext(ctx, query, stepID, deliveryJobID); err != nil {
		return fmt.Errorf("attach workflow step delivery: %w", err)
	}
	return nil
}

func (d *DB) AttachWorkflowRunStepAgentRun(ctx context.Context, stepID uuid.UUID, agentRunID uuid.UUID) error {
	const query = `UPDATE workflow_run_steps SET agent_run_id = $2 WHERE id = $1`
	if _, err := d.db.ExecContext(ctx, query, stepID, agentRunID); err != nil {
		return fmt.Errorf("attach workflow step agent run: %w", err)
	}
	return nil
}

func (d *DB) CreateWorkflowRunWait(ctx context.Context, record workflow.RunWaitRecord) (workflow.RunWait, error) {
	const query = `
		INSERT INTO workflow_run_waits (id, workflow_run_id, workflow_version_id, workflow_node_id, status, expected_event_type, expected_integration, correlation_strategy, correlation_key, matched_event_id, expires_at, created_at, matched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, workflow_run_id, workflow_version_id, workflow_node_id, status, expected_event_type, expected_integration, correlation_strategy, correlation_key, matched_event_id, expires_at, created_at, matched_at
	`
	var current workflow.RunWait
	if err := scanWorkflowRunWait(d.db.QueryRowContext(ctx, query, record.ID, record.WorkflowRunID, record.WorkflowVersionID, record.WorkflowNodeID, record.Status, record.ExpectedEventType, record.ExpectedIntegration, record.CorrelationStrategy, record.CorrelationKey, record.MatchedEventID, record.ExpiresAt, record.CreatedAt, record.MatchedAt), &current); err != nil {
		return workflow.RunWait{}, fmt.Errorf("insert workflow run wait: %w", err)
	}
	return current, nil
}

func (d *DB) GetWorkflowRun(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) (workflow.Run, error) {
	const query = `
		SELECT id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error
		FROM workflow_runs
		WHERE id = $1 AND tenant_id = $2
	`
	var run workflow.Run
	if err := scanWorkflowRun(d.db.QueryRowContext(ctx, query, runID, tenantID), &run); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Run{}, workflow.ErrWorkflowNotFound
		}
		return workflow.Run{}, fmt.Errorf("get workflow run: %w", err)
	}
	return run, nil
}

func (d *DB) GetWorkflowRunByID(ctx context.Context, runID uuid.UUID) (workflow.Run, error) {
	const query = `
		SELECT id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error
		FROM workflow_runs
		WHERE id = $1
	`
	var run workflow.Run
	if err := scanWorkflowRun(d.db.QueryRowContext(ctx, query, runID), &run); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Run{}, sql.ErrNoRows
		}
		return workflow.Run{}, fmt.Errorf("get workflow run by id: %w", err)
	}
	return run, nil
}

func (d *DB) ListWorkflowRuns(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, limit int) ([]workflow.Run, error) {
	const query = `
		SELECT id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error
		FROM workflow_runs
		WHERE tenant_id = $1 AND workflow_id = $2
		ORDER BY started_at DESC
		LIMIT $3
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, workflowID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workflow runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]workflow.Run, 0)
	for rows.Next() {
		var current workflow.Run
		if err := scanWorkflowRun(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow run: %w", err)
		}
		out = append(out, current)
	}
	return out, rows.Err()
}

func (d *DB) ListWorkflowRunSteps(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) ([]workflow.RunStep, error) {
	const query = `
		SELECT s.id, s.workflow_run_id, s.workflow_node_id, s.node_type, s.status, s.branch_key, s.input_event_id, s.output_event_id, s.subscription_id, s.delivery_job_id, s.agent_run_id, s.started_at, s.completed_at, s.error_json, s.output_summary_json
		FROM workflow_run_steps s
		INNER JOIN workflow_runs r ON r.id = s.workflow_run_id
		WHERE s.workflow_run_id = $1 AND r.tenant_id = $2
		ORDER BY s.started_at ASC, s.id ASC
	`
	rows, err := d.db.QueryContext(ctx, query, runID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query workflow run steps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]workflow.RunStep, 0)
	for rows.Next() {
		var current workflow.RunStep
		if err := scanWorkflowRunStep(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow run step: %w", err)
		}
		out = append(out, current)
	}
	return out, rows.Err()
}

func (d *DB) ListWorkflowRunWaits(ctx context.Context, tenantID tenant.ID, runID uuid.UUID) ([]workflow.RunWait, error) {
	const query = `
		SELECT w.id, w.workflow_run_id, w.workflow_version_id, w.workflow_node_id, w.status, w.expected_event_type, w.expected_integration, w.correlation_strategy, w.correlation_key, w.matched_event_id, w.expires_at, w.created_at, w.matched_at
		FROM workflow_run_waits w
		INNER JOIN workflow_runs r ON r.id = w.workflow_run_id
		WHERE w.workflow_run_id = $1 AND r.tenant_id = $2
		ORDER BY w.created_at ASC, w.id ASC
	`
	rows, err := d.db.QueryContext(ctx, query, runID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query workflow run waits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]workflow.RunWait, 0)
	for rows.Next() {
		var current workflow.RunWait
		if err := scanWorkflowRunWait(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow run wait: %w", err)
		}
		out = append(out, current)
	}
	return out, rows.Err()
}

func (d *DB) ListMatchingWorkflowRunWaits(ctx context.Context, tenantID tenant.ID, eventType, integration, correlationKey string) ([]workflow.RunWait, error) {
	const query = `
		SELECT w.id, w.workflow_run_id, w.workflow_version_id, w.workflow_node_id, w.status, w.expected_event_type, w.expected_integration, w.correlation_strategy, w.correlation_key, w.matched_event_id, w.expires_at, w.created_at, w.matched_at
		FROM workflow_run_waits w
		INNER JOIN workflow_runs r ON r.id = w.workflow_run_id
		WHERE r.tenant_id = $1
		  AND w.status = 'waiting'
		  AND w.expected_event_type = $2
		  AND w.expected_integration = $3
		  AND w.correlation_key = $4
		ORDER BY w.created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventType, integration, correlationKey)
	if err != nil {
		return nil, fmt.Errorf("query matching workflow waits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]workflow.RunWait, 0)
	for rows.Next() {
		var current workflow.RunWait
		if err := scanWorkflowRunWait(rows, &current); err != nil {
			return nil, fmt.Errorf("scan matching workflow wait: %w", err)
		}
		out = append(out, current)
	}
	return out, rows.Err()
}

func (d *DB) ListExpiredWorkflowRunWaits(ctx context.Context, now time.Time, limit int) ([]workflow.RunWait, error) {
	const query = `
		SELECT id, workflow_run_id, workflow_version_id, workflow_node_id, status, expected_event_type, expected_integration, correlation_strategy, correlation_key, matched_event_id, expires_at, created_at, matched_at
		FROM workflow_run_waits
		WHERE status = 'waiting'
		  AND expires_at IS NOT NULL
		  AND expires_at <= $1
		ORDER BY expires_at ASC
		LIMIT $2
	`
	rows, err := d.db.QueryContext(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("query expired workflow waits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]workflow.RunWait, 0)
	for rows.Next() {
		var current workflow.RunWait
		if err := scanWorkflowRunWait(rows, &current); err != nil {
			return nil, fmt.Errorf("scan expired workflow wait: %w", err)
		}
		out = append(out, current)
	}
	return out, rows.Err()
}

func (d *DB) MatchWorkflowRunWait(ctx context.Context, waitID uuid.UUID, matchedEventID uuid.UUID, matchedAt time.Time) error {
	const query = `
		UPDATE workflow_run_waits
		SET status = 'matched', matched_event_id = $2, matched_at = $3
		WHERE id = $1 AND status = 'waiting'
	`
	if _, err := d.db.ExecContext(ctx, query, waitID, matchedEventID, matchedAt); err != nil {
		return fmt.Errorf("match workflow wait: %w", err)
	}
	return nil
}

func (d *DB) TimeoutWorkflowRunWait(ctx context.Context, waitID uuid.UUID, completedAt time.Time) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin timeout workflow wait tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const updateWait = `
		UPDATE workflow_run_waits
		SET status = 'timed_out', matched_at = $2
		WHERE id = $1 AND status = 'waiting'
	`
	if _, err := tx.ExecContext(ctx, updateWait, waitID, completedAt); err != nil {
		return fmt.Errorf("timeout workflow wait: %w", err)
	}
	const updateSteps = `
		UPDATE workflow_run_steps
		SET status = 'timed_out', completed_at = $2
		WHERE workflow_run_id = (
			SELECT workflow_run_id FROM workflow_run_waits WHERE id = $1
		)
		  AND workflow_node_id = (
			SELECT workflow_node_id FROM workflow_run_waits WHERE id = $1
		)
		  AND status IN ('waiting', 'running')
	`
	if _, err := tx.ExecContext(ctx, updateSteps, waitID, completedAt); err != nil {
		return fmt.Errorf("timeout workflow wait steps: %w", err)
	}
	const updateRun = `
		UPDATE workflow_runs
		SET status = 'timed_out', completed_at = $2, last_error = 'wait timed out'
		WHERE id = (SELECT workflow_run_id FROM workflow_run_waits WHERE id = $1)
		  AND status IN ('running', 'waiting')
	`
	if _, err := tx.ExecContext(ctx, updateRun, waitID, completedAt); err != nil {
		return fmt.Errorf("timeout workflow run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit timeout workflow wait tx: %w", err)
	}
	return nil
}

func (d *DB) SetWorkflowRunStatus(ctx context.Context, runID uuid.UUID, status string, completedAt *time.Time, lastError *string) error {
	const query = `
		UPDATE workflow_runs
		SET status = $2,
		    completed_at = $3,
		    last_error = $4
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, status, completedAt, nullableString(optionalStringValue(lastError))); err != nil {
		return fmt.Errorf("set workflow run status: %w", err)
	}
	return nil
}

func (d *DB) CancelWorkflowRun(ctx context.Context, tenantID tenant.ID, runID uuid.UUID, completedAt time.Time) (workflow.Run, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Run{}, fmt.Errorf("begin cancel workflow run tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const updateRun = `
		UPDATE workflow_runs
		SET status = 'cancelled', completed_at = $3, last_error = NULL
		WHERE id = $1 AND tenant_id = $2 AND status IN ('running', 'waiting')
		RETURNING id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error
	`
	var run workflow.Run
	if err := scanWorkflowRun(tx.QueryRowContext(ctx, updateRun, runID, tenantID, completedAt), &run); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Run{}, workflow.ErrWorkflowNotFound
		}
		return workflow.Run{}, fmt.Errorf("cancel workflow run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_run_waits
		SET status = 'cancelled', matched_at = $2
		WHERE workflow_run_id = $1 AND status = 'waiting'
	`, runID, completedAt); err != nil {
		return workflow.Run{}, fmt.Errorf("cancel workflow waits: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE workflow_run_steps
		SET status = CASE
			WHEN status = 'waiting' THEN 'timed_out'
			ELSE status
		END,
		    completed_at = COALESCE(completed_at, $2)
		WHERE workflow_run_id = $1 AND status IN ('running', 'waiting')
	`, runID, completedAt); err != nil {
		return workflow.Run{}, fmt.Errorf("cancel workflow run steps: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return workflow.Run{}, fmt.Errorf("commit cancel workflow run tx: %w", err)
	}
	return run, nil
}

func (d *DB) MarkWorkflowDeliverySucceeded(ctx context.Context, deliveryJobID uuid.UUID, completedAt time.Time) error {
	const query = `
		UPDATE workflow_run_steps
		SET status = 'succeeded', completed_at = $2
		WHERE delivery_job_id = $1
		  AND status IN ('pending', 'running')
	`
	if _, err := d.db.ExecContext(ctx, query, deliveryJobID, completedAt); err != nil {
		return fmt.Errorf("mark workflow delivery succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkWorkflowDeliveryFailed(ctx context.Context, deliveryJobID uuid.UUID, completedAt time.Time, message string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow delivery failed tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const updateSteps = `
		UPDATE workflow_run_steps
		SET status = 'failed',
		    completed_at = $2,
		    error_json = $3::jsonb
		WHERE delivery_job_id = $1
		  AND status IN ('pending', 'running')
	`
	errorBody, _ := json.Marshal(map[string]any{"message": message})
	if _, err := tx.ExecContext(ctx, updateSteps, deliveryJobID, completedAt, string(errorBody)); err != nil {
		return fmt.Errorf("mark workflow delivery step failed: %w", err)
	}
	const updateRuns = `
		UPDATE workflow_runs
		SET status = 'failed', completed_at = $2, last_error = $3
		WHERE id IN (
			SELECT workflow_run_id
			FROM workflow_run_steps
			WHERE delivery_job_id = $1
		)
		  AND status IN ('running', 'waiting')
	`
	if _, err := tx.ExecContext(ctx, updateRuns, deliveryJobID, completedAt, message); err != nil {
		return fmt.Errorf("mark workflow run failed from delivery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow delivery failed tx: %w", err)
	}
	return nil
}

func insertWorkflowRun(ctx context.Context, exec interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, record workflow.RunRecord) (workflow.Run, error) {
	const query = `
		INSERT INTO workflow_runs (id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, workflow_id, workflow_version_id, tenant_id, trigger_event_id, status, root_workflow_node_id, triggered_by_event_type, triggered_by_connection_id, started_at, completed_at, last_error
	`
	var current workflow.Run
	if err := scanWorkflowRun(exec.QueryRowContext(ctx, query, record.ID, record.WorkflowID, record.WorkflowVersionID, record.TenantID, record.TriggerEventID, record.Status, record.RootWorkflowNodeID, record.TriggeredByEventType, record.TriggeredByConnectionID, record.StartedAt, record.CompletedAt, nullableString(optionalStringValue(record.LastError))), &current); err != nil {
		return workflow.Run{}, fmt.Errorf("insert workflow run: %w", err)
	}
	return current, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertWorkflowRunStep(ctx context.Context, exec queryRower, record workflow.RunStepRecord) (workflow.RunStep, error) {
	const query = `
		INSERT INTO workflow_run_steps (id, workflow_run_id, workflow_node_id, node_type, status, branch_key, input_event_id, output_event_id, subscription_id, delivery_job_id, agent_run_id, started_at, completed_at, error_json, output_summary_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, workflow_run_id, workflow_node_id, node_type, status, branch_key, input_event_id, output_event_id, subscription_id, delivery_job_id, agent_run_id, started_at, completed_at, error_json, output_summary_json
	`
	var current workflow.RunStep
	if err := scanWorkflowRunStep(exec.QueryRowContext(ctx, query, record.ID, record.WorkflowRunID, record.WorkflowNodeID, record.NodeType, record.Status, nullableString(optionalStringValue(record.BranchKey)), record.InputEventID, record.OutputEventID, record.SubscriptionID, record.DeliveryJobID, record.AgentRunID, record.StartedAt, record.CompletedAt, jsonBytes(record.ErrorJSON), jsonBytes(record.OutputSummaryJSON)), &current); err != nil {
		return workflow.RunStep{}, fmt.Errorf("insert workflow run step: %w", err)
	}
	return current, nil
}

func scanWorkflowRun(row scanner, run *workflow.Run) error {
	var triggeredByConnectionID sql.NullString
	if err := row.Scan(&run.ID, &run.WorkflowID, &run.WorkflowVersionID, &run.TenantID, &run.TriggerEventID, &run.Status, &run.RootWorkflowNodeID, &run.TriggeredByEventType, &triggeredByConnectionID, &run.StartedAt, &run.CompletedAt, &run.LastError); err != nil {
		return err
	}
	run.TriggeredByConnectionID = parseOptionalUUID(triggeredByConnectionID)
	return nil
}

func scanWorkflowRunStep(row scanner, step *workflow.RunStep) error {
	var branchKey sql.NullString
	var inputEventID sql.NullString
	var outputEventID sql.NullString
	var subscriptionID sql.NullString
	var deliveryJobID sql.NullString
	var agentRunID sql.NullString
	var errorJSON []byte
	var outputSummary []byte
	if err := row.Scan(&step.ID, &step.WorkflowRunID, &step.WorkflowNodeID, &step.NodeType, &step.Status, &branchKey, &inputEventID, &outputEventID, &subscriptionID, &deliveryJobID, &agentRunID, &step.StartedAt, &step.CompletedAt, &errorJSON, &outputSummary); err != nil {
		return err
	}
	if branchKey.Valid {
		value := branchKey.String
		step.BranchKey = &value
	}
	step.InputEventID = parseOptionalUUID(inputEventID)
	step.OutputEventID = parseOptionalUUID(outputEventID)
	step.SubscriptionID = parseOptionalUUID(subscriptionID)
	step.DeliveryJobID = parseOptionalUUID(deliveryJobID)
	step.AgentRunID = parseOptionalUUID(agentRunID)
	step.ErrorJSON = json.RawMessage(errorJSON)
	step.OutputSummaryJSON = json.RawMessage(outputSummary)
	return nil
}

func scanWorkflowRunWait(row scanner, wait *workflow.RunWait) error {
	var matchedEventID sql.NullString
	if err := row.Scan(&wait.ID, &wait.WorkflowRunID, &wait.WorkflowVersionID, &wait.WorkflowNodeID, &wait.Status, &wait.ExpectedEventType, &wait.ExpectedIntegration, &wait.CorrelationStrategy, &wait.CorrelationKey, &matchedEventID, &wait.ExpiresAt, &wait.CreatedAt, &wait.MatchedAt); err != nil {
		return err
	}
	wait.MatchedEventID = parseOptionalUUID(matchedEventID)
	return nil
}

func (d *DB) GetWorkflowVersionCompiled(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (workflow.Version, error) {
	return d.GetWorkflowVersion(ctx, tenantID, versionID)
}

func (d *DB) GetWorkflowVersionCompiledByID(ctx context.Context, versionID uuid.UUID) (workflow.Version, error) {
	const query = `
		SELECT v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
		FROM workflow_versions v
		WHERE v.id = $1
	`
	var version workflow.Version
	if err := scanWorkflowVersion(d.db.QueryRowContext(ctx, query, versionID), &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("get workflow version by id: %w", err)
	}
	return version, nil
}

func (d *DB) GetEventInternal(ctx context.Context, eventID uuid.UUID) (eventpkg.Event, error) {
	return d.GetEvent(ctx, eventID)
}
