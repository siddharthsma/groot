package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"groot/internal/config"
)

func TestJWTVerifierVerify(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": "test-key",
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
				},
			},
		})
	}))
	defer jwksServer.Close()

	cfg := config.AuthConfig{
		JWTJWKSURL:        jwksServer.URL,
		JWTAudience:       "groot",
		JWTIssuer:         "issuer",
		JWTRequiredClaims: []string{"sub", "tenant_id"},
		JWTTenantClaim:    "tenant_id",
		JWTClockSkew:      time.Second,
	}
	verifier, err := NewJWTVerifier(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewJWTVerifier() error = %v", err)
	}

	tenantID := uuid.New()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":       "user-123",
		"email":     "user@example.com",
		"tenant_id": tenantID.String(),
		"aud":       "groot",
		"iss":       "issuer",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	principal, err := verifier.Verify(context.Background(), signed)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if principal.TenantID != tenantID {
		t.Fatalf("TenantID = %s, want %s", principal.TenantID, tenantID)
	}
	if principal.Subject != "user-123" {
		t.Fatalf("Subject = %q, want user-123", principal.Subject)
	}
}
