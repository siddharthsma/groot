package connectedapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

type App struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"-"`
	Name           string    `json:"name"`
	DestinationURL string    `json:"destination_url"`
	CreatedAt      time.Time `json:"-"`
}

type Record struct {
	ID             uuid.UUID
	TenantID       tenant.ID
	Name           string
	DestinationURL string
	CreatedAt      time.Time
}

var (
	ErrInvalidName           = errors.New("name is required")
	ErrInvalidDestinationURL = errors.New("destination_url must be an absolute http or https URL with a host")
	ErrNotFound              = errors.New("connected app not found")
)

type Store interface {
	CreateConnectedApp(context.Context, Record) (App, error)
	ListConnectedApps(context.Context, tenant.ID) ([]App, error)
	GetConnectedApp(context.Context, tenant.ID, uuid.UUID) (App, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, name, destinationURL string) (App, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return App{}, ErrInvalidName
	}

	normalizedURL, err := validateDestinationURL(destinationURL)
	if err != nil {
		return App{}, err
	}

	record := Record{
		ID:             uuid.New(),
		TenantID:       tenantID,
		Name:           trimmedName,
		DestinationURL: normalizedURL,
		CreatedAt:      s.now(),
	}

	app, err := s.store.CreateConnectedApp(ctx, record)
	if err != nil {
		return App{}, fmt.Errorf("create connected app: %w", err)
	}

	return app, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]App, error) {
	apps, err := s.store.ListConnectedApps(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list connected apps: %w", err)
	}
	return apps, nil
}

func (s *Service) Get(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (App, error) {
	app, err := s.store.GetConnectedApp(ctx, tenantID, appID)
	if err != nil {
		return App{}, err
	}
	return app, nil
}

func validateDestinationURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil {
		return "", ErrInvalidDestinationURL
	}

	if !parsed.IsAbs() || parsed.Host == "" {
		return "", ErrInvalidDestinationURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidDestinationURL
	}

	return parsed.String(), nil
}
