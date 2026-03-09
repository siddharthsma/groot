package inboundroute

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connection"
	"groot/internal/tenant"
)

type Route struct {
	ID              uuid.UUID  `json:"id"`
	IntegrationName string     `json:"integration_name"`
	RouteKey        string     `json:"route_key"`
	TenantID        uuid.UUID  `json:"tenant_id,omitempty"`
	ConnectionID    *uuid.UUID `json:"connection_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at,omitempty"`
}

type Record struct {
	ID              uuid.UUID
	IntegrationName string
	RouteKey        string
	TenantID        tenant.ID
	ConnectionID    *uuid.UUID
	CreatedAt       time.Time
}

var (
	ErrInvalidIntegrationName = errors.New("integration_name is required")
	ErrInvalidRouteKey        = errors.New("route_key is required")
	ErrDuplicateRoute         = errors.New("inbound route already exists")
	ErrConnectionNotFound     = errors.New("connection not found")
	ErrInvalidConnection      = errors.New("connection must be tenant-scoped and owned by tenant")
)

type Store interface {
	CreateInboundRoute(context.Context, Record) (Route, error)
	ListInboundRoutes(context.Context, tenant.ID) ([]Route, error)
	ListAllInboundRoutes(context.Context) ([]Route, error)
	GetConnection(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
}

type Metrics interface {
	IncInboundRoutes(string)
}

type Logger interface {
	Info(string, ...any)
}

type Service struct {
	store   Store
	metrics Metrics
	now     func() time.Time
}

func NewService(store Store, metrics Metrics) *Service {
	return &Service{
		store:   store,
		metrics: metrics,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, integrationName, routeKey string, connectionID *uuid.UUID) (Route, error) {
	normalizedIntegration := strings.TrimSpace(integrationName)
	if normalizedIntegration == "" {
		return Route{}, ErrInvalidIntegrationName
	}
	normalizedRouteKey := strings.TrimSpace(routeKey)
	if normalizedRouteKey == "" {
		return Route{}, ErrInvalidRouteKey
	}
	if connectionID != nil {
		instance, err := s.store.GetConnection(ctx, tenantID, *connectionID)
		if err != nil {
			if errors.Is(err, connection.ErrNotFound) {
				return Route{}, ErrConnectionNotFound
			}
			return Route{}, fmt.Errorf("get connection: %w", err)
		}
		if instance.Scope != connection.ScopeTenant || instance.OwnerTenantID == nil || *instance.OwnerTenantID != uuid.UUID(tenantID) {
			return Route{}, ErrInvalidConnection
		}
	}
	route, err := s.store.CreateInboundRoute(ctx, Record{
		ID:              uuid.New(),
		IntegrationName: normalizedIntegration,
		RouteKey:        normalizedRouteKey,
		TenantID:        tenantID,
		ConnectionID:    connectionID,
		CreatedAt:       s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateRoute) {
			return Route{}, ErrDuplicateRoute
		}
		return Route{}, fmt.Errorf("create inbound route: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncInboundRoutes(normalizedIntegration)
	}
	return route, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Route, error) {
	routes, err := s.store.ListInboundRoutes(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list inbound routes: %w", err)
	}
	return routes, nil
}

func (s *Service) ListAll(ctx context.Context) ([]Route, error) {
	routes, err := s.store.ListAllInboundRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all inbound routes: %w", err)
	}
	return routes, nil
}
