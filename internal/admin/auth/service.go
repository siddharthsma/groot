package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/golang-jwt/jwt/v5"

	iauth "groot/internal/auth"
	"groot/internal/config"
)

var ErrUnauthorized = errors.New("unauthorized")

type Service struct {
	cfg  config.AdminConfig
	jwks *keyfunc.JWKS
}

func New(ctx context.Context, cfg config.AdminConfig) (*Service, error) {
	service := &Service{cfg: cfg}
	if !cfg.Enabled {
		return service, nil
	}
	if cfg.AuthMode == "jwt" {
		jwks, err := keyfunc.Get(cfg.JWTJWKSURL, keyfunc.Options{
			Ctx:               ctx,
			RefreshUnknownKID: true,
			RefreshTimeout:    10 * time.Second,
			RefreshInterval:   time.Hour,
		})
		if err != nil {
			return nil, fmt.Errorf("load admin jwks: %w", err)
		}
		service.jwks = jwks
	}
	return service, nil
}

func (s *Service) AuthenticateRequest(r *http.Request) (iauth.Principal, error) {
	if !s.cfg.Enabled {
		return iauth.Principal{}, ErrUnauthorized
	}
	switch s.cfg.AuthMode {
	case "jwt":
		return s.authenticateJWT(r)
	default:
		return s.authenticateAPIKey(r)
	}
}

func (s *Service) authenticateAPIKey(r *http.Request) (iauth.Principal, error) {
	provided := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	expected := strings.TrimSpace(s.cfg.APIKey)
	if provided == "" || expected == "" {
		return iauth.Principal{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return iauth.Principal{}, ErrUnauthorized
	}
	return iauth.Principal{
		PrincipalKind: "api_key",
		PrincipalID:   "admin_api_key",
		Actor: iauth.Actor{
			Type: "operator",
			ID:   "admin_api_key",
		},
		RequestInfo: iauth.BuildRequestInfo(r),
	}, nil
}

func (s *Service) authenticateJWT(r *http.Request) (iauth.Principal, error) {
	tokenString := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if tokenString == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return iauth.Principal{}, ErrUnauthorized
	}
	claims := jwt.MapClaims{}
	options := []jwt.ParserOption{jwt.WithLeeway(60 * time.Second)}
	if strings.TrimSpace(s.cfg.JWTAudience) != "" {
		options = append(options, jwt.WithAudience(strings.TrimSpace(s.cfg.JWTAudience)))
	}
	if strings.TrimSpace(s.cfg.JWTIssuer) != "" {
		options = append(options, jwt.WithIssuer(strings.TrimSpace(s.cfg.JWTIssuer)))
	}
	token, err := jwt.ParseWithClaims(tokenString, claims, s.jwks.Keyfunc, options...)
	if err != nil || !token.Valid {
		return iauth.Principal{}, ErrUnauthorized
	}
	for _, claim := range s.cfg.JWTRequiredClaims {
		if _, ok := claims[claim]; !ok {
			return iauth.Principal{}, ErrUnauthorized
		}
	}
	subject := strings.TrimSpace(fmt.Sprint(claims["sub"]))
	email := ""
	if rawEmail, ok := claims["email"]; ok {
		email = strings.TrimSpace(fmt.Sprint(rawEmail))
	}
	return iauth.Principal{
		PrincipalKind: "jwt",
		PrincipalID:   subject,
		Actor: iauth.Actor{
			Type:  "operator",
			ID:    subject,
			Email: email,
		},
		RequestInfo: iauth.BuildRequestInfo(r),
	}, nil
}
