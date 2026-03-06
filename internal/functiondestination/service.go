package functiondestination

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/tenant"
)

const (
	DefaultTimeoutSeconds = 10
	MaxTimeoutSeconds     = 30
)

type Destination struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"-"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Secret         string    `json:"-"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	CreatedAt      time.Time `json:"-"`
}

type CreatedDestination struct {
	Destination Destination
	Secret      string
}

type Record struct {
	ID             uuid.UUID
	TenantID       tenant.ID
	Name           string
	URL            string
	Secret         string
	TimeoutSeconds int
	CreatedAt      time.Time
}

var (
	ErrInvalidName    = errors.New("name is required")
	ErrInvalidURL     = errors.New("url must be an absolute https URL with a host, except local test hosts may use http")
	ErrNotFound       = errors.New("function destination not found")
	ErrInUse          = errors.New("function destination has active subscriptions")
	ErrInvalidTimeout = errors.New("timeout_seconds must be between 1 and 30")
)

type Store interface {
	CreateFunctionDestination(context.Context, Record) (Destination, error)
	ListFunctionDestinations(context.Context, tenant.ID) ([]Destination, error)
	GetFunctionDestination(context.Context, tenant.ID, uuid.UUID) (Destination, error)
	DeleteFunctionDestination(context.Context, tenant.ID, uuid.UUID) error
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

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, name, rawURL string) (CreatedDestination, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return CreatedDestination{}, ErrInvalidName
	}

	normalizedURL, err := validateURL(rawURL)
	if err != nil {
		return CreatedDestination{}, err
	}

	secret, err := generateSecret()
	if err != nil {
		return CreatedDestination{}, fmt.Errorf("generate secret: %w", err)
	}

	record := Record{
		ID:             uuid.New(),
		TenantID:       tenantID,
		Name:           trimmedName,
		URL:            normalizedURL,
		Secret:         secret,
		TimeoutSeconds: DefaultTimeoutSeconds,
		CreatedAt:      s.now(),
	}

	destination, err := s.store.CreateFunctionDestination(ctx, record)
	if err != nil {
		return CreatedDestination{}, fmt.Errorf("create function destination: %w", err)
	}
	return CreatedDestination{Destination: destination, Secret: secret}, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Destination, error) {
	destinations, err := s.store.ListFunctionDestinations(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list function destinations: %w", err)
	}
	return destinations, nil
}

func (s *Service) Get(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (Destination, error) {
	destination, err := s.store.GetFunctionDestination(ctx, tenantID, id)
	if err != nil {
		return Destination{}, err
	}
	return destination, nil
}

func (s *Service) Delete(ctx context.Context, tenantID tenant.ID, id uuid.UUID) error {
	if err := s.store.DeleteFunctionDestination(ctx, tenantID, id); err != nil {
		return err
	}
	return nil
}

func validateURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil {
		return "", ErrInvalidURL
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", ErrInvalidURL
	}

	host := parsed.Hostname()
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLocalHost(host) {
			return "", ErrInvalidURL
		}
	default:
		return "", ErrInvalidURL
	}

	return parsed.String(), nil
}

func isLocalHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
