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
		INSERT INTO events (event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`
	var schemaVersion any
	if evt.SchemaVersion > 0 {
		schemaVersion = evt.SchemaVersion
	}
	if _, err := d.db.ExecContext(ctx, query, evt.EventID, evt.TenantID, evt.Type, evt.Source, evt.SourceKind, evt.ChainDepth, evt.Timestamp, []byte(evt.Payload), nullableString(evt.SchemaFullName), schemaVersion); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (d *DB) GetEvent(ctx context.Context, eventID uuid.UUID) (eventpkg.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1
	`
	var evt eventpkg.Event
	var payload []byte
	var schemaFullName sql.NullString
	var schemaVersion sql.NullInt64
	err := d.db.QueryRowContext(ctx, query, eventID).Scan(&evt.EventID, &evt.TenantID, &evt.Type, &evt.Source, &evt.SourceKind, &evt.ChainDepth, &evt.Timestamp, &payload, &schemaFullName, &schemaVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return eventpkg.Event{}, fmt.Errorf("get event: %w", sql.ErrNoRows)
		}
		return eventpkg.Event{}, fmt.Errorf("get event: %w", err)
	}
	evt.Payload = payload
	evt.SchemaFullName = nullableStringValue(schemaFullName)
	if schemaVersion.Valid {
		evt.SchemaVersion = int(schemaVersion.Int64)
	}
	return evt, nil
}

func (d *DB) GetEventForTenant(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (eventpkg.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1 AND tenant_id = $2
	`
	var evt eventpkg.Event
	var payload []byte
	var schemaFullName sql.NullString
	var schemaVersion sql.NullInt64
	err := d.db.QueryRowContext(ctx, query, eventID, tenantID).Scan(&evt.EventID, &evt.TenantID, &evt.Type, &evt.Source, &evt.SourceKind, &evt.ChainDepth, &evt.Timestamp, &payload, &schemaFullName, &schemaVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return eventpkg.Event{}, sql.ErrNoRows
		}
		return eventpkg.Event{}, fmt.Errorf("get event for tenant: %w", err)
	}
	evt.Payload = payload
	evt.SchemaFullName = nullableStringValue(schemaFullName)
	if schemaVersion.Valid {
		evt.SchemaVersion = int(schemaVersion.Int64)
	}
	return evt, nil
}

func (d *DB) ListEvents(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]eventpkg.Event, error) {
	query := `
		SELECT event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version
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
		var payload []byte
		var schemaFullName sql.NullString
		var schemaVersion sql.NullInt64
		if err := rows.Scan(&evt.EventID, &evt.TenantID, &evt.Type, &evt.Source, &evt.SourceKind, &evt.ChainDepth, &evt.Timestamp, &payload, &schemaFullName, &schemaVersion); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		evt.Payload = payload
		evt.SchemaFullName = nullableStringValue(schemaFullName)
		if schemaVersion.Valid {
			evt.SchemaVersion = int(schemaVersion.Int64)
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}
