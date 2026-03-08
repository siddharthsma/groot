package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"groot/internal/apikey"
	"groot/internal/tenant"
)

func (d *DB) GetTenantByAPIKeyHash(ctx context.Context, apiKeyHash string) (tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		WHERE api_key_hash = $1
	`

	var record tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, apiKeyHash).Scan(&record.ID, &record.Name, &record.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenant.Tenant{}, tenant.ErrTenantNotFound
		}
		return tenant.Tenant{}, fmt.Errorf("get tenant by api key hash: %w", err)
	}

	return record, nil
}

func (d *DB) CreateAPIKey(ctx context.Context, record apikey.Record) (apikey.APIKey, error) {
	const query = `
		INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tenant_id, name, key_prefix, is_active, created_at, revoked_at, last_used_at
	`
	var key apikey.APIKey
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.Name, record.KeyPrefix, record.KeyHash, record.IsActive, record.CreatedAt).Scan(
		&key.ID, &key.TenantID, &key.Name, &key.KeyPrefix, &key.IsActive, &key.CreatedAt, &key.RevokedAt, &key.LastUsedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return apikey.APIKey{}, apikey.ErrDuplicatePrefix
		}
		return apikey.APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return key, nil
}

func (d *DB) ListAPIKeys(ctx context.Context, tenantID tenant.ID) ([]apikey.APIKey, error) {
	const query = `
		SELECT id, tenant_id, name, key_prefix, is_active, created_at, revoked_at, last_used_at
		FROM api_keys
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []apikey.APIKey
	for rows.Next() {
		var key apikey.APIKey
		if err := rows.Scan(&key.ID, &key.TenantID, &key.Name, &key.KeyPrefix, &key.IsActive, &key.CreatedAt, &key.RevokedAt, &key.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (d *DB) RevokeAPIKey(ctx context.Context, tenantID tenant.ID, id uuid.UUID, revokedAt time.Time) (apikey.APIKey, error) {
	const query = `
		UPDATE api_keys
		SET is_active = FALSE, revoked_at = $3
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, name, key_prefix, is_active, created_at, revoked_at, last_used_at
	`
	var key apikey.APIKey
	err := d.db.QueryRowContext(ctx, query, id, tenantID, revokedAt).Scan(&key.ID, &key.TenantID, &key.Name, &key.KeyPrefix, &key.IsActive, &key.CreatedAt, &key.RevokedAt, &key.LastUsedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apikey.APIKey{}, apikey.ErrNotFound
		}
		return apikey.APIKey{}, fmt.Errorf("revoke api key: %w", err)
	}
	return key, nil
}

func (d *DB) GetAPIKeyByPrefix(ctx context.Context, prefix string) (apikey.APIKeyRecord, error) {
	const query = `
		SELECT id, tenant_id, name, key_prefix, key_hash, is_active, created_at, revoked_at, last_used_at
		FROM api_keys
		WHERE key_prefix = $1
	`
	var record apikey.APIKeyRecord
	err := d.db.QueryRowContext(ctx, query, prefix).Scan(&record.ID, &record.TenantID, &record.Name, &record.KeyPrefix, &record.KeyHash, &record.IsActive, &record.CreatedAt, &record.RevokedAt, &record.LastUsedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apikey.APIKeyRecord{}, apikey.ErrNotFound
		}
		return apikey.APIKeyRecord{}, fmt.Errorf("get api key by prefix: %w", err)
	}
	return record, nil
}

func (d *DB) TouchAPIKeyLastUsed(ctx context.Context, id uuid.UUID, lastUsedAt time.Time) error {
	const query = `UPDATE api_keys SET last_used_at = $2 WHERE id = $1`
	if _, err := d.db.ExecContext(ctx, query, id, lastUsedAt); err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}
	return nil
}
