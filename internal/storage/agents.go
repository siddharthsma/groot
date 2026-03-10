package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"groot/internal/agent"
	"groot/internal/tenant"
)

func (d *DB) CreateAgent(ctx context.Context, record agent.DefinitionRecord) (agent.Definition, error) {
	const query = `
		INSERT INTO agents (id, tenant_id, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, updated_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $13, $14, $15)
		RETURNING id, tenant_id, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, updated_at
	`
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("begin create agent tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	actor := actorFromContext(ctx)
	var definition agent.Definition
	allowedTools, _ := json.Marshal(record.AllowedTools)
	toolBindings, _ := json.Marshal(record.ToolBindings)
	err = scanAgentDefinition(tx.QueryRowContext(ctx, query, record.ID, record.TenantID, record.Name, record.Instructions, nullableString(optionalStringValue(record.Integration)), nullableString(optionalStringValue(record.Model)), allowedTools, toolBindings, record.MemoryEnabled, record.SessionAutoCreate, record.CreatedAt, record.UpdatedAt, actor.Type, actor.ID, actor.Email), &definition)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return agent.Definition{}, agent.ErrDuplicateName
		}
		return agent.Definition{}, fmt.Errorf("insert agent: %w", err)
	}
	if err := insertAgentVersion(ctx, tx, actor, agent.VersionRecord{
		ID:                uuid.New(),
		AgentID:           definition.ID,
		TenantID:          definition.TenantID,
		VersionNumber:     1,
		Name:              definition.Name,
		Instructions:      definition.Instructions,
		Integration:       definition.Integration,
		Model:             definition.Model,
		AllowedTools:      definition.AllowedTools,
		ToolBindings:      definition.ToolBindings,
		MemoryEnabled:     definition.MemoryEnabled,
		SessionAutoCreate: definition.SessionAutoCreate,
		CreatedAt:         definition.CreatedAt,
	}); err != nil {
		return agent.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Definition{}, fmt.Errorf("commit create agent tx: %w", err)
	}
	return definition, nil
}

func (d *DB) UpdateAgent(ctx context.Context, agentID uuid.UUID, tenantID tenant.ID, record agent.DefinitionRecord) (agent.Definition, error) {
	const query = `
		UPDATE agents
		SET name = $3,
		    instructions = $4,
		    integration = $5,
		    model = $6,
		    allowed_tools = $7,
		    tool_bindings = $8,
		    memory_enabled = $9,
		    session_auto_create = $10,
		    updated_at = $11,
		    updated_by_actor_type = $12,
		    updated_by_actor_id = $13,
		    updated_by_actor_email = $14
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, updated_at
	`
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("begin update agent tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	actor := actorFromContext(ctx)
	if err := lockAgent(ctx, tx, tenantID, agentID); err != nil {
		return agent.Definition{}, err
	}

	var definition agent.Definition
	allowedTools, _ := json.Marshal(record.AllowedTools)
	toolBindings, _ := json.Marshal(record.ToolBindings)
	err = scanAgentDefinition(tx.QueryRowContext(ctx, query, agentID, tenantID, record.Name, record.Instructions, nullableString(optionalStringValue(record.Integration)), nullableString(optionalStringValue(record.Model)), allowedTools, toolBindings, record.MemoryEnabled, record.SessionAutoCreate, record.UpdatedAt, actor.Type, actor.ID, actor.Email), &definition)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Definition{}, agent.ErrNotFound
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return agent.Definition{}, agent.ErrDuplicateName
		}
		return agent.Definition{}, fmt.Errorf("update agent: %w", err)
	}
	nextVersion, err := nextAgentVersionNumber(ctx, tx, agentID)
	if err != nil {
		return agent.Definition{}, err
	}
	if err := insertAgentVersion(ctx, tx, actor, agent.VersionRecord{
		ID:                uuid.New(),
		AgentID:           definition.ID,
		TenantID:          definition.TenantID,
		VersionNumber:     nextVersion,
		Name:              definition.Name,
		Instructions:      definition.Instructions,
		Integration:       definition.Integration,
		Model:             definition.Model,
		AllowedTools:      definition.AllowedTools,
		ToolBindings:      definition.ToolBindings,
		MemoryEnabled:     definition.MemoryEnabled,
		SessionAutoCreate: definition.SessionAutoCreate,
		CreatedAt:         definition.UpdatedAt,
	}); err != nil {
		return agent.Definition{}, err
	}
	if err := tx.Commit(); err != nil {
		return agent.Definition{}, fmt.Errorf("commit update agent tx: %w", err)
	}
	return definition, nil
}

