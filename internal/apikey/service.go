package apikey

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"

	"groot/internal/tenant"
)

type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"-"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type CreatedAPIKey struct {
	APIKey
	Secret string `json:"api_key"`
}

type Record struct {
	ID        uuid.UUID
	TenantID  tenant.ID
	Name      string
	KeyPrefix string
	KeyHash   string
	IsActive  bool
	CreatedAt time.Time
}

type AuthenticatedKey struct {
	Key APIKey
}

var (
	ErrInvalidName     = errors.New("name is required")
	ErrNotFound        = errors.New("api key not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrDuplicatePrefix = errors.New("api key prefix already exists")
)

type Store interface {
	CreateAPIKey(context.Context, Record) (APIKey, error)
	ListAPIKeys(context.Context, tenant.ID) ([]APIKey, error)
	RevokeAPIKey(context.Context, tenant.ID, uuid.UUID, time.Time) (APIKey, error)
	GetAPIKeyByPrefix(context.Context, string) (APIKeyRecord, error)
	TouchAPIKeyLastUsed(context.Context, uuid.UUID, time.Time) error
}

type APIKeyRecord struct {
	APIKey
	KeyHash string
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, name string) (CreatedAPIKey, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return CreatedAPIKey{}, ErrInvalidName
	}
	fullKey, prefix, err := generateKey()
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("generate api key: %w", err)
	}
	hash, err := Hash(fullKey)
	if err != nil {
		return CreatedAPIKey{}, fmt.Errorf("hash api key: %w", err)
	}
	created, err := s.store.CreateAPIKey(ctx, Record{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      trimmedName,
		KeyPrefix: prefix,
		KeyHash:   hash,
		IsActive:  true,
		CreatedAt: s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrDuplicatePrefix) {
			return CreatedAPIKey{}, ErrDuplicatePrefix
		}
		return CreatedAPIKey{}, err
	}
	return CreatedAPIKey{APIKey: created, Secret: fullKey}, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]APIKey, error) {
	return s.store.ListAPIKeys(ctx, tenantID)
}

func (s *Service) Revoke(ctx context.Context, tenantID tenant.ID, id uuid.UUID) (APIKey, error) {
	key, err := s.store.RevokeAPIKey(ctx, tenantID, id, s.now())
	if err != nil {
		return APIKey{}, err
	}
	return key, nil
}

func (s *Service) Authenticate(ctx context.Context, fullKey string) (AuthenticatedKey, error) {
	prefix, err := ParsePrefix(fullKey)
	if err != nil {
		return AuthenticatedKey{}, ErrUnauthorized
	}
	record, err := s.store.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthenticatedKey{}, ErrUnauthorized
		}
		return AuthenticatedKey{}, err
	}
	if !record.IsActive {
		return AuthenticatedKey{}, ErrUnauthorized
	}
	ok, err := Verify(record.KeyHash, fullKey)
	if err != nil {
		return AuthenticatedKey{}, err
	}
	if !ok {
		return AuthenticatedKey{}, ErrUnauthorized
	}
	_ = s.store.TouchAPIKeyLastUsed(ctx, record.ID, s.now())
	return AuthenticatedKey{Key: record.APIKey}, nil
}

func ParsePrefix(fullKey string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(fullKey), "_", 3)
	if len(parts) != 3 || parts[0] != "groot" || len(parts[1]) != 8 || strings.TrimSpace(parts[2]) == "" {
		return "", errors.New("invalid api key format")
	}
	return parts[1], nil
}

func generateKey() (string, string, error) {
	prefixBytes := make([]byte, 4)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", err
	}
	prefix := hex.EncodeToString(prefixBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return "groot_" + prefix + "_" + secret, prefix, nil
}

func Hash(fullKey string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(fullKey), salt, 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + "." + base64.RawStdEncoding.EncodeToString(hash), nil
}

func Verify(encodedHash string, fullKey string) (bool, error) {
	parts := strings.Split(strings.TrimSpace(encodedHash), ".")
	if len(parts) != 2 {
		return false, errors.New("invalid api key hash format")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false, err
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(fullKey), salt, 1, 64*1024, 4, uint32(len(expected)))
	return subtle.ConstantTimeCompare(expected, actual) == 1, nil
}
