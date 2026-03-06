package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"groot/internal/config"
)

type verifier struct {
	cfg  config.AuthConfig
	jwks *keyfunc.JWKS
}

func NewJWTVerifier(ctx context.Context, cfg config.AuthConfig) (JWTVerifier, error) {
	if strings.TrimSpace(cfg.JWTJWKSURL) == "" {
		return nil, nil
	}
	jwks, err := keyfunc.Get(cfg.JWTJWKSURL, keyfunc.Options{
		Ctx:               ctx,
		RefreshUnknownKID: true,
		RefreshTimeout:    10 * time.Second,
		RefreshInterval:   time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("load jwks: %w", err)
	}
	return &verifier{cfg: cfg, jwks: jwks}, nil
}

func (v *verifier) Verify(_ context.Context, token string) (JWTPrincipal, error) {
	claims := jwt.MapClaims{}
	options := []jwt.ParserOption{jwt.WithLeeway(v.cfg.JWTClockSkew)}
	if strings.TrimSpace(v.cfg.JWTAudience) != "" {
		options = append(options, jwt.WithAudience(strings.TrimSpace(v.cfg.JWTAudience)))
	}
	if strings.TrimSpace(v.cfg.JWTIssuer) != "" {
		options = append(options, jwt.WithIssuer(strings.TrimSpace(v.cfg.JWTIssuer)))
	}
	parsed, err := jwt.ParseWithClaims(token, claims, v.jwks.Keyfunc, options...)
	if err != nil {
		return JWTPrincipal{}, fmt.Errorf("parse jwt: %w", err)
	}
	if !parsed.Valid {
		return JWTPrincipal{}, errors.New("invalid jwt")
	}
	for _, claim := range v.cfg.JWTRequiredClaims {
		if strings.TrimSpace(claim) == "" {
			continue
		}
		if _, ok := claims[claim]; !ok {
			return JWTPrincipal{}, fmt.Errorf("missing required claim %s", claim)
		}
	}
	tenantRaw, ok := claims[v.cfg.JWTTenantClaim]
	if !ok {
		return JWTPrincipal{}, fmt.Errorf("missing tenant claim %s", v.cfg.JWTTenantClaim)
	}
	tenantValue := strings.TrimSpace(fmt.Sprint(tenantRaw))
	tenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return JWTPrincipal{}, fmt.Errorf("invalid tenant claim: %w", err)
	}
	subject := strings.TrimSpace(fmt.Sprint(claims["sub"]))
	email := ""
	if rawEmail, ok := claims["email"]; ok {
		email = strings.TrimSpace(fmt.Sprint(rawEmail))
	}
	return JWTPrincipal{
		TenantID: tenantID,
		Subject:  subject,
		Email:    email,
		Claims:   claims,
	}, nil
}
