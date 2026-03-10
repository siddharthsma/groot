package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	eventpkg "groot/internal/event"
	"groot/internal/tenant"
)

func (d *DB) SaveEvent(ctx context.Context, evt eventpkg.Event) error {
	const query = `
		INSERT INTO events (
			event_id, tenant_id, type, source, source_kind, source_connection_id, source_connection_name, source_external_account_id,
			lineage_integration, lineage_connection_id, lineage_connection_name, lineage_external_account_id,
			chain_depth, timestamp, payload, schema_full_name, schema_version, workflow_run_id, workflow_node_id, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
	`
	var schemaVersion any
	if evt.SchemaVersion > 0 {
		schemaVersion = evt.SchemaVersion
	}
	if _, err := d.db.ExecContext(ctx, query,
		evt.EventID,
		evt.TenantID,
		evt.Type,
		evt.Source.Integration,
		evt.SourceKind,
		evt.Source.ConnectionID,
		nullableString(evt.Source.ConnectionName),
		nullableString(evt.Source.ExternalAccountID),
		lineageIntegration(evt.Lineage),
		lineageConnectionID(evt.Lineage),
		lineageConnectionName(evt.Lineage),
		lineageExternalAccountID(evt.Lineage),
		evt.ChainDepth,
		evt.Timestamp,
		[]byte(evt.Payload),
		nullableString(evt.SchemaFullName),
		schemaVersion,
		evt.WorkflowRunID,
		nullableString(evt.WorkflowNodeID),
	); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (d *DB) GetEvent(ctx context.Context, eventID uuid.UUID) (eventpkg.Event, error) {
	const query = `
		SELECT event_id, tenant_id, workflow_run_id, workflow_node_id, type, source, source_kind, source_connection_id, source_connection_name, source_external_account_id,
		       lineage_integration, lineage_connection_id, lineage_connection_name, lineage_external_account_id,
		       chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1
	`
	var evt eventpkg.Event
	err := scanEvent(d.db.QueryRowContext(ctx, query, eventID), &evt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return eventpkg.Event{}, fmt.Errorf("get event: %w", sql.ErrNoRows)
		}
		return eventpkg.Event{}, fmt.Errorf("get event: %w", err)
	}
	return evt, nil
}

func (d *DB) GetEventForTenant(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (eventpkg.Event, error) {
	const query = `
		SELECT event_id, tenant_id, workflow_run_id, workflow_node_id, type, source, source_kind, source_connection_id, source_connection_name, source_external_account_id,
		       lineage_integration, lineage_connection_id, lineage_connection_name, lineage_external_account_id,
		       chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1 AND tenant_id = $2
	`
	var evt eventpkg.Event
	err := scanEvent(d.db.QueryRowContext(ctx, query, eventID, tenantID), &evt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return eventpkg.Event{}, sql.ErrNoRows
		}
		return eventpkg.Event{}, fmt.Errorf("get event for tenant: %w", err)
	}
	return evt, nil
}

func (d *DB) ListEvents(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]eventpkg.Event, error) {
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
	if source != "" {
		query += fmt.Sprintf(" AND source = $%d", nextArg)
		args = append(args, source)
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
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []eventpkg.Event
	for rows.Next() {
		var evt eventpkg.Event
		if err := scanEvent(rows, &evt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func scanEvent(row scanner, evt *eventpkg.Event) error {
	var payload []byte
	var workflowRunID sql.NullString
	var workflowNodeID sql.NullString
	var sourceConnectionID sql.NullString
	var sourceConnectionName sql.NullString
	var sourceExternalAccountID sql.NullString
	var lineageIntegration sql.NullString
	var lineageConnectionID sql.NullString
	var lineageConnectionName sql.NullString
	var lineageExternalAccountID sql.NullString
	var schemaFullName sql.NullString
	var schemaVersion sql.NullInt64

	if err := row.Scan(
		&evt.EventID,
		&evt.TenantID,
		&workflowRunID,
		&workflowNodeID,
		&evt.Type,
		&evt.Source.Integration,
		&evt.SourceKind,
		&sourceConnectionID,
		&sourceConnectionName,
		&sourceExternalAccountID,
		&lineageIntegration,
		&lineageConnectionID,
		&lineageConnectionName,
		&lineageExternalAccountID,
		&evt.ChainDepth,
		&evt.Timestamp,
		&payload,
		&schemaFullName,
		&schemaVersion,
	); err != nil {
		return err
	}

	evt.Source.Kind = evt.SourceKind
	evt.WorkflowRunID = parseOptionalUUID(workflowRunID)
	evt.WorkflowNodeID = nullableStringValue(workflowNodeID)
	evt.Source.ConnectionID = parseOptionalUUID(sourceConnectionID)
	evt.Source.ConnectionName = nullableStringValue(sourceConnectionName)
	evt.Source.ExternalAccountID = nullableStringValue(sourceExternalAccountID)
	evt.Lineage = eventpkg.NormalizeLineage(&eventpkg.Lineage{
		Integration:       nullableStringValue(lineageIntegration),
		ConnectionID:      parseOptionalUUID(lineageConnectionID),
		ConnectionName:    nullableStringValue(lineageConnectionName),
		ExternalAccountID: nullableStringValue(lineageExternalAccountID),
	})
	evt.Payload = payload
	evt.SchemaFullName = nullableStringValue(schemaFullName)
	if schemaVersion.Valid {
		evt.SchemaVersion = int(schemaVersion.Int64)
	}
	return nil
}

func lineageIntegration(lineage *eventpkg.Lineage) any {
	if lineage == nil {
		return nil
	}
	return nullableString(lineage.Integration)
}

func lineageConnectionID(lineage *eventpkg.Lineage) any {
	if lineage == nil {
		return nil
	}
	return lineage.ConnectionID
}

func lineageConnectionName(lineage *eventpkg.Lineage) any {
	if lineage == nil {
		return nil
	}
	return nullableString(lineage.ConnectionName)
}

func lineageExternalAccountID(lineage *eventpkg.Lineage) any {
	if lineage == nil {
		return nil
	}
	return nullableString(lineage.ExternalAccountID)
}