func (d *DB) GetAgent(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) (agent.Definition, error) {
	const query = `
		SELECT id, tenant_id, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, updated_at
		FROM agents
		WHERE id = $1 AND tenant_id = $2
	`
	var definition agent.Definition
	err := scanAgentDefinition(d.db.QueryRowContext(ctx, query, agentID, tenantID), &definition)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Definition{}, agent.ErrNotFound
		}
		return agent.Definition{}, fmt.Errorf("get agent: %w", err)
	}
	return definition, nil
}

func (d *DB) ListAgents(ctx context.Context, tenantID tenant.ID) ([]agent.Definition, error) {
	const query = `
		SELECT id, tenant_id, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, updated_at
		FROM agents
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var definitions []agent.Definition
	for rows.Next() {
		var definition agent.Definition
		if err := scanAgentDefinition(rows, &definition); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}
	return definitions, nil
}

func (d *DB) ListAgentVersions(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) ([]agent.Version, error) {
	const lockQuery = `
		SELECT id
		FROM agents
		WHERE id = $1 AND tenant_id = $2
	`
	var existing uuid.UUID
	if err := d.db.QueryRowContext(ctx, lockQuery, agentID, tenantID).Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, agent.ErrNotFound
		}
		return nil, fmt.Errorf("check agent exists: %w", err)
	}

	const query = `
		SELECT id, agent_id, tenant_id, version_number, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at
		FROM agent_versions
		WHERE tenant_id = $1 AND agent_id = $2
		ORDER BY version_number DESC, created_at DESC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("query agent versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var versions []agent.Version
	for rows.Next() {
		var version agent.Version
		if err := scanAgentVersion(rows, &version); err != nil {
			return nil, fmt.Errorf("scan agent version: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent versions: %w", err)
	}
	return versions, nil
}

func (d *DB) DeleteAgent(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) error {
	const query = `DELETE FROM agents WHERE id = $1 AND tenant_id = $2`
	result, err := d.db.ExecContext(ctx, query, agentID, tenantID)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete agent rows affected: %w", err)
	}
	if rows == 0 {
		return agent.ErrNotFound
	}
	return nil
}

func (d *DB) CountActiveSubscriptionsForAgent(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM subscriptions
		WHERE tenant_id = $1 AND agent_id = $2 AND status = 'active'
	`
	var count int
	if err := d.db.QueryRowContext(ctx, query, tenantID, agentID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active subscriptions for agent: %w", err)
	}
	return count, nil
}

func (d *DB) CountActiveSessionsForAgent(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM agent_sessions
		WHERE tenant_id = $1 AND agent_id = $2 AND status = 'active'
	`
	var count int
	if err := d.db.QueryRowContext(ctx, query, tenantID, agentID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active sessions for agent: %w", err)
	}
	return count, nil
}

func (d *DB) GetAgentSession(ctx context.Context, tenantID tenant.ID, sessionID uuid.UUID) (agent.Session, error) {
	const query = `
		SELECT id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
		FROM agent_sessions
		WHERE id = $1 AND tenant_id = $2
	`
	var session agent.Session
	err := scanAgentSession(d.db.QueryRowContext(ctx, query, sessionID, tenantID), &session)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Session{}, agent.ErrSessionNotFound
		}
		return agent.Session{}, fmt.Errorf("get agent session: %w", err)
	}
	return session, nil
}

