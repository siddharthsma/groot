package edition

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveRejectsRuntimeEditionMismatch(t *testing.T) {
	requested, err := Parse("cloud", "multi")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := Resolve("community", requested, LicenseConfig{}, ""); err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

func TestResolveWithSignedLicense(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	payload, err := json.Marshal(LicenseClaims{
		Edition:    EditionInternal,
		Licensee:   "Acme",
		MaxTenants: 1,
		ExpiresAt:  &expiresAt,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	signature := ed25519.Sign(privateKey, canonicalizePayload(payload))
	envelope, err := json.Marshal(LicenseEnvelope{
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(signature),
	})
	if err != nil {
		t.Fatalf("Marshal() envelope error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "license.json")
	if err := os.WriteFile(path, envelope, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	requested, err := Parse("internal", "single")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	runtime, err := Resolve("internal", requested, LicenseConfig{
		Path:             path,
		EnforceSignature: true,
	}, base64.StdEncoding.EncodeToString(publicKey))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !runtime.License.Present || !runtime.License.Valid {
		t.Fatalf("license state = %+v", runtime.License)
	}
	if runtime.MaxTenants != 1 {
		t.Fatalf("MaxTenants = %d, want 1", runtime.MaxTenants)
	}
	if runtime.TenancyMode != TenancySingle {
		t.Fatalf("TenancyMode = %s, want %s", runtime.TenancyMode, TenancySingle)
	}
	if runtime.Capabilities.MultiTenant {
		t.Fatal("MultiTenant = true, want false")
	}
}
