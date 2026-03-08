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
	"groot/internal/connectorinstance"
	"groot/internal/tenant"
)

func (d *DB) EnsureConnectorInstance(ctx context.Context, tenantID tenant.ID, connectorName string, createdAt time.Time) error {
	const query = `
		INSERT INTO connector_instances (id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at)
		VALUES ($1, $2, $2, $3, 'tenant', 'enabled', '{}'::jsonb, $4)
		ON CONFLICT (tenant_id, connector_name) DO NOTHING
	`
	if _, err := d.db.ExecContext(ctx, query, uuid.New(), tenantID, connectorName, createdAt); err != nil {
		return fmt.Errorf("ensure connector instance: %w", err)
	}
	return nil
}

func (d *DB) CreateConnectorInstance(ctx context.Context, record connectorinstance.Record) (connectorinstance.Instance, error) {
	const query = `
		INSERT INTO connector_instances (id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $9, $10, $11)
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var instance connectorinstance.Instance
	err := scanConnectorInstance(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.OwnerTenantID, record.ConnectorName, record.Scope, record.Status, []byte(record.Config), record.CreatedAt, actor.Type, actor.ID, actor.Email),
		&instance,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return connectorinstance.Instance{}, connectorinstance.ErrDuplicateInstance
		}
		return connectorinstance.Instance{}, fmt.Errorf("insert connector instance: %w", err)
	}
	return instance, nil
}

func (d *DB) ListConnectorInstances(ctx context.Context, tenantID tenant.ID) ([]connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE owner_tenant_id = $1 OR scope = 'global'
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query connector instances: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var instances []connectorinstance.Instance
	for rows.Next() {
		var instance connectorinstance.Instance
		if err := scanConnectorInstance(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan connector instance: %w", err)
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connector instances: %w", err)
	}
	return instances, nil
}

func (d *DB) ListAllConnectorInstances(ctx context.Context) ([]connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all connector instances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var instances []connectorinstance.Instance
	for rows.Next() {
		var instance connectorinstance.Instance
		if err := scanConnectorInstance(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan connector instance: %w", err)
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all connector instances: %w", err)
	}
	return instances, nil
}

func (d *DB) GetConnectorInstance(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE id = $1 AND (owner_tenant_id = $2 OR scope = 'global')
	`
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, id, tenantID), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("get connector instance: %w", err)
	}
	return instance, nil
}

func (d *DB) GetTenantConnectorInstanceByName(ctx context.Context, tenantID tenant.ID, connectorName string) (connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE owner_tenant_id = $1 AND connector_name = $2 AND scope = 'tenant'
	`
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, tenantID, connectorName), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("get tenant connector instance by name: %w", err)
	}
	return instance, nil
}

func (d *DB) GetGlobalConnectorInstanceByName(ctx context.Context, connectorName string) (connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE connector_name = $1 AND scope = 'global'
		ORDER BY created_at ASC
		LIMIT 1
	`
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, connectorName), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("get global connector instance by name: %w", err)
	}
	return instance, nil
}

func (d *DB) UpdateConnectorInstanceConfig(ctx context.Context, tenantID tenant.ID, connectorName string, config json.RawMessage) (connectorinstance.Instance, error) {
	const query = `
		UPDATE connector_instances
		SET config_json = $3,
		    updated_by_actor_type = $4,
		    updated_by_actor_id = $5,
		    updated_by_actor_email = $6,
		    updated_at = NOW()
		WHERE owner_tenant_id = $1 AND connector_name = $2 AND scope = 'tenant'
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
	`
	actor := actorFromContext(ctx)
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, tenantID, connectorName, []byte(config), actor.Type, actor.ID, actor.Email), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("update connector instance config: %w", err)
	}
	return instance, nil
}

func (d *DB) GetConnectorInstanceByID(ctx context.Context, id uuid.UUID) (connectorinstance.Instance, error) {
	const query = `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE id = $1
	`
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, id), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("get connector instance by id: %w", err)
	}
	return instance, nil
}

func (d *DB) UpdateConnectorInstanceByID(ctx context.Context, id uuid.UUID, config json.RawMessage) (connectorinstance.Instance, error) {
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
	var instance connectorinstance.Instance
	err := scanConnectorInstance(d.db.QueryRowContext(ctx, query, id, []byte(config), actor.Type, actor.ID, actor.Email), &instance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorinstance.Instance{}, connectorinstance.ErrNotFound
		}
		return connectorinstance.Instance{}, fmt.Errorf("update connector instance by id: %w", err)
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

func scanConnectorInstance(row scanner, instance *connectorinstance.Instance) error {
	var ownerTenantID sql.NullString
	if err := row.Scan(&instance.ID, &instance.TenantID, &ownerTenantID, &instance.ConnectorName, &instance.Scope, &instance.Status, &instance.Config, &instance.CreatedAt, &instance.UpdatedAt); err != nil {
		return err
	}
	instance.OwnerTenantID = parseOptionalUUID(ownerTenantID)
	return nil
}
