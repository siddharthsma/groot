package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ID = uuid.UUID

type Tenant struct {
	ID        ID        `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type CreatedTenant struct {
	Tenant Tenant
	APIKey string
}

var (
	ErrInvalidTenantName = errors.New("name is required")
	ErrTenantNameExists  = errors.New("tenant name already exists")
	ErrTenantNotFound    = errors.New("tenant not found")
	ErrUnauthorized      = errors.New("unauthorized")
)

type Store interface {
	CreateTenant(context.Context, TenantRecord) (Tenant, error)
	ListTenants(context.Context) ([]Tenant, error)
	GetTenant(context.Context, ID) (Tenant, error)
	GetTenantByAPIKeyHash(context.Context, string) (Tenant, error)
}

type TenantRecord struct {
	ID         ID
	Name       string
	APIKeyHash string
	CreatedAt  time.Time
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

func (s *Service) CreateTenant(ctx context.Context, name string) (CreatedTenant, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return CreatedTenant{}, ErrInvalidTenantName
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return CreatedTenant{}, fmt.Errorf("generate API key: %w", err)
	}

	record := TenantRecord{
		ID:         uuid.New(),
		Name:       trimmedName,
		APIKeyHash: HashAPIKey(apiKey),
		CreatedAt:  s.now(),
	}

	created, err := s.store.CreateTenant(ctx, record)
	if err != nil {
		return CreatedTenant{}, err
	}

	return CreatedTenant{
		Tenant: created,
		APIKey: apiKey,
	}, nil
}

func (s *Service) ListTenants(ctx context.Context) ([]Tenant, error) {
	return s.store.ListTenants(ctx)
}

func (s *Service) GetTenant(ctx context.Context, id ID) (Tenant, error) {
	return s.store.GetTenant(ctx, id)
}

func (s *Service) Authenticate(ctx context.Context, apiKey string) (Tenant, error) {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return Tenant{}, ErrUnauthorized
	}

	record, err := s.store.GetTenantByAPIKeyHash(ctx, HashAPIKey(trimmed))
	if err != nil {
		if errors.Is(err, ErrTenantNotFound) {
			return Tenant{}, ErrUnauthorized
		}
		return Tenant{}, err
	}
	return record, nil
}

func HashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

func generateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
