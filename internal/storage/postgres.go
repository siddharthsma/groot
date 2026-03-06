package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"groot/internal/agent"
	"groot/internal/apikey"
	"groot/internal/audit"
	iauth "groot/internal/auth"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/delivery"
	"groot/internal/functiondestination"
	"groot/internal/inboundroute"
	"groot/internal/schemas"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/tenant"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	db *sql.DB
}

func New(ctx context.Context, dsn string) (*DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(checkCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Check(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

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

func scanConnectorInstance(row scanner, instance *connectorinstance.Instance) error {
	var ownerTenantID sql.NullString
	if err := row.Scan(&instance.ID, &instance.TenantID, &ownerTenantID, &instance.ConnectorName, &instance.Scope, &instance.Status, &instance.Config, &instance.CreatedAt, &instance.UpdatedAt); err != nil {
		return err
	}
	instance.OwnerTenantID = parseOptionalUUID(ownerTenantID)
	return nil
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

func (d *DB) ListConnectorInstancesAdmin(ctx context.Context, tenantID *tenant.ID, connectorName, scope string) ([]connectorinstance.Instance, error) {
	query := `
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at, updated_at
		FROM connector_instances
		WHERE 1=1
	`
	args := []any{}
	nextArg := 1
	if tenantID != nil {
		query += fmt.Sprintf(" AND tenant_id = $%d", nextArg)
		args = append(args, *tenantID)
		nextArg++
	}
	if strings.TrimSpace(connectorName) != "" {
		query += fmt.Sprintf(" AND connector_name = $%d", nextArg)
		args = append(args, strings.TrimSpace(connectorName))
		nextArg++
	}
	if strings.TrimSpace(scope) != "" {
		query += fmt.Sprintf(" AND scope = $%d", nextArg)
		args = append(args, strings.TrimSpace(scope))
		nextArg++
	}
	query += " ORDER BY created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin connector instances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var instances []connectorinstance.Instance
	for rows.Next() {
		var instance connectorinstance.Instance
		if err := scanConnectorInstance(rows, &instance); err != nil {
			return nil, fmt.Errorf("scan admin connector instance: %w", err)
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (d *DB) GetInboundRouteByTenant(ctx context.Context, connectorName string, tenantID tenant.ID) (inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		WHERE connector_name = $1 AND tenant_id = $2
		ORDER BY created_at ASC
		LIMIT 1
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, connectorName, tenantID).Scan(&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inboundroute.Route{}, sql.ErrNoRows
		}
		return inboundroute.Route{}, fmt.Errorf("get inbound route by tenant: %w", err)
	}
	return route, nil
}

func (d *DB) UpdateInboundRouteByTenant(ctx context.Context, connectorName string, tenantID tenant.ID, routeKey string, connectorInstanceID *uuid.UUID) (inboundroute.Route, error) {
	const query = `
		UPDATE inbound_routes
		SET route_key = $3, connector_instance_id = $4
		WHERE connector_name = $1 AND tenant_id = $2
		RETURNING id, connector_name, route_key, tenant_id, connector_instance_id, created_at
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, connectorName, tenantID, routeKey, connectorInstanceID).Scan(
		&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inboundroute.Route{}, sql.ErrNoRows
		}
		if isUniqueViolation(err) {
			return inboundroute.Route{}, inboundroute.ErrDuplicateRoute
		}
		return inboundroute.Route{}, fmt.Errorf("update inbound route by tenant: %w", err)
	}
	return route, nil
}

func (d *DB) CreateInboundRoute(ctx context.Context, record inboundroute.Record) (inboundroute.Route, error) {
	const query = `
		INSERT INTO inbound_routes (id, connector_name, route_key, tenant_id, connector_instance_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, connector_name, route_key, tenant_id, connector_instance_id, created_at
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, record.ID, record.ConnectorName, record.RouteKey, record.TenantID, record.ConnectorInstanceID, record.CreatedAt).Scan(
		&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return inboundroute.Route{}, inboundroute.ErrDuplicateRoute
		}
		return inboundroute.Route{}, fmt.Errorf("create inbound route: %w", err)
	}
	return route, nil
}

func (d *DB) GetInboundRoute(ctx context.Context, connectorName, routeKey string) (inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		WHERE connector_name = $1 AND route_key = $2
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, connectorName, routeKey).Scan(
		&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inboundroute.Route{}, sql.ErrNoRows
		}
		return inboundroute.Route{}, fmt.Errorf("get inbound route: %w", err)
	}
	return route, nil
}

func (d *DB) ListInboundRoutes(ctx context.Context, tenantID tenant.ID) ([]inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query inbound routes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var routes []inboundroute.Route
	for rows.Next() {
		var route inboundroute.Route
		if err := rows.Scan(&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan inbound route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inbound routes: %w", err)
	}
	return routes, nil
}

func (d *DB) ListAllInboundRoutes(ctx context.Context) ([]inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all inbound routes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var routes []inboundroute.Route
	for rows.Next() {
		var route inboundroute.Route
		if err := rows.Scan(&route.ID, &route.ConnectorName, &route.RouteKey, &route.TenantID, &route.ConnectorInstanceID, &route.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan inbound route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all inbound routes: %w", err)
	}
	return routes, nil
}

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
	defer func() {
		_ = rows.Close()
	}()

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

func (d *DB) CreateAuditEvent(ctx context.Context, event audit.Event) error {
	const query = `
		INSERT INTO audit_events (id, tenant_id, actor_type, actor_id, actor_email, action, resource_type, resource_id, request_id, ip, user_agent, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	if _, err := d.db.ExecContext(ctx, query, event.ID, event.TenantID, event.ActorType, event.ActorID, event.ActorEmail, event.Action, event.ResourceType, event.ResourceID, event.RequestID, event.IP, event.UserAgent, jsonBytes(event.Metadata), event.CreatedAt); err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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

func (d *DB) CreateSubscription(ctx context.Context, record subscription.Record) (subscription.Subscription, error) {
	const query = `
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at, created_by_actor_type, created_by_actor_id, created_by_actor_email, updated_by_actor_type, updated_by_actor_id, updated_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $16, $17, $18)
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectorInstanceID, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, record.CreatedAt, actor.Type, actor.ID, actor.Email),
		&sub,
	)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) UpdateSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, record subscription.Record) (subscription.Subscription, error) {
	const query = `
		UPDATE subscriptions
		SET connected_app_id = $3,
		    destination_type = $4,
		    function_destination_id = $5,
		    connector_instance_id = $6,
		    operation = $7,
		    operation_params = $8,
		    filter_json = $9,
		    event_type = $10,
		    event_source = $11,
		    emit_success_event = $12,
		    emit_failure_event = $13,
		    status = $14,
		    updated_by_actor_type = $15,
		    updated_by_actor_id = $16,
		    updated_by_actor_email = $17
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectorInstanceID, record.Operation, []byte(record.OperationParams), jsonBytes(record.Filter), record.EventType, record.EventSource, record.EmitSuccessEvent, record.EmitFailureEvent, record.Status, actor.Type, actor.ID, actor.Email), &sub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) GetSubscription(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE id = $1 AND tenant_id = $2
	`
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID), &sub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) ListSubscriptionsAdmin(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]subscription.Subscription, error) {
	query := `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE 1=1
	`
	args := []any{}
	nextArg := 1
	if tenantID != nil {
		query += fmt.Sprintf(" AND tenant_id = $%d", nextArg)
		args = append(args, *tenantID)
		nextArg++
	}
	if strings.TrimSpace(eventType) != "" {
		query += fmt.Sprintf(" AND event_type = $%d", nextArg)
		args = append(args, strings.TrimSpace(eventType))
		nextArg++
	}
	if strings.TrimSpace(destinationType) != "" {
		query += fmt.Sprintf(" AND destination_type = $%d", nextArg)
		args = append(args, strings.TrimSpace(destinationType))
		nextArg++
	}
	query += " ORDER BY created_at ASC"
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan admin subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (d *DB) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		  AND event_type = $2
		  AND status = 'active'
		  AND (event_source IS NULL OR event_source = $3)
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventType, eventSource)
	if err != nil {
		return nil, fmt.Errorf("query matching subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := scanSubscription(rows, &sub); err != nil {
			return nil, fmt.Errorf("scan matching subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matching subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) SetSubscriptionStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (subscription.Subscription, error) {
	const query = `
		UPDATE subscriptions
		SET status = $3,
		    updated_by_actor_type = $4,
		    updated_by_actor_id = $5,
		    updated_by_actor_email = $6
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
	`
	actor := actorFromContext(ctx)
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, status, actor.Type, actor.ID, actor.Email), &sub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return subscription.Subscription{}, subscription.ErrSubscriptionNotFound
		}
		return subscription.Subscription{}, fmt.Errorf("set subscription status: %w", err)
	}
	return sub, nil
}

