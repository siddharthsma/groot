package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/delivery"
	"groot/internal/functiondestination"
	"groot/internal/inboundroute"
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
		INSERT INTO connector_instances (id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at
	`
	var instance connectorinstance.Instance
	err := scanConnectorInstance(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.OwnerTenantID, record.ConnectorName, record.Scope, record.Status, []byte(record.Config), record.CreatedAt),
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
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at
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
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at
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
		SELECT id, tenant_id, owner_tenant_id, connector_name, scope, status, config_json, created_at
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

func scanConnectorInstance(row scanner, instance *connectorinstance.Instance) error {
	var ownerTenantID sql.NullString
	if err := row.Scan(&instance.ID, &instance.TenantID, &ownerTenantID, &instance.ConnectorName, &instance.Scope, &instance.Status, &instance.Config, &instance.CreatedAt); err != nil {
		return err
	}
	instance.OwnerTenantID = parseOptionalUUID(ownerTenantID)
	return nil
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
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at
	`
	var sub subscription.Subscription
	err := scanSubscription(
		d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.ConnectedAppID, record.DestinationType, record.FunctionDestinationID, record.ConnectorInstanceID, record.Operation, []byte(record.OperationParams), record.EventType, record.EventSource, record.Status, record.CreatedAt),
		&sub,
	)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at
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

func (d *DB) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at
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
		SET status = $3
		WHERE id = $1 AND tenant_id = $2
		RETURNING id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at
	`
	var sub subscription.Subscription
	err := scanSubscription(d.db.QueryRowContext(ctx, query, subscriptionID, tenantID, status), &sub)
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
		INSERT INTO delivery_jobs (id, tenant_id, subscription_id, event_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id, subscription_id) DO NOTHING
	`
	result, err := d.db.ExecContext(ctx, query, record.ID, record.TenantID, record.SubscriptionID, record.EventID, record.Status, record.CreatedAt)
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
		INSERT INTO events (event_id, tenant_id, type, source, timestamp, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	if _, err := d.db.ExecContext(ctx, query, event.EventID, event.TenantID, event.Type, event.Source, event.Timestamp, []byte(event.Payload)); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (d *DB) GetEvent(ctx context.Context, eventID uuid.UUID) (stream.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, timestamp, payload
		FROM events
		WHERE event_id = $1
	`
	var event stream.Event
	var payload []byte
	err := d.db.QueryRowContext(ctx, query, eventID).Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.Timestamp, &payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stream.Event{}, fmt.Errorf("get event: %w", sql.ErrNoRows)
		}
		return stream.Event{}, fmt.Errorf("get event: %w", err)
	}
	event.Payload = json.RawMessage(payload)
	return event, nil
}

func (d *DB) ListEvents(ctx context.Context, tenantID tenant.ID, eventType, source string, from, to *time.Time, limit int) ([]stream.Event, error) {
	query := `
		SELECT event_id, tenant_id, type, source, timestamp, payload
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
		if err := rows.Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.Timestamp, &payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func (d *DB) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, destination_type, function_destination_id, connector_instance_id, operation, operation_params, event_type, event_source, status, created_at
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
	if err := row.Scan(
		&sub.ID,
		&sub.TenantID,
		&connectedAppID,
		&sub.DestinationType,
		&functionDestinationID,
		&connectorInstanceID,
		&operation,
		&operationParams,
		&sub.EventType,
		&sub.EventSource,
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
		SELECT id, tenant_id, subscription_id, event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1
	`
	var job delivery.Job
	err := d.db.QueryRowContext(ctx, query, jobID).Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt)
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
		SELECT id, tenant_id, subscription_id, event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
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
		if err := rows.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delivery jobs: %w", err)
	}
	return jobs, nil
}

func (d *DB) GetDeliveryJobForTenant(ctx context.Context, tenantID tenant.ID, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1 AND tenant_id = $2
	`
	var job delivery.Job
	err := d.db.QueryRowContext(ctx, query, jobID, tenantID).Scan(
		&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt,
	)
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
		SET status = 'pending', attempts = 0, last_error = NULL, external_id = NULL, last_status_code = NULL, completed_at = NULL
		WHERE id = $1 AND tenant_id = $2 AND status IN ('dead_letter', 'failed')
		RETURNING id, tenant_id, subscription_id, event_id, status, attempts, last_error, external_id, last_status_code, completed_at, created_at
	`
	var job delivery.Job
	err := d.db.QueryRowContext(ctx, query, jobID, tenantID).Scan(
		&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.ExternalID, &job.LastStatusCode, &job.CompletedAt, &job.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.Job{}, delivery.ErrRetryNotAllowed
		}
		return delivery.Job{}, fmt.Errorf("reset delivery job: %w", err)
	}
	return job, nil
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
