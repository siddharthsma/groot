package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

func (d *DB) CreateFunctionDestination(ctx context.Context, record functiondestination.Record) (functiondestination.Destination, error) {
	const query = `
		INSERT INTO function_destinations (id, tenant_id, name, url, secret, timeout_seconds, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tenant_id, name, url, secret, timeout_seconds, created_at
	`
	var destination functiondestination.Destination
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.Name, record.URL, record.Secret, record.TimeoutSeconds, record.CreatedAt).Scan(
		&destination.ID, &destination.TenantID, &destination.Name, &destination.URL, &destination.Secret, &destination.TimeoutSeconds, &destination.CreatedAt,
	)
	if err != nil {
		return functiondestination.Destination{}, fmt.Errorf("insert function destination: %w", err)
	}
	return destination, nil
}

func (d *DB) ListFunctionDestinations(ctx context.Context, tenantID tenant.ID) ([]functiondestination.Destination, error) {
	const query = `
		SELECT id, tenant_id, name, url, secret, timeout_seconds, created_at
		FROM function_destinations
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query function destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var destinations []functiondestination.Destination
	for rows.Next() {
		var destination functiondestination.Destination
		if err := rows.Scan(&destination.ID, &destination.TenantID, &destination.Name, &destination.URL, &destination.Secret, &destination.TimeoutSeconds, &destination.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan function destination: %w", err)
		}
		destinations = append(destinations, destination)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate function destinations: %w", err)
	}
	return destinations, nil
}

func (d *DB) GetFunctionDestination(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (functiondestination.Destination, error) {
	const query = `
		SELECT id, tenant_id, name, url, secret, timeout_seconds, created_at
		FROM function_destinations
		WHERE id = $1 AND tenant_id = $2
	`
	var destination functiondestination.Destination
	err := d.db.QueryRowContext(ctx, query, id, tenantID).Scan(
		&destination.ID, &destination.TenantID, &destination.Name, &destination.URL, &destination.Secret, &destination.TimeoutSeconds, &destination.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return functiondestination.Destination{}, functiondestination.ErrNotFound
		}
		return functiondestination.Destination{}, fmt.Errorf("get function destination: %w", err)
	}
	return destination, nil
}

func (d *DB) DeleteFunctionDestination(ctx context.Context, tenantID tenant.ID, id uuid.UUID) error {
	const inUseQuery = `
		SELECT 1
		FROM subscriptions
		WHERE tenant_id = $1
		  AND function_destination_id = $2
		  AND destination_type = 'function'
		  AND status = 'active'
		LIMIT 1
	`
	var exists int
	err := d.db.QueryRowContext(ctx, inUseQuery, tenantID, id).Scan(&exists)
	if err == nil {
		return functiondestination.ErrInUse
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check function destination usage: %w", err)
	}

	const deleteQuery = `
		DELETE FROM function_destinations
		WHERE id = $1 AND tenant_id = $2
	`
	result, err := d.db.ExecContext(ctx, deleteQuery, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete function destination: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("function destination rows affected: %w", err)
	}
	if rows == 0 {
		return functiondestination.ErrNotFound
	}
	return nil
}
