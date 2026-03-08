//go:build integration

package integration

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"groot/internal/edition"
	"groot/tests/helpers"
)

func bearerHeader(token string) http.Header {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+token)
	return header
}

func adminHeader(key string) http.Header {
	header := make(http.Header)
	header.Set("X-Admin-Key", key)
	return header
}

func apiKeyHeader(key string) http.Header {
	header := make(http.Header)
	header.Set("X-API-Key", key)
	return header
}

func authBearer(token string) http.Header {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+token)
	return header
}

func doJSONRequest(t *testing.T, method, url string, headers http.Header, body any) (*http.Response, []byte) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, responseBody
}

func mustStatus(t *testing.T, resp *http.Response, body []byte, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d, body=%s", resp.StatusCode, want, body)
	}
}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v body=%s", err, body)
	}
	return payload
}

func createRealAPIKey(t *testing.T, h *helpers.Harness, legacyKey string, name string) string {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/api-keys", bearerHeader(legacyKey), map[string]any{"name": name})
	mustStatus(t, resp, body, http.StatusCreated)
	return decodeBody(t, body)["api_key"].(string)
}

func createGlobalLLMConnector(t *testing.T, h *helpers.Harness) string {
	t.Helper()
	connectorID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	resp, body := h.JSONRequest(http.MethodPut, "/admin/connector-instances/"+connectorID, adminHeader(h.AdminKey), map[string]any{
		"connector_name": "llm",
		"scope":          "global",
		"config": map[string]any{
			"default_provider": "openai",
			"providers": map[string]any{
				"openai": map[string]any{"api_key": "env:OPENAI_API_KEY"},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create global llm connector status=%d body=%s logs=\n%s", resp.StatusCode, body, h.Logs())
	}
	return connectorID
}

func createAgent(t *testing.T, h *helpers.Harness, legacyKey string, request map[string]any) string {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/agents", bearerHeader(legacyKey), request)
	mustStatus(t, resp, body, http.StatusCreated)
	return mustString(t, decodeBody(t, body)["id"])
}

func waitForAuditEvent(t *testing.T, db *sql.DB, action string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := db.QueryRow(`SELECT action FROM audit_events WHERE action = $1 ORDER BY created_at DESC LIMIT 1`, action)
		var got string
		if err := row.Scan(&got); err == nil && got == action {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for audit event %s", action)
}

func waitForHTTPRequest(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("timed out waiting for expected HTTP call")
}

func writeSignedLicense(t *testing.T, claims edition.LicenseClaims) (licensePath string, publicKeyPath string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal() claims error = %v", err)
	}
	signature := ed25519.Sign(privateKey, canonicalizeLicensePayload(t, payload))
	envelope, err := json.Marshal(map[string]any{
		"payload":   json.RawMessage(payload),
		"signature": base64.StdEncoding.EncodeToString(signature),
	})
	if err != nil {
		t.Fatalf("Marshal() envelope error = %v", err)
	}
	dir := t.TempDir()
	licensePath = filepath.Join(dir, "license.json")
	if err := os.WriteFile(licensePath, envelope, 0o644); err != nil {
		t.Fatalf("WriteFile() license error = %v", err)
	}
	publicKeyPath = filepath.Join(dir, "license.pub")
	if err := os.WriteFile(publicKeyPath, []byte(base64.StdEncoding.EncodeToString(publicKey)), 0o644); err != nil {
		t.Fatalf("WriteFile() public key error = %v", err)
	}
	return licensePath, publicKeyPath
}

func canonicalizeLicensePayload(t *testing.T, payload []byte) []byte {
	t.Helper()
	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("Unmarshal() payload error = %v", err)
	}
	body, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("Marshal() canonical payload error = %v", err)
	}
	return body
}

func writeAuditReport(t *testing.T, lines []string) {
	t.Helper()
	if err := helpers.WriteAuditReport(helpers.RepoRoot(), lines); err != nil {
		t.Fatalf("write audit report: %v", err)
	}
}

func checkpointArtifactsRoot() string {
	return filepath.Join(helpers.RepoRoot(), "artifacts")
}

func issueJWT(t *testing.T, h *helpers.Harness, tenantID string, audience string) string {
	t.Helper()
	token, err := h.Mocks.SignedJWT(jwt.MapClaims{
		"sub":       "phase20-user",
		"tenant_id": tenantID,
		"aud":       audience,
		"iss":       "https://phase20.local/",
		"exp":       time.Now().Add(10 * time.Minute).Unix(),
		"iat":       time.Now().Add(-1 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return token
}

func bodyContainsSecret(body []byte, secrets ...string) bool {
	for _, secret := range secrets {
		if secret != "" && bytes.Contains(body, []byte(secret)) {
			return true
		}
	}
	return false
}

func removeAuditArtifact(t *testing.T) {
	t.Helper()
	_ = os.Remove(filepath.Join(checkpointArtifactsRoot(), "phase20_audit_report.md"))
}

func jsonString(body []byte) string {
	return strings.TrimSpace(string(body))
}

func eventByType(t *testing.T, h *helpers.Harness, tenantID, eventType string) map[string]any {
	t.Helper()
	return h.WaitForEvent(tenantID, eventType, 20*time.Second)
}

func deliveryByEventStatus(t *testing.T, h *helpers.Harness, tenantID, eventID, status string) map[string]any {
	t.Helper()
	return h.WaitForDeliveryStatus(tenantID, eventID, status, 20*time.Second)
}

func mustJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	typed, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any", value)
	}
	return typed
}

func mustJSONSlice(t *testing.T, value any) []any {
	t.Helper()
	typed, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want []any", value)
	}
	return typed
}

func mustString(t *testing.T, value any) string {
	t.Helper()
	typed, ok := value.(string)
	if !ok {
		t.Fatalf("value is %T, want string", value)
	}
	return typed
}

func dumpLogs(t *testing.T, h *helpers.Harness) {
	t.Helper()
	t.Logf("api logs:\n%s", h.Logs())
}

func mustNoSecretInBody(t *testing.T, body []byte, secrets ...string) {
	t.Helper()
	if bodyContainsSecret(body, secrets...) {
		t.Fatalf("response body contains secret: %s", body)
	}
}

func mustPathNon404(t *testing.T, h *helpers.Harness, method, path string, headers http.Header) int {
	t.Helper()
	resp, body := h.Request(method, path, headers, nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("%s %s returned 404 body=%s", method, path, body)
	}
	return resp.StatusCode
}

func summaryLine(name string, ok bool, detail string) string {
	status := "PASS"
	if !ok {
		status = "FAIL"
	}
	if detail == "" {
		return fmt.Sprintf("- %s: %s", name, status)
	}
	return fmt.Sprintf("- %s: %s (%s)", name, status, detail)
}

func bytesReader(body []byte) *bytes.Reader {
	return bytes.NewReader(body)
}
