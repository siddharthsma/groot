package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"groot/internal/apikey"
	"groot/internal/config"
	"groot/internal/tenant"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

type LegacyAuthenticator interface {
	Authenticate(context.Context, string) (tenant.Tenant, error)
}

type APIKeyAuthenticator interface {
	Authenticate(context.Context, string) (apikey.AuthenticatedKey, error)
}

type JWTVerifier interface {
	Verify(context.Context, string) (JWTPrincipal, error)
}

type JWTPrincipal struct {
	TenantID tenant.ID
	Subject  string
	Email    string
	Claims   jwt.MapClaims
}

type Service struct {
	cfg    config.AuthConfig
	legacy LegacyAuthenticator
	keys   APIKeyAuthenticator
	jwt    JWTVerifier
}

func NewService(cfg config.AuthConfig, legacy LegacyAuthenticator, keys APIKeyAuthenticator, jwtVerifier JWTVerifier) *Service {
	return &Service{cfg: cfg, legacy: legacy, keys: keys, jwt: jwtVerifier}
}

func (s *Service) AuthenticateRequest(r *http.Request) (Principal, error) {
	requestInfo := BuildRequestInfo(r)
	headerKey := strings.TrimSpace(r.Header.Get(s.cfg.APIKeyHeader))
	bearer := bearerToken(r)
	switch s.cfg.Mode {
	case "jwt":
		if bearer == "" {
			return Principal{}, ErrUnauthorized
		}
		return s.authenticateJWT(r.Context(), bearer, r, requestInfo)
	case "mixed":
		return s.authenticateMixed(r, headerKey, bearer, requestInfo)
	default:
		return s.authenticateAPIKeyMode(r, headerKey, bearer, requestInfo)
	}
}

func (s *Service) authenticateAPIKeyMode(r *http.Request, headerKey, bearer string, requestInfo RequestInfo) (Principal, error) {
	if headerKey != "" {
		return s.authenticateAPIKey(r.Context(), headerKey, r, requestInfo)
	}
	if bearer != "" {
		return s.authenticateLegacy(r.Context(), bearer, r, requestInfo)
	}
	return Principal{}, ErrUnauthorized
}

func (s *Service) authenticateMixed(r *http.Request, headerKey, bearer string, requestInfo RequestInfo) (Principal, error) {
	switch {
	case headerKey != "" && bearer != "":
		apiPrincipal, err := s.authenticateAPIKey(r.Context(), headerKey, r, requestInfo)
		if err != nil {
			return Principal{}, err
		}
		var other Principal
		if looksLikeJWT(bearer) {
			other, err = s.authenticateJWT(r.Context(), bearer, r, requestInfo)
		} else {
			other, err = s.authenticateLegacy(r.Context(), bearer, r, requestInfo)
		}
		if err != nil {
			return Principal{}, err
		}
		if apiPrincipal.TenantID != other.TenantID {
			return Principal{}, ErrForbidden
		}
		return apiPrincipal, nil
	case headerKey != "":
		return s.authenticateAPIKey(r.Context(), headerKey, r, requestInfo)
	case bearer != "":
		if looksLikeJWT(bearer) {
			return s.authenticateJWT(r.Context(), bearer, r, requestInfo)
		}
		return s.authenticateLegacy(r.Context(), bearer, r, requestInfo)
	default:
		return Principal{}, ErrUnauthorized
	}
}

func (s *Service) authenticateAPIKey(ctx context.Context, fullKey string, r *http.Request, requestInfo RequestInfo) (Principal, error) {
	if s.keys == nil {
		return Principal{}, ErrUnauthorized
	}
	authenticated, err := s.keys.Authenticate(ctx, fullKey)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		TenantID:      tenant.ID(authenticated.Key.TenantID),
		PrincipalKind: "api_key",
		PrincipalID:   "api_key:" + authenticated.Key.ID.String(),
		Actor: Actor{
			Type:  headerOrDefault(r.Header.Get(s.cfg.ActorTypeHeader), "service"),
			ID:    strings.TrimSpace(r.Header.Get(s.cfg.ActorIDHeader)),
			Email: strings.TrimSpace(r.Header.Get(s.cfg.ActorEmailHeader)),
		},
		RequestInfo: requestInfo,
	}, nil
}

func (s *Service) authenticateLegacy(ctx context.Context, fullKey string, r *http.Request, requestInfo RequestInfo) (Principal, error) {
	if s.legacy == nil {
		return Principal{}, ErrUnauthorized
	}
	record, err := s.legacy.Authenticate(ctx, fullKey)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		TenantID:      record.ID,
		PrincipalKind: "api_key",
		PrincipalID:   "tenant:" + record.ID.String() + ":legacy",
		Actor: Actor{
			Type:  headerOrDefault(r.Header.Get(s.cfg.ActorTypeHeader), "service"),
			ID:    strings.TrimSpace(r.Header.Get(s.cfg.ActorIDHeader)),
			Email: strings.TrimSpace(r.Header.Get(s.cfg.ActorEmailHeader)),
		},
		RequestInfo: requestInfo,
	}, nil
}

func (s *Service) authenticateJWT(ctx context.Context, token string, r *http.Request, requestInfo RequestInfo) (Principal, error) {
	if s.jwt == nil {
		return Principal{}, ErrUnauthorized
	}
	principal, err := s.jwt.Verify(ctx, token)
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		TenantID:      principal.TenantID,
		PrincipalKind: "jwt",
		PrincipalID:   principal.Subject,
		Actor: Actor{
			Type:  headerOrDefault(r.Header.Get(s.cfg.ActorTypeHeader), "user"),
			ID:    headerOrDefault(r.Header.Get(s.cfg.ActorIDHeader), principal.Subject),
			Email: headerOrDefault(r.Header.Get(s.cfg.ActorEmailHeader), principal.Email),
		},
		RequestInfo: requestInfo,
	}, nil
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func looksLikeJWT(token string) bool {
	return strings.Count(strings.TrimSpace(token), ".") == 2
}

func headerOrDefault(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return strings.TrimSpace(fallback)
	}
	return trimmed
}