func (d *DB) CreateDeliveryJob(ctx context.Context, record delivery.JobRecord) (bool, error) {
	const query = `
		INSERT INTO delivery_jobs (id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
	`
	result, err := d.db.ExecContext(ctx, query, record.ID, record.TenantID, record.SubscriptionID, record.EventID, record.IsReplay, record.ReplayOfEventID, record.Status, record.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert delivery job: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delivery job rows affected: %w", err)
	}
	return rows == 1, nil
}

func (d *DB) SaveEvent(ctx context.Context, event stream.Event) error {
	const query = `
		INSERT INTO events (event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`
	var schemaVersion any
	if event.SchemaVersion > 0 {
		schemaVersion = event.SchemaVersion
	}
	if _, err := d.db.ExecContext(ctx, query, event.EventID, event.TenantID, event.Type, event.Source, event.SourceKind, event.ChainDepth, event.Timestamp, []byte(event.Payload), nullableString(event.SchemaFullName), schemaVersion); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (d *DB) GetEvent(ctx context.Context, eventID uuid.UUID) (stream.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1
	`
	var event stream.Event
	var payload []byte
	var schemaFullName sql.NullString
	var schemaVersion sql.NullInt64
	err := d.db.QueryRowContext(ctx, query, eventID).Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.SourceKind, &event.ChainDepth, &event.Timestamp, &payload, &schemaFullName, &schemaVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stream.Event{}, fmt.Errorf("get event: %w", sql.ErrNoRows)
		}
		return stream.Event{}, fmt.Errorf("get event: %w", err)
	}
	event.Payload = json.RawMessage(payload)
	event.SchemaFullName = nullableStringValue(schemaFullName)
	if schemaVersion.Valid {
		event.SchemaVersion = int(schemaVersion.Int64)
	}
	return event, nil
}

func (d *DB) GetEventForTenant(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID) (stream.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version
		FROM events
		WHERE event_id = $1 AND tenant_id = $2
	`
	var event stream.Event
	var payload []byte
	var schemaFullName sql.NullString
	var schemaVersion sql.NullInt64
	err := d.db.QueryRowContext(ctx, query, eventID, tenantID).Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.SourceKind, &event.ChainDepth, &event.Timestamp, &payload, &schemaFullName, &schemaVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stream.Event{}, sql.ErrNoRows
		}
		return stream.Event{}, fmt.Errorf("get event for tenant: %w", err)
	}
	event.Payload = json.RawMessage(payload)
	event.SchemaFullName = nullableStringValue(schemaFullName)
	if schemaVersion.Valid {
		event.SchemaVersion = int(schemaVersion.Int64)
	}
	return event, nil
}

