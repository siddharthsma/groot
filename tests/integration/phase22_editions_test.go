//go:build integration

package integration

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/internal/edition"
	"groot/internal/tenant"
	"groot/tests/helpers"
)

func TestPhase22CommunityTenantRestriction(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"GROOT_TENANCY_MODE":    "single",
		"COMMUNITY_TENANT_NAME": "Community Bootstrap",
		"ADMIN_MODE_ENABLED":    "true",
	}, BuildEdition: "community"})

	resp, body := h.JSONRequest(http.MethodPost, "/tenants", nil, map[string]any{"name": "blocked"})
	mustStatus(t, resp, body, http.StatusForbidden)
	if !strings.Contains(string(body), "community_edition_restriction") {
		t.Fatalf("body = %s", body)
	}

	resp, body = h.Request(http.MethodGet, "/tenants", nil, nil)
	mustStatus(t, resp, body, http.StatusForbidden)

	resp, body = h.Request(http.MethodGet, "/admin/tenants", adminHeader(h.AdminKey), nil)
	mustStatus(t, resp, body, http.StatusForbidden)
}

func TestPhase22CommunityBootstrapTenant(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"GROOT_TENANCY_MODE":    "single",
		"COMMUNITY_TENANT_NAME": "Community Bootstrap",
	}, BuildEdition: "community"})

	var count int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM tenants WHERE id <> '00000000-0000-0000-0000-000000000000'`).Scan(&count); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if count != 1 {
		t.Fatalf("tenant count = %d, want 1", count)
	}

	var name string
	if err := h.DB.QueryRow(`SELECT name FROM tenants WHERE id <> '00000000-0000-0000-0000-000000000000' ORDER BY created_at ASC LIMIT 1`).Scan(&name); err != nil {
		t.Fatalf("select tenant name: %v", err)
	}
	if name != "Community Bootstrap" {
		t.Fatalf("tenant name = %q, want %q", name, "Community Bootstrap")
	}
}

func TestPhase22CommunityMultiTenantPrevention(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	if err := h.ResetDatabase(); err != nil {
		t.Fatalf("reset database: %v", err)
	}

	now := time.Now().UTC()
	insertTenant := func(name, apiKey string) {
		t.Helper()
		if _, err := h.DB.Exec(
			`INSERT INTO tenants (id, name, api_key_hash, created_at) VALUES ($1, $2, $3, $4)`,
			uuid.New(),
			name,
			tenant.HashAPIKey(apiKey),
			now,
		); err != nil {
			t.Fatalf("insert tenant %s: %v", name, err)
		}
	}
	insertTenant("first", "first-secret")
	insertTenant("second", "second-secret")

	err := h.StartAPI(helpers.HarnessOptions{ExtraEnv: map[string]string{
		"GROOT_TENANCY_MODE":    "single",
		"COMMUNITY_TENANT_NAME": "Community Bootstrap",
	}, BuildEdition: "community"})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "community edition requires exactly one tenant") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestPhase22InternalEditionMultiTenant(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"GROOT_TENANCY_MODE": "multi",
	}, BuildEdition: "internal"})

	firstTenantID, _ := h.CreateTenant("internal-one")
	secondTenantID, _ := h.CreateTenant("internal-two")
	if firstTenantID == secondTenantID {
		t.Fatalf("tenant ids should differ: %s", firstTenantID)
	}
}

func TestPhase22EditionEndpoint(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"GROOT_TENANCY_MODE":    "single",
		"COMMUNITY_TENANT_NAME": "Community Bootstrap",
	}, BuildEdition: "community"})

	resp, body := h.Request(http.MethodGet, "/system/edition", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	payload := decodeBody(t, body)
	if got, want := payload["build_edition"], "community"; got != want {
		t.Fatalf("build_edition = %v, want %s", got, want)
	}
	if got, want := payload["effective_edition"], "community"; got != want {
		t.Fatalf("effective_edition = %v, want %s", got, want)
	}
	if got, want := payload["tenancy_mode"], "single"; got != want {
		t.Fatalf("tenancy_mode = %v, want %s", got, want)
	}
	capabilities, ok := payload["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing: %v", payload)
	}
	if capabilities["multi_tenant"] != false {
		t.Fatalf("multi_tenant = %v, want false", capabilities["multi_tenant"])
	}
	if capabilities["cross_tenant_admin"] != false {
		t.Fatalf("cross_tenant_admin = %v, want false", capabilities["cross_tenant_admin"])
	}
	licenseInfo, ok := payload["license"].(map[string]any)
	if !ok {
		t.Fatalf("license missing: %v", payload)
	}
	if licenseInfo["present"] != false {
		t.Fatalf("license.present = %v, want false", licenseInfo["present"])
	}
}

func TestPhase22RuntimeOverrideRejected(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	err := h.StartAPI(helpers.HarnessOptions{
		BuildEdition: "community",
		ExtraEnv: map[string]string{
			"GROOT_EDITION":         "cloud",
			"GROOT_TENANCY_MODE":    "single",
			"COMMUNITY_TENANT_NAME": "Community Bootstrap",
		},
	})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "does not match build edition") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestPhase22CommunityTenancyOverrideRejected(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	err := h.StartAPI(helpers.HarnessOptions{
		BuildEdition: "community",
		ExtraEnv: map[string]string{
			"GROOT_TENANCY_MODE":    "multi",
			"COMMUNITY_TENANT_NAME": "Community Bootstrap",
		},
	})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid edition tenancy combination") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestPhase22ValidSignedLicenseAccepted(t *testing.T) {
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	licensePath, publicKeyPath := writeSignedLicense(t, edition.LicenseClaims{
		Edition:    edition.EditionCommunity,
		Licensee:   "Acme Ltd",
		MaxTenants: 1,
		ExpiresAt:  &expiresAt,
	})
	h := helpers.NewHarness(t, helpers.HarnessOptions{
		BuildEdition: "community",
		ExtraEnv: map[string]string{
			"GROOT_TENANCY_MODE":              "single",
			"COMMUNITY_TENANT_NAME":           "Community Bootstrap",
			"GROOT_LICENSE_PATH":              licensePath,
			"GROOT_LICENSE_PUBLIC_KEY_PATH":   publicKeyPath,
			"GROOT_LICENSE_REQUIRED":          "true",
			"GROOT_LICENSE_ENFORCE_SIGNATURE": "true",
		},
	})

	resp, body := h.Request(http.MethodGet, "/system/edition", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	payload := decodeBody(t, body)
	licenseInfo := payload["license"].(map[string]any)
	if licenseInfo["present"] != true {
		t.Fatalf("license.present = %v, want true", licenseInfo["present"])
	}
	if licenseInfo["licensee"] != "Acme Ltd" {
		t.Fatalf("license.licensee = %v", licenseInfo["licensee"])
	}
	if licenseInfo["max_tenants"] != float64(1) {
		t.Fatalf("license.max_tenants = %v", licenseInfo["max_tenants"])
	}
}

func TestPhase22InvalidSignatureRejected(t *testing.T) {
	licensePath, publicKeyPath := writeSignedLicense(t, edition.LicenseClaims{
		Edition:    edition.EditionCommunity,
		MaxTenants: 1,
	})
	if err := os.WriteFile(licensePath, []byte(`{"payload":{"edition":"community","max_tenants":1},"signature":"tampered"}`), 0o644); err != nil {
		t.Fatalf("tamper license: %v", err)
	}
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	err := h.StartAPI(helpers.HarnessOptions{
		BuildEdition: "community",
		ExtraEnv: map[string]string{
			"GROOT_TENANCY_MODE":              "single",
			"COMMUNITY_TENANT_NAME":           "Community Bootstrap",
			"GROOT_LICENSE_PATH":              licensePath,
			"GROOT_LICENSE_PUBLIC_KEY_PATH":   publicKeyPath,
			"GROOT_LICENSE_REQUIRED":          "true",
			"GROOT_LICENSE_ENFORCE_SIGNATURE": "true",
		},
	})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "license signature verification failed") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestPhase22LicenseBuildMismatchRejected(t *testing.T) {
	licensePath, publicKeyPath := writeSignedLicense(t, edition.LicenseClaims{
		Edition:    edition.EditionCloud,
		MaxTenants: 1,
	})
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	err := h.StartAPI(helpers.HarnessOptions{
		BuildEdition: "community",
		ExtraEnv: map[string]string{
			"GROOT_TENANCY_MODE":            "single",
			"COMMUNITY_TENANT_NAME":         "Community Bootstrap",
			"GROOT_LICENSE_PATH":            licensePath,
			"GROOT_LICENSE_PUBLIC_KEY_PATH": publicKeyPath,
			"GROOT_LICENSE_REQUIRED":        "true",
		},
	})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "license edition cloud does not match build edition community") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}

func TestPhase22MaxTenantRestrictionEnforced(t *testing.T) {
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	licensePath, publicKeyPath := writeSignedLicense(t, edition.LicenseClaims{
		Edition:    edition.EditionInternal,
		Licensee:   "Acme Ltd",
		MaxTenants: 1,
		ExpiresAt:  &expiresAt,
	})
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	if err := h.StopAPI(); err != nil {
		t.Fatalf("stop api: %v", err)
	}
	if err := h.ResetDatabase(); err != nil {
		t.Fatalf("reset database: %v", err)
	}
	now := time.Now().UTC()
	insertTenant := func(name, apiKey string) {
		t.Helper()
		if _, err := h.DB.Exec(
			`INSERT INTO tenants (id, name, api_key_hash, created_at) VALUES ($1, $2, $3, $4)`,
			uuid.New(),
			name,
			tenant.HashAPIKey(apiKey),
			now,
		); err != nil {
			t.Fatalf("insert tenant %s: %v", name, err)
		}
	}
	insertTenant("first", "first-secret")
	insertTenant("second", "second-secret")
	err := h.StartAPI(helpers.HarnessOptions{
		BuildEdition: "internal",
		ExtraEnv: map[string]string{
			"GROOT_LICENSE_PATH":            licensePath,
			"GROOT_LICENSE_PUBLIC_KEY_PATH": publicKeyPath,
			"GROOT_LICENSE_REQUIRED":        "true",
		},
	})
	if err == nil {
		t.Fatal("StartAPI() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "tenant limit exceeded") {
		t.Fatalf("unexpected startup error: %v", err)
	}
}
