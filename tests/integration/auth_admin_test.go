//go:build integration

package integration

import (
	"net/http"
	"testing"

	"groot/tests/helpers"
)

func TestAPIKeyAndJWTAuthFlows(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	tenantID, legacyKey := h.CreateTenant("phase20-auth")
	realAPIKey := createRealAPIKey(t, h, legacyKey, "ci-bot")
	h.AssertNoSecrets(realAPIKey)

	resp, body := h.JSONRequest(http.MethodPost, "/events", apiKeyHeader(realAPIKey), map[string]any{
		"type":   "example.auth.v1",
		"source": "manual",
		"payload": map[string]any{
			"via": "api_key",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)

	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	if err := h.StartAPI(helpers.HarnessOptions{AuthMode: "mixed", AdminAuthMode: "api_key", JWKSURL: h.Mocks.JWKSServer.URL}); err != nil {
		t.Fatalf("restart api with mixed auth: %v", err)
	}

	jwtToken := issueJWT(t, h, tenantID, "groot")
	resp, body = h.JSONRequest(http.MethodPost, "/events", bearerHeader(jwtToken), map[string]any{
		"type":   "example.jwt.v1",
		"source": "manual",
		"payload": map[string]any{
			"via": "jwt",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.Request(http.MethodGet, "/admin/tenants", adminHeader(h.AdminKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	mustNoSecretInBody(t, body, legacyKey, realAPIKey, h.AdminKey)
}
