package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"groot/internal/schema"
)

func (d *DB) UpsertEventSchema(ctx context.Context, record schema.Record) error {
	const query = `
		INSERT INTO event_schemas (id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW(), $8, $9, $10, $8, $9, $10)
		ON CONFLICT (full_name) DO UPDATE
		SET event_type = EXCLUDED.event_type,
		    version = EXCLUDED.version,
		    source = EXCLUDED.source,
		    source_kind = EXCLUDED.source_kind,
		    schema_json = EXCLUDED.schema_json,
		    updated_by_actor_type = $8,
		    updated_by_actor_id = $9,
		    updated_by_actor_email = $10,
		    updated_at = NOW()
	`
	actor := actorFromContext(ctx)
	if _, err := d.db.ExecContext(ctx, query, record.ID, record.EventType, record.Version, record.FullName, record.Source, record.SourceKind, []byte(record.SchemaJSON), actor.Type, actor.ID, actor.Email); err != nil {
		return fmt.Errorf("upsert event schema: %w", err)
	}
	return nil
}

func (d *DB) ListEventSchemas(ctx context.Context) ([]schema.Schema, error) {
	const query = `
		SELECT id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at
		FROM event_schemas
		ORDER BY event_type ASC, version ASC
	`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query event schemas: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []schema.Schema
	for rows.Next() {
		var record schema.Schema
		if err := rows.Scan(&record.ID, &record.EventType, &record.Version, &record.FullName, &record.Source, &record.SourceKind, &record.SchemaJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan event schema: %w", err)
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event schemas: %w", err)
	}
	return out, nil
}

func (d *DB) GetEventSchema(ctx context.Context, fullName string) (schema.Schema, error) {
	const query = `
		SELECT id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at
		FROM event_schemas
		WHERE full_name = $1
	`
	var record schema.Schema
	if err := d.db.QueryRowContext(ctx, query, fullName).Scan(&record.ID, &record.EventType, &record.Version, &record.FullName, &record.Source, &record.SourceKind, &record.SchemaJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schema.Schema{}, sql.ErrNoRows
		}
		return schema.Schema{}, fmt.Errorf("get event schema: %w", err)
	}
	return record, nil
}

func (d *DB) GetLatestEventSchema(ctx context.Context, eventType string) (schema.Schema, error) {
	const query = `
		SELECT id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at
		FROM event_schemas
		WHERE event_type = $1
		ORDER BY version DESC
		LIMIT 1
	`
	var record schema.Schema
	if err := d.db.QueryRowContext(ctx, query, eventType).Scan(&record.ID, &record.EventType, &record.Version, &record.FullName, &record.Source, &record.SourceKind, &record.SchemaJSON, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schema.Schema{}, sql.ErrNoRows
		}
		return schema.Schema{}, fmt.Errorf("get latest event schema: %w", err)
	}
	return record, nil
}
