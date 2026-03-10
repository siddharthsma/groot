package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/tenant"
	"groot/internal/workflow"
)

func (d *DB) CreateWorkflow(ctx context.Context, record workflow.WorkflowRecord) (workflow.Workflow, error) {
	const query = `
		INSERT INTO workflows (id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $12, $13, $14)
		RETURNING id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var created workflow.Workflow
	err := scanWorkflow(d.db.QueryRowContext(
		ctx,
		query,
		record.ID,
		record.TenantID,
		record.Name,
		nullableString(record.Description),
		record.Status,
		record.CurrentDraftVersionID,
		record.PublishedVersionID,
		record.PublishedAt,
		nullableString(optionalStringValue(record.LastPublishError)),
		record.CreatedAt,
		record.UpdatedAt,
		actor.Type,
		actor.ID,
		actor.Email,
	), &created)
	if err != nil {
		if isUniqueViolation(err) {
			return workflow.Workflow{}, workflow.ErrDuplicateWorkflowName
		}
		return workflow.Workflow{}, fmt.Errorf("insert workflow: %w", err)
	}
	return created, nil
}

func (d *DB) ListWorkflows(ctx context.Context, tenantID tenant.ID) ([]workflow.Workflow, error) {
	const query = `
		SELECT id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at
		FROM workflows
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []workflow.Workflow
	for rows.Next() {
		var current workflow.Workflow
		if err := scanWorkflow(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		out = append(out, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflows: %w", err)
	}
	return out, nil
}

func (d *DB) GetWorkflow(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) (workflow.Workflow, error) {
	const query = `
		SELECT id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at
		FROM workflows
		WHERE id = $1 AND tenant_id = $2
	`
	var current workflow.Workflow
	err := scanWorkflow(d.db.QueryRowContext(ctx, query, workflowID, tenantID), &current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Workflow{}, workflow.ErrWorkflowNotFound
		}
		return workflow.Workflow{}, fmt.Errorf("get workflow: %w", err)
	}
	return current, nil
}

func (d *DB) UpdateWorkflow(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, record workflow.WorkflowRecord) (workflow.Workflow, error) {
	const query = `
		UPDATE workflows
		SET name = $3,
		    description = $4,
		    status = $5,
		    current_draft_version_id = $6,
		    published_version_id = $7,
		    published_at = $8,
		    last_publish_error = $9,
		    updated_at = $10,
		    updated_by_actor_type = $11,
		    updated_by_actor_id = $12,
		    updated_by_actor_email = $13
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, description, status, current_draft_version_id, published_version_id, published_at, last_publish_error, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var updated workflow.Workflow
	err := scanWorkflow(
		d.db.QueryRowContext(
			ctx,
			query,
			workflowID,
			tenantID,
			record.Name,
			nullableString(record.Description),
			record.Status,
			record.CurrentDraftVersionID,
			record.PublishedVersionID,
			record.PublishedAt,
			nullableString(optionalStringValue(record.LastPublishError)),
			record.UpdatedAt,
			actor.Type,
			actor.ID,
			actor.Email,
		),
		&updated,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Workflow{}, workflow.ErrWorkflowNotFound
		}
		if isUniqueViolation(err) {
			return workflow.Workflow{}, workflow.ErrDuplicateWorkflowName
		}
		return workflow.Workflow{}, fmt.Errorf("update workflow: %w", err)
	}
	return updated, nil
}

func (d *DB) CreateWorkflowVersion(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, record workflow.VersionRecord) (workflow.Version, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Version{}, fmt.Errorf("begin workflow version tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	actor := actorFromContext(ctx)
	now := record.CreatedAt
	if err := lockWorkflow(ctx, tx, tenantID, workflowID); err != nil {
		return workflow.Version{}, err
	}

	versionNumber, err := nextWorkflowVersionNumber(ctx, tx, workflowID)
	if err != nil {
		return workflow.Version{}, err
	}

	const insertQuery = `
		INSERT INTO workflow_versions (id, workflow_id, version_number, status, definition_json, compiled_json, validation_errors_json, compiled_hash, is_valid, published_at, superseded_at, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, workflow_id, version_number, status, definition_json, compiled_json, validation_errors_json, compiled_hash, is_valid, published_at, superseded_at, created_at
	`
	var created workflow.Version
	err = scanWorkflowVersion(
		tx.QueryRowContext(
			ctx,
			insertQuery,
			record.ID,
			workflowID,
			versionNumber,
			record.Status,
			[]byte(record.DefinitionJSON),
			jsonBytes(record.CompiledJSON),
			jsonBytes(record.ValidationErrorsJSON),
			record.CompiledHash,
			record.IsValid,
			record.PublishedAt,
			record.SupersededAt,
			now,
			actor.Type,
			actor.ID,
			actor.Email,
		),
		&created,
	)
	if err != nil {
		return workflow.Version{}, fmt.Errorf("insert workflow version: %w", err)
	}

	const updateWorkflowQuery = `
		UPDATE workflows
		SET current_draft_version_id = $3,
		    updated_at = $4,
		    updated_by_actor_type = $5,
		    updated_by_actor_id = $6,
		    updated_by_actor_email = $7
		WHERE id = $1 AND tenant_id = $2
	`
	if _, err := tx.ExecContext(ctx, updateWorkflowQuery, workflowID, tenantID, created.ID, now, actor.Type, actor.ID, actor.Email); err != nil {
		return workflow.Version{}, fmt.Errorf("update workflow current draft version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return workflow.Version{}, fmt.Errorf("commit workflow version tx: %w", err)
	}
	return created, nil
}

func (d *DB) ListWorkflowVersions(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) ([]workflow.Version, error) {
	if _, err := d.GetWorkflow(ctx, tenantID, workflowID); err != nil {
		return nil, err
	}

	const query = `
		SELECT v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
		FROM workflow_versions v
		INNER JOIN workflows w ON w.id = v.workflow_id
		WHERE v.workflow_id = $1 AND w.tenant_id = $2
		ORDER BY v.version_number ASC
	`
	rows, err := d.db.QueryContext(ctx, query, workflowID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query workflow versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []workflow.Version
	for rows.Next() {
		var current workflow.Version
		if err := scanWorkflowVersion(rows, &current); err != nil {
			return nil, fmt.Errorf("scan workflow version: %w", err)
		}
		out = append(out, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow versions: %w", err)
	}
	return out, nil
}

func (d *DB) GetWorkflowVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (workflow.Version, error) {
	const query = `
		SELECT v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
		FROM workflow_versions v
		INNER JOIN workflows w ON w.id = v.workflow_id
		WHERE v.id = $1 AND w.tenant_id = $2
	`
	var current workflow.Version
	err := scanWorkflowVersion(d.db.QueryRowContext(ctx, query, versionID, tenantID), &current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("get workflow version: %w", err)
	}
	return current, nil
}

func (d *DB) UpdateWorkflowVersionDefinition(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID, definitionJSON json.RawMessage) (workflow.Version, error) {
	const query = `
		UPDATE workflow_versions v
		SET definition_json = $3,
		    compiled_json = NULL,
		    validation_errors_json = NULL,
		    compiled_hash = NULL,
		    is_valid = FALSE,
		    published_at = NULL,
		    superseded_at = NULL
		FROM workflows w
		WHERE v.id = $1
		  AND v.workflow_id = w.id
		  AND w.tenant_id = $2
		RETURNING v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
	`
	var updated workflow.Version
	err := scanWorkflowVersion(d.db.QueryRowContext(ctx, query, versionID, tenantID, definitionJSON), &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("update workflow version definition: %w", err)
	}
	return updated, nil
}

func (d *DB) UpdateWorkflowVersionValidation(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID, validationErrorsJSON json.RawMessage) (workflow.Version, error) {
	const query = `
		UPDATE workflow_versions v
		SET validation_errors_json = $3::jsonb,
		    is_valid = CASE
		        WHEN $3::jsonb IS NULL THEN TRUE
		        ELSE COALESCE(jsonb_array_length($3::jsonb), 0) = 0
		    END
		FROM workflows w
		WHERE v.id = $1
		  AND v.workflow_id = w.id
		  AND w.tenant_id = $2
		RETURNING v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
	`
	var updated workflow.Version
	err := scanWorkflowVersion(d.db.QueryRowContext(ctx, query, versionID, tenantID, jsonBytes(validationErrorsJSON)), &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("update workflow version validation: %w", err)
	}
	return updated, nil
}

func (d *DB) UpdateWorkflowVersionCompiled(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID, compiledJSON, validationErrorsJSON json.RawMessage) (workflow.Version, error) {
	const query = `
		UPDATE workflow_versions v
		SET compiled_json = $3,
		    validation_errors_json = $4,
		    compiled_hash = $5,
		    is_valid = TRUE
		FROM workflows w
		WHERE v.id = $1
		  AND v.workflow_id = w.id
		  AND w.tenant_id = $2
		RETURNING v.id, v.workflow_id, v.version_number, v.status, v.definition_json, v.compiled_json, v.validation_errors_json, v.compiled_hash, v.is_valid, v.published_at, v.superseded_at, v.created_at
	`
	var updated workflow.Version
	hash := sha256.Sum256(compiledJSON)
	err := scanWorkflowVersion(d.db.QueryRowContext(ctx, query, versionID, tenantID, jsonBytes(compiledJSON), jsonBytes(validationErrorsJSON), fmt.Sprintf("%x", hash[:])), &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.Version{}, workflow.ErrVersionNotFound
		}
		return workflow.Version{}, fmt.Errorf("update workflow version compiled: %w", err)
	}
	return updated, nil
}

func (d *DB) GetAgentVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (agent.Version, error) {
	const query = `
		SELECT id, agent_id, tenant_id, version_number, name, instructions, integration, model, allowed_tools, tool_bindings, memory_enabled, session_auto_create, created_at
		FROM agent_versions
		WHERE id = $1 AND tenant_id = $2
	`
	var current agent.Version
	err := scanAgentVersion(d.db.QueryRowContext(ctx, query, versionID, tenantID), &current)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agent.Version{}, agent.ErrVersionNotFound
		}
		return agent.Version{}, fmt.Errorf("get agent version: %w", err)
	}
	return current, nil
}

func lockWorkflow(ctx context.Context, tx *sql.Tx, tenantID tenant.ID, workflowID uuid.UUID) error {
	const query = `
		SELECT id
		FROM workflows
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE
	`
	var id uuid.UUID
	if err := tx.QueryRowContext(ctx, query, workflowID, tenantID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workflow.ErrWorkflowNotFound
		}
		return fmt.Errorf("lock workflow: %w", err)
	}
	return nil
}

func nextWorkflowVersionNumber(ctx context.Context, tx *sql.Tx, workflowID uuid.UUID) (int, error) {
	const query = `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM workflow_versions
		WHERE workflow_id = $1
	`
	var next int
	if err := tx.QueryRowContext(ctx, query, workflowID).Scan(&next); err != nil {
		return 0, fmt.Errorf("next workflow version number: %w", err)
	}
	return next, nil
}

func scanWorkflow(row scanner, current *workflow.Workflow) error {
	var description sql.NullString
	var currentDraftID sql.NullString
	var publishedVersionID sql.NullString
	var publishedAt sql.NullTime
	var lastPublishError sql.NullString
	if err := row.Scan(
		&current.ID,
		&current.TenantID,
		&current.Name,
		&description,
		&current.Status,
		&currentDraftID,
		&publishedVersionID,
		&publishedAt,
		&lastPublishError,
		&current.CreatedAt,
		&current.UpdatedAt,
	); err != nil {
		return err
	}
	current.Description = nullableStringValue(description)
	current.CurrentDraftVersionID = parseOptionalUUID(currentDraftID)
	current.PublishedVersionID = parseOptionalUUID(publishedVersionID)
	if publishedAt.Valid {
		value := publishedAt.Time
		current.PublishedAt = &value
	}
	if lastPublishError.Valid {
		value := lastPublishError.String
		current.LastPublishError = &value
	}
	return nil
}

func scanWorkflowVersion(row scanner, current *workflow.Version) error {
	var compiledJSON []byte
	var validationErrorsJSON []byte
	var compiledHash sql.NullString
	var publishedAt sql.NullTime
	var supersededAt sql.NullTime
	if err := row.Scan(
		&current.ID,
		&current.WorkflowID,
		&current.VersionNumber,
		&current.Status,
		&current.DefinitionJSON,
		&compiledJSON,
		&validationErrorsJSON,
		&compiledHash,
		&current.IsValid,
		&publishedAt,
		&supersededAt,
		&current.CreatedAt,
	); err != nil {
		return err
	}
	if len(compiledJSON) > 0 {
		current.CompiledJSON = compiledJSON
	}
	if len(validationErrorsJSON) > 0 {
		current.ValidationErrorsJSON = validationErrorsJSON
	}
	if compiledHash.Valid {
		value := compiledHash.String
		current.CompiledHash = &value
	}
	if publishedAt.Valid {
		value := publishedAt.Time
		current.PublishedAt = &value
	}
	if supersededAt.Valid {
		value := supersededAt.Time
		current.SupersededAt = &value
	}
	return nil
}

func scanAgentVersion(row scanner, current *agent.Version) error {
	var allowedTools []byte
	var toolBindings []byte
	if err := row.Scan(
		&current.ID,
		&current.AgentID,
		&current.TenantID,
		&current.VersionNumber,
		&current.Name,
		&current.Instructions,
		&current.Integration,
		&current.Model,
		&allowedTools,
		&toolBindings,
		&current.MemoryEnabled,
		&current.SessionAutoCreate,
		&current.CreatedAt,
	); err != nil {
		return err
	}
	if len(allowedTools) == 0 {
		current.AllowedTools = []string{}
	} else if err := json.Unmarshal(allowedTools, &current.AllowedTools); err != nil {
		return fmt.Errorf("decode allowed tools: %w", err)
	}
	if len(toolBindings) == 0 {
		current.ToolBindings = map[string]agent.ToolBinding{}
	} else if err := json.Unmarshal(toolBindings, &current.ToolBindings); err != nil {
		return fmt.Errorf("decode tool bindings: %w", err)
	}
	if current.ToolBindings == nil {
		current.ToolBindings = map[string]agent.ToolBinding{}
	}
	return nil
}
