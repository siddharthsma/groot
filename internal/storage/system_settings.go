package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) GetSystemSetting(ctx context.Context, key string) (string, error) {
	const query = `
		SELECT value
		FROM system_settings
		WHERE key = $1
	`
	var value string
	err := d.db.QueryRowContext(ctx, query, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", sql.ErrNoRows
		}
		return "", fmt.Errorf("get system setting: %w", err)
	}
	return value, nil
}

func (d *DB) UpsertSystemSetting(ctx context.Context, key, value string) error {
	const query = `
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_at = NOW()
	`
	if _, err := d.db.ExecContext(ctx, query, key, value); err != nil {
		return fmt.Errorf("upsert system setting: %w", err)
	}
	return nil
}