func (d *DB) ListAgentSessions(ctx context.Context, tenantID tenant.ID, agentID *uuid.UUID, status string, limit int) ([]agent.Session, error) {
	query := `
		SELECT id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
		FROM agent_sessions
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	next := 2
	if agentID != nil {
		query += fmt.Sprintf(" AND agent_id = $%d", next)
		args = append(args, *agentID)
		next++
	}
	if strings.TrimSpace(status) != "" {
		query += fmt.Sprintf(" AND status = $%d", next)
		args = append(args, strings.TrimSpace(status))
		next++
	}
	query += " ORDER BY last_activity_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", next)
		args = append(args, limit)
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query agent sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessions []agent.Session
	for rows.Next() {
		var session agent.Session
		if err := scanAgentSession(rows, &session); err != nil {
			return nil, fmt.Errorf("scan agent session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent sessions: %w", err)
	}
	return sessions, nil
}

func (d *DB) GetAgentSessionByKey(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID, sessionKey string) (agent.Session, error) {
	const query = `
		SELECT id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
		FROM agent_sessions
		WHERE tenant_id = $1 AND agent_id = $2 AND session_key = $3
	`
	var session agent.Session
	err := scanAgentSession(d.db.QueryRowContext(ctx, query, tenantID, agentID, sessionKey), &session)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Session{}, agent.ErrSessionNotFound
		}
		return agent.Session{}, fmt.Errorf("get agent session by key: %w", err)
	}
	return session, nil
}

func (d *DB) CreateAgentSession(ctx context.Context, record agent.SessionRecord) (agent.Session, error) {
	const query = `
		INSERT INTO agent_sessions (id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $11, $12, $13)
		RETURNING id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var session agent.Session
	err := scanAgentSession(d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.AgentID, record.SessionKey, record.Status, nullableString(optionalStringValue(record.Summary)), record.LastEventID, record.LastActivityAt, record.CreatedAt, record.UpdatedAt, actor.Type, actor.ID, actor.Email), &session)
	if err != nil {
		return agent.Session{}, fmt.Errorf("insert agent session: %w", err)
	}
	return session, nil
}

func (d *DB) CloseAgentSession(ctx context.Context, tenantID tenant.ID, sessionID uuid.UUID) (agent.Session, error) {
	const query = `
		UPDATE agent_sessions
		SET status = 'closed',
		    updated_at = NOW(),
		    updated_by_actor_type = $3,
		    updated_by_actor_id = $4,
		    updated_by_actor_email = $5
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var session agent.Session
	err := scanAgentSession(d.db.QueryRowContext(ctx, query, sessionID, tenantID, actor.Type, actor.ID, actor.Email), &session)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Session{}, agent.ErrSessionNotFound
		}
		return agent.Session{}, fmt.Errorf("close agent session: %w", err)
	}
	return session, nil
}

func (d *DB) UpdateAgentSessionAfterRun(ctx context.Context, sessionID uuid.UUID, summary *string, eventID *uuid.UUID, lastActivityAt time.Time) (agent.Session, error) {
	const query = `
		UPDATE agent_sessions
		SET summary = $2,
		    last_event_id = $3,
		    last_activity_at = $4,
		    updated_at = $4,
		    updated_by_actor_type = $5,
		    updated_by_actor_id = $6,
		    updated_by_actor_email = $7
		WHERE id = $1
		RETURNING id, tenant_id, agent_id, session_key, status, summary, last_event_id, last_activity_at, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var session agent.Session
	err := scanAgentSession(d.db.QueryRowContext(ctx, query, sessionID, nullableString(optionalStringValue(summary)), eventID, lastActivityAt, actor.Type, actor.ID, actor.Email), &session)
	if err != nil {
		return agent.Session{}, fmt.Errorf("update agent session after run: %w", err)
	}
	return session, nil
}

