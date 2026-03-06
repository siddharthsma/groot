package inboundroute

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/connectorinstance"
	"groot/internal/tenant"
)

type Route struct {
	ID                  uuid.UUID  `json:"id"`
	ConnectorName       string     `json:"connector_name"`
	RouteKey            string     `json:"route_key"`
	TenantID            uuid.UUID  `json:"tenant_id,omitempty"`
	ConnectorInstanceID *uuid.UUID `json:"connector_instance_id,omitempty"`
	CreatedAt           time.Time  `json:"created_at,omitempty"`
}

type Record struct {
	ID                  uuid.UUID
	ConnectorName       string
	RouteKey            string
	TenantID            tenant.ID
	ConnectorInstanceID *uuid.UUID
	CreatedAt           time.Time
}

var (
	ErrInvalidConnectorName      = errors.New("connector_name is required")
	ErrInvalidRouteKey           = errors.New("route_key is required")
	ErrDuplicateRoute            = errors.New("inbound route already exists")
	ErrConnectorInstanceNotFound = errors.New("connector instance not found")
	ErrInvalidConnectorInstance  = errors.New("connector instance must be tenant-scoped and owned by tenant")
)

type Store interface {
	CreateInboundRoute(context.Context, Record) (Route, error)
	ListInboundRoutes(context.Context, tenant.ID) ([]Route, error)
	ListAllInboundRoutes(context.Context) ([]Route, error)
	GetConnectorInstance(context.Context, tenant.ID, uuid.UUID) (connectorinstance.Instance, error)
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

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, connectorName, routeKey string, connectorInstanceID *uuid.UUID) (Route, error) {
	normalizedConnector := strings.TrimSpace(connectorName)
	if normalizedConnector == "" {
		return Route{}, ErrInvalidConnectorName
	}
	normalizedRouteKey := strings.TrimSpace(routeKey)
	if normalizedRouteKey == "" {
		return Route{}, ErrInvalidRouteKey
	}
	if connectorInstanceID != nil {
		instance, err := s.store.GetConnectorInstance(ctx, tenantID, *connectorInstanceID)
		if err != nil {
			if errors.Is(err, connectorinstance.ErrNotFound) {
				return Route{}, ErrConnectorInstanceNotFound
			}
			return Route{}, fmt.Errorf("get connector instance: %w", err)
		}
		if instance.Scope != connectorinstance.ScopeTenant || instance.OwnerTenantID == nil || *instance.OwnerTenantID != uuid.UUID(tenantID) {
			return Route{}, ErrInvalidConnectorInstance
		}
	}
	route, err := s.store.CreateInboundRoute(ctx, Record{
		ID:                  uuid.New(),
		ConnectorName:       normalizedConnector,
		RouteKey:            normalizedRouteKey,
		TenantID:            tenantID,
		ConnectorInstanceID: connectorInstanceID,
		CreatedAt:           s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateRoute) {
			return Route{}, ErrDuplicateRoute
		}
		return Route{}, fmt.Errorf("create inbound route: %w", err)
	}
	if s.metrics != nil {
		s.metrics.IncInboundRoutes(normalizedConnector)
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
