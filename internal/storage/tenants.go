package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"groot/internal/tenant"
)

func (d *DB) CreateTenant(ctx context.Context, record tenant.TenantRecord) (tenant.Tenant, error) {
	const query = `
		INSERT INTO tenants (id, name, api_key_hash, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, created_at
	`

	var created tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, record.ID, record.Name, record.APIKeyHash, record.CreatedAt).Scan(
		&created.ID,
		&created.Name,
		&created.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return tenant.Tenant{}, tenant.ErrTenantNameExists
		}
		return tenant.Tenant{}, fmt.Errorf("insert tenant: %w", err)
	}

	return created, nil
}

func (d *DB) ListTenants(ctx context.Context) ([]tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		WHERE id <> '00000000-0000-0000-0000-000000000000'
		ORDER BY created_at ASC
	`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query tenants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tenants []tenant.Tenant
	for rows.Next() {
		var record tenant.Tenant
		if err := rows.Scan(&record.ID, &record.Name, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}

	return tenants, nil
}

func (d *DB) GetTenant(ctx context.Context, id tenant.ID) (tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		WHERE id = $1
	`

	var record tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, id).Scan(&record.ID, &record.Name, &record.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenant.Tenant{}, tenant.ErrTenantNotFound
		}
		return tenant.Tenant{}, fmt.Errorf("get tenant: %w", err)
	}

	return record, nil
}

func (d *DB) UpdateTenantName(ctx context.Context, id tenant.ID, name string) (tenant.Tenant, error) {
	const query = `
		UPDATE tenants
		SET name = $2
		WHERE id = $1
		RETURNING id, name, created_at
	`
	var record tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, id, name).Scan(&record.ID, &record.Name, &record.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return tenant.Tenant{}, tenant.ErrTenantNameExists
		}
		if errors.Is(err, sql.ErrNoRows) {
			return tenant.Tenant{}, tenant.ErrTenantNotFound
		}
		return tenant.Tenant{}, fmt.Errorf("update tenant name: %w", err)
	}
	return record, nil
}
