package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectedapp"
	"groot/internal/connection"
	"groot/internal/tenant"
)

func (d *DB) EnsureConnection(ctx context.Context, tenantID tenant.ID, integrationName string, createdAt time.Time) error {
	const query = `
		INSERT INTO connector_instances (id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at)
		SELECT $1, $2, $2, $3, 'tenant', 'enabled', '{}'::jsonb, $4
		WHERE NOT EXISTS (
			SELECT 1
			FROM connector_instances
			WHERE owner_tenant_id = $2
			  AND connector_name = $3
			  AND scope = 'tenant'
		)
	`
	if _, err := d.db.ExecContext(ctx, query, uuid.New(), tenantID, integrationName, createdAt); err != nil {
		return fmt.Errorf("ensure connection: %w", err)
	}
	return nil
}

func (d *DB) CreateConnection(ctx context.Context, record connection.Record) (connection.Instance, error) {
	const query = `
		INSERT INTO connector_instances (id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $9, $10, $11)
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var instance connection.Instance
	err := scanConnection(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.OwnerTenantID, record.IntegrationName, record.Scope, record.Status, []byte(record.Config), record.CreatedAt, actor.Type, actor.ID, actor.Email),
		&instance,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return connection.Instance{}, connection.ErrDuplicateInstance
		}
		return connection.Instance{}, fmt.Errorf("insert connection: %w", err)
	}
	return instance, nil
}

func (d *DB) ListConnections(ctx context.Context, tenantID tenant.ID) ([]connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE owner_tenant_id = $1 OR scope = 'global'
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var instances []connection.Instance
	for rows.Next() {
		var instance connection.Instance
		if err := scanConnection(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connections: %w", err)
	}
	return instances, nil
}

func (d *DB) ListAllConnections(ctx context.Context) ([]connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var instances []connection.Instance
	for rows.Next() {
		var instance connection.Instance
		if err := scanConnection(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all connections: %w", err)
	}
	return instances, nil
}

func (d *DB) GetConnection(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE id = $1 AND (owner_tenant_id = $2 OR scope = 'global')
	`
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, id, tenantID), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("get connection: %w", err)
	}
	return instance, nil
}

func (d *DB) GetTenantConnectionByName(ctx context.Context, tenantID tenant.ID, integrationName string) (connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE owner_tenant_id = $1 AND connector_name = $2 AND scope = 'tenant'
		ORDER BY created_at ASC
		LIMIT 1
	`
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, tenantID, integrationName), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("get tenant connection by name: %w", err)
	}
	return instance, nil
}

func (d *DB) GetGlobalConnectionByName(ctx context.Context, integrationName string) (connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE connector_name = $1 AND scope = 'global'
		ORDER BY created_at ASC
		LIMIT 1
	`
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, integrationName), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("get global connection by name: %w", err)
	}
	return instance, nil
}

func (d *DB) UpdateConnectionConfig(ctx context.Context, tenantID tenant.ID, integrationName string, config json.RawMessage) (connection.Instance, error) {
	const query = `
		UPDATE connector_instances
		SET config_json = $3,
		    updated_by_actor_type = $4,
		    updated_by_actor_id = $5,
		    updated_by_actor_email = $6,
		    updated_at = NOW()
		WHERE id = (
			SELECT id
			FROM connector_instances
			WHERE owner_tenant_id = $1 AND connector_name = $2 AND scope = 'tenant'
			ORDER BY created_at ASC
			LIMIT 1
		)
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, tenantID, integrationName, []byte(config), actor.Type, actor.ID, actor.Email), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("update connection config: %w", err)
	}
	return instance, nil
}

func (d *DB) GetConnectionByID(ctx context.Context, id uuid.UUID) (connection.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE id = $1
	`
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, id), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("get connection by id: %w", err)
	}
	return instance, nil
}

func (d *DB) UpdateConnectionByID(ctx context.Context, id uuid.UUID, config json.RawMessage) (connection.Instance, error) {
	const query = `
		UPDATE connector_instances
		SET config_json = $2,
		    updated_by_actor_type = $3,
		    updated_by_actor_id = $4,
		    updated_by_actor_email = $5,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var instance connection.Instance
	err := scanConnection(d.db.QueryRowContext(ctx, query, id, []byte(config), actor.Type, actor.ID, actor.Email), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connection.Instance{}, connection.ErrNotFound
		}
		return connection.Instance{}, fmt.Errorf("update connection by id: %w", err)
	}
	return instance, nil
}

func (d *DB) CreateConnectedApp(ctx context.Context, record connectedapp.Record) (connectedapp.App, error) {
	const query = `
		INSERT INTO connected_apps (id, tenant_id, name, destination_url, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, name, destination_url, created_at
	`

	var app connectedapp.App
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.Name, record.DestinationURL, record.CreatedAt).Scan(
		&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt,
	)
	if err != nil {
		return connectedapp.App{}, fmt.Errorf("insert connected app: %w", err)
	}
	return app, nil
}

func (d *DB) ListConnectedApps(ctx context.Context, tenantID tenant.ID) ([]connectedapp.App, error) {
	const query = `
		SELECT id, tenant_id, name, destination_url, created_at
		FROM connected_apps
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query connected apps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var apps []connectedapp.App
	for rows.Next() {
		var app connectedapp.App
		if err := rows.Scan(&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connected app: %w", err)
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connected apps: %w", err)
	}
	return apps, nil
}

func (d *DB) GetConnectedApp(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (connectedapp.App, error) {
	const query = `
		SELECT id, tenant_id, name, destination_url, created_at
		FROM connected_apps
		WHERE id = $1 AND tenant_id = $2
	`
	var app connectedapp.App
	err := d.db.QueryRowContext(ctx, query, appID, tenantID).Scan(&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectedapp.App{}, connectedapp.ErrNotFound
		}
		return connectedapp.App{}, fmt.Errorf("get connected app: %w", err)
	}
	return app, nil
}

func scanConnection(row scanner, instance *connection.Instance) error {
	var ownerTenantID sql.NullString
	if err := row.Scan(&instance.ID, &instance.TenantID, &ownerTenantID, &instance.IntegrationName, &instance.Scope, &instance.Status, &instance.Config, &instance.CreatedAt, &instance.UpdatedAt); err != nil {
		return err
	}
	instance.OwnerTenantID = parseOptionalUUID(ownerTenantID)
	return nil
}