func (d *DB) ListEvents(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]stream.Event, error) {
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

	var events []stream.Event
	for rows.Next() {
		var event stream.Event
		var payload []byte
		var schemaFullName sql.NullString
		var schemaVersion sql.NullInt64
		if err := rows.Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.SourceKind, &event.ChainDepth, &event.Timestamp, &payload, &schemaFullName, &schemaVersion); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		event.SchemaFullName = nullableStringValue(schemaFullName)
		if schemaVersion.Valid {
			event.SchemaVersion = int(schemaVersion.Int64)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func (d *DB) ListEventsAdmin(ctx context.Context, tenantID tenant.ID, eventType string, from, to *time.Time, limit int) ([]stream.Event, error) {
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
		return nil, fmt.Errorf("query admin events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var events []stream.Event
	for rows.Next() {
		var event stream.Event
		var payload []byte
		var schemaFullName sql.NullString
		var schemaVersion sql.NullInt64
		if err := rows.Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.SourceKind, &event.ChainDepth, &event.Timestamp, &payload, &schemaFullName, &schemaVersion); err != nil {
			return nil, fmt.Errorf("scan admin event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		event.SchemaFullName = nullableStringValue(schemaFullName)
		if schemaVersion.Valid {
			event.SchemaVersion = int(schemaVersion.Int64)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (d *DB) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, filter_json, event_type, event_source, emit_success_event, emit_failure_event, status, created_at
		FROM subscriptions
		WHERE id = $1
	`
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID), &sub)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("get subscription by id: %w", err)
	}
	return sub, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row scanner, sub *subscription.Subscription) error {
	var connectedAppID sql.NullString
	var functionDestinationID sql.NullString
	var connectorInstanceID sql.NullString
	var operation sql.NullString
	var operationParams []byte
	var filter []byte
	if err := row.Scan(
		&sub.ID,
		&sub.TenantID,
		&connectedAppID,
		&sub.DestinationType,
		&functionDestinationID,
		&connectorInstanceID,
		&operation,
		&operationParams,
		&filter,
		&sub.EventType,
		&sub.EventSource,
		&sub.EmitSuccessEvent,
		&sub.EmitFailureEvent,
		&sub.Status,
		&sub.CreatedAt,
	); err != nil {
		return err
	}

	sub.ConnectedAppID = parseOptionalUUID(connectedAppID)
	sub.FunctionDestinationID = parseOptionalUUID(functionDestinationID)
	sub.ConnectorInstanceID = parseOptionalUUID(connectorInstanceID)
	if operation.Valid {
		value := operation.String
		sub.Operation = &value
	}
	sub.OperationParams = json.RawMessage(operationParams)
	sub.Filter = json.RawMessage(filter)
	return nil
}

func parseOptionalUUID(value sql.NullString) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed, err := uuid.Parse(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func (d *DB) ClaimPendingJobs(ctx context.Context, limit int) ([]delivery.Job, error) {
	const query = `
		WITH claimed AS (
			SELECT id
			FROM delivery_jobs
			WHERE status = 'pending'
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE delivery_jobs dj
		SET status = 'in_progress'
		FROM claimed
		WHERE dj.id = claimed.id
		RETURNING dj.id, dj.tenant_id, dj.subscription_id, dj.event_id, dj.status, dj.attempts, dj.last_error, dj.completed_at, dj.created_at
	`
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim jobs tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := rows.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.CompletedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan claimed delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed delivery jobs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim jobs tx: %w", err)
	}
	return jobs, nil
}

func (d *DB) RequeueJob(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("requeue delivery job: %w", err)
	}
	return nil
}

func (d *DB) GetDeliveryJob(ctx context.Context, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, fmt.Errorf("get delivery job: %w", sql.ErrNoRows)
		}
		return delivery.Job{}, fmt.Errorf("get delivery job: %w", err)
	}
	return job, nil
}

func (d *DB) ListDeliveryJobs(ctx context.Context, tenantID tenant.ID, status string, subscriptionID, eventID *uuid.UUID, limit int) ([]delivery.Job, error) {
	query := `
		SELECT id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	nextArg := 2
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", nextArg)
		args = append(args, status)
		nextArg++
	}
	if subscriptionID != nil {
		query += fmt.Sprintf(" AND subscription_id = $%d", nextArg)
		args = append(args, *subscriptionID)
		nextArg++
	}
	if eventID != nil {
		query += fmt.Sprintf(" AND event_id = $%d", nextArg)
		args = append(args, *eventID)
		nextArg++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", nextArg)
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delivery jobs: %w", err)
	}
	return jobs, nil
}

func (d *DB) ListDeliveryJobsAdmin(ctx context.Context, tenantID tenant.ID, status string, from, to *time.Time, limit int) ([]delivery.Job, error) {
	query := `
		SELECT id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE tenant_id = $1
	`
	args := []any{tenantID}
	nextArg := 2
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", nextArg)
		args = append(args, status)
		nextArg++
	}
	if from != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", nextArg)
		args = append(args, *from)
		nextArg++
	}
	if to != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", nextArg)
		args = append(args, *to)
		nextArg++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", nextArg)
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan admin delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (d *DB) ListDeliveryJobsForEvent(ctx context.Context, tenantID tenant.ID, eventID uuid.UUID, limit int) ([]delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE tenant_id = $1 AND event_id = $2
		ORDER BY created_at ASC
		LIMIT $3
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventID, limit)
	if err != nil {
		return nil, fmt.Errorf("query delivery jobs for event: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := scanDeliveryJob(rows, &job); err != nil {
			return nil, fmt.Errorf("scan delivery job for event: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (d *DB) GetDeliveryJobForTenant(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1 AND tenant_id = $2
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID, tenantID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, delivery.ErrJobNotFound
		}
		return delivery.Job{}, fmt.Errorf("get delivery job for tenant: %w", err)
	}
	return job, nil
}

func (d *DB) ResetDeliveryJob(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', attempts = 0, last_error = NULL, completed_at = NULL
		WHERE id = $1 AND tenant_id = $2 AND status IN ('dead_letter', 'failed')
		RETURNING id, tenant_id, subscription_id, event_id, is_replay, replay_of_event_id, result_event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
	`
	var job delivery.Job
	err := scanDeliveryJob(d.db.QueryRowContext(ctx, query, jobID, tenantID), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, delivery.ErrRetryNotAllowed
		}
		return delivery.Job{}, fmt.Errorf("reset delivery job: %w", err)
	}
	return job, nil
}

func scanDeliveryJob(row scanner, job *delivery.Job) error {
	var replayOfEventID sql.NullString
	var resultEventID sql.NullString
	if err := row.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.IsReplay, &replayOfEventID, &resultEventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt); err != nil {
		return err
	}
	job.ReplayOfEventID = parseOptionalUUID(replayOfEventID)
	job.ResultEventID = parseOptionalUUID(resultEventID)
	return nil
}

func (d *DB) SetDeliveryJobAttempt(ctx context.Context, jobID uuid.UUID, attempt int) error {
	const query = `
		UPDATE delivery_jobs
		SET attempts = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, attempt); err != nil {
		return fmt.Errorf("set delivery job attempt: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobSucceeded(ctx context.Context, jobID uuid.UUID, completedAt time.Time, externalID *string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'succeeded', last_error = NULL, external_id = $3, last_status_code = $4, completed_at = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, completedAt, externalID, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobRetryableFailure(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'in_progress', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job retryable failure: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobDeadLetter(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'dead_letter', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job dead letter: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobFailed(ctx context.Context, jobID uuid.UUID, lastError string, lastStatusCode *int) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'failed', last_error = $2, last_status_code = $3
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError, lastStatusCode); err != nil {
		return fmt.Errorf("mark delivery job failed: %w", err)
	}
	return nil
}

func (d *DB) SaveResultEvent(ctx context.Context, jobID uuid.UUID, event stream.Event) (bool, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin result event tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const insertEvent = `
		INSERT INTO events (event_id, tenant_id, type, source, source_kind, chain_depth, timestamp, payload, schema_full_name, schema_version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`
	var schemaVersion any
	if event.SchemaVersion > 0 {
		schemaVersion = event.SchemaVersion
	}
	if _, err := tx.ExecContext(ctx, insertEvent, event.EventID, event.TenantID, event.Type, event.Source, event.SourceKind, event.ChainDepth, event.Timestamp, []byte(event.Payload), nullableString(event.SchemaFullName), schemaVersion); err != nil {
		return false, fmt.Errorf("insert result event: %w", err)
	}

	const updateJob = `
		UPDATE delivery_jobs
		SET result_event_id = $2
		WHERE id = $1 AND result_event_id IS NULL
	`
	result, err := tx.ExecContext(ctx, updateJob, jobID, event.EventID)
	if err != nil {
		return false, fmt.Errorf("link result event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("result event rows affected: %w", err)
	}
	if rows == 0 {
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit result event tx: %w", err)
	}
	return true, nil
}

func (d *DB) CreateAgentRun(ctx context.Context, record agent.RunRecord) (agent.Run, error) {
	const query = `
		INSERT INTO agent_runs (id, tenant_id, input_event_id, subscription_id, status, steps, started_at, created_by_actor_type, created_by_actor_id, created_by_actor_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, tenant_id, input_event_id, subscription_id, status, steps, started_at, completed_at, last_error
	`
	actor := actorFromContext(ctx)
	var run agent.Run
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.InputEventID, record.SubscriptionID, record.Status, record.Steps, record.StartedAt, actor.Type, actor.ID, actor.Email).Scan(
		&run.ID,
		&run.TenantID,
		&run.InputEventID,
		&run.SubscriptionID,
		&run.Status,
		&run.Steps,
		&run.StartedAt,
		&run.CompletedAt,
		&run.LastError,
	)
	if err != nil {
		return agent.Run{}, fmt.Errorf("insert agent run: %w", err)
	}
	return run, nil
}

func (d *DB) CreateAgentStep(ctx context.Context, record agent.StepRecord) error {
	const query = `
		INSERT INTO agent_steps (id, agent_run_id, step_num, kind, tool_name, tool_args, tool_result, llm_provider, llm_model, usage, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	if _, err := d.db.ExecContext(
		ctx,
		query,
		record.ID,
		record.AgentRunID,
		record.StepNum,
		record.Kind,
		nullableString(optionalStringValue(record.ToolName)),
		jsonBytes(record.ToolArgs),
		jsonBytes(record.ToolResult),
		nullableString(optionalStringValue(record.LLMProvider)),
		nullableString(optionalStringValue(record.LLMModel)),
		jsonBytes(record.Usage),
		record.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert agent step: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentRunSucceeded(ctx context.Context, runID uuid.UUID, steps int, completedAt time.Time) error {
	const query = `
		UPDATE agent_runs
		SET status = 'succeeded', steps = $2, completed_at = $3, last_error = NULL
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, steps, completedAt); err != nil {
		return fmt.Errorf("mark agent run succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkAgentRunFailed(ctx context.Context, runID uuid.UUID, steps int, completedAt time.Time, lastError string) error {
	const query = `
		UPDATE agent_runs
		SET status = 'failed', steps = $2, completed_at = $3, last_error = $4
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, runID, steps, completedAt, lastError); err != nil {
		return fmt.Errorf("mark agent run failed: %w", err)
	}
	return nil
}

func (d *DB) UpsertEventSchema(ctx context.Context, record schemas.Record) error {
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

func (d *DB) ListEventSchemas(ctx context.Context) ([]schemas.Schema, error) {
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
	var out []schemas.Schema
	for rows.Next() {
		var schema schemas.Schema
		if err := rows.Scan(&schema.ID, &schema.EventType, &schema.Version, &schema.FullName, &schema.Source, &schema.SourceKind, &schema.SchemaJSON, &schema.CreatedAt, &schema.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan event schema: %w", err)
		}
		out = append(out, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event schemas: %w", err)
	}
	return out, nil
}

func (d *DB) GetEventSchema(ctx context.Context, fullName string) (schemas.Schema, error) {
	const query = `
		SELECT id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at
		FROM event_schemas
		WHERE full_name = $1
	`
	var schema schemas.Schema
	if err := d.db.QueryRowContext(ctx, query, fullName).Scan(&schema.ID, &schema.EventType, &schema.Version, &schema.FullName, &schema.Source, &schema.SourceKind, &schema.SchemaJSON, &schema.CreatedAt, &schema.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schemas.Schema{}, sql.ErrNoRows
		}
		return schemas.Schema{}, fmt.Errorf("get event schema: %w", err)
	}
	return schema, nil
}

func (d *DB) GetLatestEventSchema(ctx context.Context, eventType string) (schemas.Schema, error) {
	const query = `
		SELECT id, event_type, version, full_name, source, source_kind, schema_json, created_at, updated_at
		FROM event_schemas
		WHERE event_type = $1
		ORDER BY version DESC
		LIMIT 1
	`
	var schema schemas.Schema
	if err := d.db.QueryRowContext(ctx, query, eventType).Scan(&schema.ID, &schema.EventType, &schema.Version, &schema.FullName, &schema.Source, &schema.SourceKind, &schema.SchemaJSON, &schema.CreatedAt, &schema.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schemas.Schema{}, sql.ErrNoRows
		}
		return schemas.Schema{}, fmt.Errorf("get latest event schema: %w", err)
	}
	return schema, nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func jsonBytes(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

type actorMetadata struct {
	Type  any
	ID    any
	Email any
}

func actorFromContext(ctx context.Context) actorMetadata {
	principal, ok := iauth.PrincipalFromContext(ctx)
	if !ok {
		return actorMetadata{}
	}
	return actorMetadata{
		Type:  nullableString(principal.Actor.Type),
		ID:    nullableString(principal.Actor.ID),
		Email: nullableString(principal.Actor.Email),
	}
}
