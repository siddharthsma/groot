package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"groot/internal/inboundroute"
	"groot/internal/tenant"
)

func (d *DB) GetInboundRouteByTenant(ctx context.Context, integrationName string, tenantID tenant.ID) (inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		WHERE connector_name = $1 AND tenant_id = $2
		ORDER BY created_at ASC
		LIMIT 1
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, integrationName, tenantID).Scan(&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return inboundroute.Route{}, sql.ErrNoRows
		}
		return inboundroute.Route{}, fmt.Errorf("get inbound route by tenant: %w", err)
	}
	return route, nil
}

func (d *DB) UpdateInboundRouteByTenant(ctx context.Context, integrationName string, tenantID tenant.ID, routeKey string, connectionID *uuid.UUID) (inboundroute.Route, error) {
	const query = `
		UPDATE inbound_routes
		SET route_key = $3, connector_instance_id = $4
		WHERE connector_name = $1 AND tenant_id = $2
		RETURNING id, connector_name, route_key, tenant_id, connector_instance_id, created_at
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, integrationName, tenantID, routeKey, connectionID).Scan(
		&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt,
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
	err := d.db.QueryRowContext(ctx, query, record.ID, record.IntegrationName, record.RouteKey, record.TenantID, record.ConnectionID, record.CreatedAt).Scan(
		&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return inboundroute.Route{}, inboundroute.ErrDuplicateRoute
		}
		return inboundroute.Route{}, fmt.Errorf("create inbound route: %w", err)
	}
	return route, nil
}

func (d *DB) GetInboundRoute(ctx context.Context, integrationName, routeKey string) (inboundroute.Route, error) {
	const query = `
		SELECT id, connector_name, route_key, tenant_id, connector_instance_id, created_at
		FROM inbound_routes
		WHERE connector_name = $1 AND route_key = $2
	`
	var route inboundroute.Route
	err := d.db.QueryRowContext(ctx, query, integrationName, routeKey).Scan(
		&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt,
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
		if err := rows.Scan(&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt); err != nil {
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
		if err := rows.Scan(&route.ID, &route.IntegrationName, &route.RouteKey, &route.TenantID, &route.ConnectionID, &route.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan inbound route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all inbound routes: %w", err)
	}
	return routes, nil
}
