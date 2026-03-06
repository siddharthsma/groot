package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"groot/internal/apikey"
	"groot/internal/config"
	"groot/internal/tenant"
)

type stubLegacyAuthenticator struct {
	record tenant.Tenant
	err    error
}

func (s stubLegacyAuthenticator) Authenticate(context.Context, string) (tenant.Tenant, error) {
	if s.err != nil {
		return tenant.Tenant{}, s.err
	}
	return s.record, nil
}

type stubAPIKeyAuthenticator struct {
	record apikey.AuthenticatedKey
	err    error
}

func (s stubAPIKeyAuthenticator) Authenticate(context.Context, string) (apikey.AuthenticatedKey, error) {
	if s.err != nil {
		return apikey.AuthenticatedKey{}, s.err
	}
	return s.record, nil
}

type stubJWTVerifier struct {
	principal JWTPrincipal
	err       error
}

func (s stubJWTVerifier) Verify(context.Context, string) (JWTPrincipal, error) {
	if s.err != nil {
		return JWTPrincipal{}, s.err
	}
	return s.principal, nil
}

func TestServiceAuthenticateAPIKeyHeader(t *testing.T) {
	tenantID := uuid.New()
	svc := NewService(config.AuthConfig{
		Mode:             "api_key",
		APIKeyHeader:     "X-API-Key",
		ActorIDHeader:    "X-Actor-Id",
		ActorTypeHeader:  "X-Actor-Type",
		ActorEmailHeader: "X-Actor-Email",
	}, nil, stubAPIKeyAuthenticator{
		record: apikey.AuthenticatedKey{
			Key: apikey.APIKey{ID: uuid.New(), TenantID: tenantID},
		},
	}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("X-API-Key", "groot_abcd1234_secret")

	principal, err := svc.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest() error = %v", err)
	}
	if principal.TenantID != tenantID {
		t.Fatalf("TenantID = %s, want %s", principal.TenantID, tenantID)
	}
	if principal.PrincipalKind != "api_key" {
		t.Fatalf("PrincipalKind = %q, want api_key", principal.PrincipalKind)
	}
	if principal.Actor.Type != "service" {
		t.Fatalf("Actor.Type = %q, want service", principal.Actor.Type)
	}
}

func TestServiceAuthenticateLegacyBearer(t *testing.T) {
	tenantID := uuid.New()
	svc := NewService(config.AuthConfig{
		Mode:             "api_key",
		APIKeyHeader:     "X-API-Key",
		ActorIDHeader:    "X-Actor-Id",
		ActorTypeHeader:  "X-Actor-Type",
		ActorEmailHeader: "X-Actor-Email",
	}, stubLegacyAuthenticator{
		record: tenant.Tenant{ID: tenantID},
	}, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Authorization", "Bearer legacy-secret")

	principal, err := svc.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest() error = %v", err)
	}
	if principal.PrincipalID != "tenant:"+tenantID.String()+":legacy" {
		t.Fatalf("PrincipalID = %q", principal.PrincipalID)
	}
}

func TestServiceAuthenticateMixedMismatch(t *testing.T) {
	svc := NewService(config.AuthConfig{
		Mode:             "mixed",
		APIKeyHeader:     "X-API-Key",
		ActorIDHeader:    "X-Actor-Id",
		ActorTypeHeader:  "X-Actor-Type",
		ActorEmailHeader: "X-Actor-Email",
	}, nil, stubAPIKeyAuthenticator{
		record: apikey.AuthenticatedKey{
			Key: apikey.APIKey{ID: uuid.New(), TenantID: uuid.New()},
		},
	}, stubJWTVerifier{
		principal: JWTPrincipal{TenantID: uuid.New(), Subject: "user-1"},
	})

	req, _ := http.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("X-API-Key", "groot_abcd1234_secret")
	req.Header.Set("Authorization", "Bearer a.b.c")

	_, err := svc.AuthenticateRequest(req)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("AuthenticateRequest() error = %v, want forbidden", err)
	}
}