func (d *DB) LinkAgentSessionEvent(ctx context.Context, record agent.SessionEventRecord) error {
	const query = `
		INSERT INTO agent_session_events (id, agent_session_id, event_id, linked_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (agent_session_id, event_id) DO NOTHING
	`
	if _, err := d.db.ExecContext(ctx, query, record.ID, record.AgentSessionID, record.EventID, record.LinkedAt); err != nil {
		return fmt.Errorf("insert agent session event: %w", err)
	}
	return nil
}

func (d *DB) UpdateAgentRunContext(ctx context.Context, runID uuid.UUID, agentID uuid.UUID, sessionID *uuid.UUID) error {
	const query = `
		UPDATE agent_runs
		SET agent_id = $2, agent_session_id = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, agentID, sessionID); err != nil {
		return fmt.Errorf("update agent run context: %w", err)
	}
	return nil
}

func (d *DB) CreateAgentRun(ctx context.Context, record agent.RunRecord) (agent.Run, error) {
	const query = `
		INSERT INTO agent_runs (id, tenant_id, input_event_id, subscription_id, agent_id, agent_session_id, workflow_run_id, workflow_node_id, agent_version_id, status, steps, started_at, created_by_actor_type, created_by_actor_id, created_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, tenant_id, input_event_id, subscription_id, agent_id, agent_session_id, workflow_run_id, workflow_node_id, agent_version_id, status, steps, started_at, completed_at, last_error
	`
	actor := actorFromContext(ctx)
	var run agent.Run
	var agentID sql.NullString
	var sessionID sql.NullString
	var workflowRunID sql.NullString
	var workflowNodeID sql.NullString
	var agentVersionID sql.NullString
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.InputEventID, record.SubscriptionID, record.AgentID, record.AgentSessionID, record.WorkflowRunID, nullableString(optionalStringValue(record.WorkflowNodeID)), record.AgentVersionID, record.Status, record.Steps, record.StartedAt, actor.Type, actor.ID, actor.Email).Scan(
		&run.ID,
		&run.TenantID,
		&run.InputEventID,
		&run.SubscriptionID,
		&agentID,
		&sessionID,
		&workflowRunID,
		&workflowNodeID,
		&agentVersionID,
		&run.Status,
		&run.Steps,
		&run.StartedAt,
		&run.CompletedAt,
		&run.LastError,
	)
	if err != nil {
		return agent.Run{}, fmt.Errorf("insert agent run: %w", err)
	}
	run.AgentID = parseOptionalUUID(agentID)
	run.AgentSessionID = parseOptionalUUID(sessionID)
	run.WorkflowRunID = parseOptionalUUID(workflowRunID)
	if workflowNodeID.Valid {
		value := workflowNodeID.String
		run.WorkflowNodeID = &value
	}
	run.AgentVersionID = parseOptionalUUID(agentVersionID)
	return run, nil
}

func (d *DB) CreateAgentStep(ctx context.Context, record agent.StepRecord) error {
	const query = `
		INSERT INTO agent_steps (id, agent_run_id, step_num, kind, tool_name, tool_args, tool_result, llm_integration, llm_model, usage, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	if _, err := d.db.ExecContext(
		ctx,
		query,
		record.ID,
		record.AgentRunID,
		record.StepNum,
		record.Kind,
		nullableString(optionalStringValue(record.ToolName)),
		jsonBytes(record.ToolArgs),
		jsonBytes(record.ToolResult),
		nullableString(optionalStringValue(record.LLMIntegration)),
		nullableString(optionalStringValue(record.LLMModel)),
		jsonBytes(record.Usage),
		record.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert agent step: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentRunSucceeded(ctx context.Context, runID uuid.UUID, steps int, completedAt time.Time) error {
	const query = `
		UPDATE agent_runs
		SET status = 'succeeded', steps = $2, completed_at = $3, last_error = NULL
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, steps, completedAt); err != nil {
		return fmt.Errorf("mark agent run succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentRunFailed(ctx context.Context, runID uuid.UUID, steps int, completedAt time.Time, lastError string) error {
	const query = `
		UPDATE agent_runs
		SET status = 'failed', steps = $2, completed_at = $3, last_error = $4
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, steps, completedAt, lastError); err != nil {
		return fmt.Errorf("mark agent run failed: %w", err)
	}
	return nil
}

func scanAgentSession(row scanner, session *agent.Session) error {
	var summary sql.NullString
	var lastEventID sql.NullString
	if err := row.Scan(&session.ID, &session.TenantID, &session.AgentID, &session.SessionKey, &session.Status, &summary, &lastEventID, &session.LastActivityAt, &session.CreatedAt, &session.UpdatedAt); err != nil {
		return err
	}
	if summary.Valid {
		value := summary.String
		session.Summary = &value
	}
	session.LastEventID = parseOptionalUUID(lastEventID)
	return nil
}

func scanAgentDefinition(row scanner, definition *agent.Definition) error {
	var allowedTools []byte
	var toolBindings []byte
	if err := row.Scan(&definition.ID, &definition.TenantID, &definition.Name, &definition.Instructions, &definition.Integration, &definition.Model, &allowedTools, &toolBindings, &definition.MemoryEnabled, &definition.SessionAutoCreate, &definition.CreatedAt, &definition.UpdatedAt); err != nil {
		return err
	}
	if len(allowedTools) == 0 {
		definition.AllowedTools = []string{}
	} else if err := json.Unmarshal(allowedTools, &definition.AllowedTools); err != nil {
		return fmt.Errorf("decode allowed tools: %w", err)
	}
	if len(toolBindings) == 0 {
		definition.ToolBindings = map[string]agent.ToolBinding{}
	} else if err := json.Unmarshal(toolBindings, &definition.ToolBindings); err != nil {
		return fmt.Errorf("decode tool bindings: %w", err)
	}
	if definition.ToolBindings == nil {
		definition.ToolBindings = map[string]agent.ToolBinding{}
	}
	return nil
}

func insertAgentVersion(ctx context.Context, tx *sql.Tx, actor actorMetadata, record agent.VersionRecord) error {
	const query = `
		INSERT INTO agent_versions (id, agent_id, tenant_id, version_number, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	allowedTools, _ := json.Marshal(record.AllowedTools)
	toolBindings, _ := json.Marshal(record.ToolBindings)
	if _, err := tx.ExecContext(
		ctx,
		query,
		record.ID,
		record.AgentID,
		record.TenantID,
		record.VersionNumber,
		record.Name,
		record.Instructions,
		nullableString(optionalStringValue(record.Integration)),
		nullableString(optionalStringValue(record.Model)),
		allowedTools,
		toolBindings,
		record.MemoryEnabled,
		record.SessionAutoCreate,
		record.CreatedAt,
		actor.Type,
		actor.ID,
		actor.Email,
	); err != nil {
		return fmt.Errorf("insert agent version: %w", err)
	}
	return nil
}

func lockAgent(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, agentID uuid.UUID) error {
	const query = `
		SELECT id
		FROM agents
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`
	var id uuid.UUID
	if err := tx.QueryRowContext(ctx, query, agentID, tenantID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.ErrNotFound
		}
		return fmt.Errorf("lock agent: %w", err)
	}
	return nil
}

func nextAgentVersionNumber(ctx context.Context, tx *sql.Tx, agentID uuid.UUID) (int, error) {
	const query = `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM agent_versions
		WHERE agent_id = $1
	`
	var next int
	if err := tx.QueryRowContext(ctx, query, agentID).Scan(&next); err != nil {
		return 0, fmt.Errorf("next agent version number: %w", err)
	}
	return next, nil
}
