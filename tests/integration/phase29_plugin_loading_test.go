//go:build integration

package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase29PluginLoading(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}

	pluginDir := t.TempDir()
	buildExamplePlugin(t, pluginDir)

	h := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"GROOT_INTEGRATION_PLUGIN_DIR": pluginDir,
		},
	})

	resp, body := h.Request(http.MethodGet, "/integrations", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"name":"example_echo_integration"`) || !strings.Contains(string(body), `"source":"plugin"`) {
		t.Fatalf("body = %s", string(body))
	}

	resp, body = h.Request(http.MethodGet, "/integrations/example_echo_integration", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"source":"plugin"`) {
		t.Fatalf("body = %s", string(body))
	}

	tenantID, legacyKey := h.CreateTenant("plugin-tenant")

	resp, body = h.JSONRequest(http.MethodPost, "/connections", bearerHeader(legacyKey), map[string]any{
		"integration_name": "example_echo_integration",
		"config":           map[string]any{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing config status=%d body=%s", resp.StatusCode, body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/connections", bearerHeader(legacyKey), map[string]any{
		"integration_name": "example_echo_integration",
		"config": map[string]any{
			"prefix": "plugin:",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	var connectionResp struct {
		ConnectionID string `json:"id"`
	}
	if err := json.Unmarshal(body, &connectionResp); err != nil {
		t.Fatalf("decode connection create: %v", err)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type": "connection",
		"connection_id":    connectionResp.ConnectionID,
		"operation":        "echo",
		"operation_params": map[string]any{
			"text": "{{payload.message}}",
		},
		"event_type":   "example.event.v1",
		"event_source": "manual",
	})
	mustStatus(t, resp, body, http.StatusCreated)

	resp, body = h.JSONRequest(http.MethodPost, "/events", bearerHeader(legacyKey), map[string]any{
		"type":   "example.event.v1",
		"source": "manual",
		"payload": map[string]any{
			"message": "hello",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	var eventResp struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(body, &eventResp); err != nil {
		t.Fatalf("decode event response: %v", err)
	}
	delivery := waitForDeliveryOutcome(t, h, tenantID, eventResp.EventID, 20*time.Second)
	if got := delivery["last_status_code"]; got != 200 {
		t.Fatalf("last_status_code = %#v", got)
	}
}

func TestPhase29PluginDuplicateIntegrationRejected(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginDir := t.TempDir()
	buildInlinePlugin(t, pluginDir, "duplicate_slack", duplicateIntegrationPluginSource("slack"))
	logs := runAPIExpectFailure(t, pluginDir)
	if !strings.Contains(logs, `duplicate integration`) {
		t.Fatalf("logs = %s", logs)
	}
}

func TestPhase29PluginInvalidSymbolRejected(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginDir := t.TempDir()
	buildInlinePlugin(t, pluginDir, "bad_symbol", invalidSymbolPluginSource())
	logs := runAPIExpectFailure(t, pluginDir)
	if !strings.Contains(logs, `Integration symbol has wrong type`) {
		t.Fatalf("logs = %s", logs)
	}
}

func pluginSupported() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd":
		return true
	default:
		return false
	}
}

func buildExamplePlugin(t *testing.T, pluginDir string) {
	t.Helper()
	root := helpers.RepoRoot()
	output := filepath.Join(pluginDir, "example_echo_integration.so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", output, ".")
	cmd.Dir = filepath.Join(root, "examples", "integration-plugin")
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Run(); err != nil {
		t.Fatalf("build example plugin: %v\n%s", err, logs.String())
	}
}

func buildInlinePlugin(t *testing.T, pluginDir, name, source string) {
	t.Helper()
	root := helpers.RepoRoot()
	moduleDir := t.TempDir()
	goMod := "module phase29plugin\n\ngo 1.23.0\n\nrequire groot/sdk v0.0.0\n\nreplace groot/sdk => " + filepath.Join(root, "sdk") + "\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "integration.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("write integration.go: %v", err)
	}
	output := filepath.Join(pluginDir, name+".so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", output, ".")
	cmd.Dir = moduleDir
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Run(); err != nil {
		t.Fatalf("build inline plugin: %v\n%s", err, logs.String())
	}
}

func runAPIExpectFailure(t *testing.T, pluginDir string) string {
	t.Helper()
	root := helpers.RepoRoot()
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "groot-api")
	buildCmd := exec.Command("go", "build", "-o", binary, "./cmd/groot-api")
	buildCmd.Dir = root
	var buildLogs bytes.Buffer
	buildCmd.Stdout = &buildLogs
	buildCmd.Stderr = &buildLogs
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("build api: %v\n%s", err, buildLogs.String())
	}
	httpPort := freePort(t)
	env := []string{
		"GROOT_EDITION=internal",
		"GROOT_TENANCY_MODE=multi",
		"GROOT_HTTP_ADDR=127.0.0.1:" + httpPort,
		"GROOT_INTEGRATION_PLUGIN_DIR=" + pluginDir,
		"POSTGRES_DSN=postgres://groot:groot@localhost:5432/groot?sslmode=disable",
		"KAFKA_BROKERS=localhost:9092",
		"ROUTER_CONSUMER_GROUP=phase29-plugin-" + httpPort,
		"TEMPORAL_ADDRESS=localhost:7233",
		"TEMPORAL_NAMESPACE=default",
		"GROOT_DELIVERY_TASK_QUEUE=phase29-delivery-" + httpPort,
		"GROOT_SYSTEM_API_KEY=system-secret",
		"AUTH_MODE=api_key",
		"API_KEY_HEADER=X-API-Key",
		"TENANT_HEADER=X-Tenant-Id",
		"ACTOR_ID_HEADER=X-Actor-Id",
		"ACTOR_TYPE_HEADER=X-Actor-Type",
		"ACTOR_EMAIL_HEADER=X-Actor-Email",
		"JWT_REQUIRED_CLAIMS=sub,tenant_id",
		"JWT_TENANT_CLAIM=tenant_id",
		"JWT_CLOCK_SKEW_SECONDS=60",
		"ADMIN_MODE_ENABLED=false",
		"ADMIN_AUTH_MODE=api_key",
		"ADMIN_API_KEY=phase29-admin-secret",
		"ADMIN_ALLOW_VIEW_PAYLOADS=false",
		"ADMIN_REPLAY_ENABLED=true",
		"ADMIN_REPLAY_MAX_EVENTS=100",
		"ADMIN_RATE_LIMIT_RPS=5",
		"AUDIT_ENABLED=true",
		"GROOT_ALLOW_GLOBAL_INSTANCES=true",
		"MAX_CHAIN_DEPTH=10",
		"MAX_REPLAY_EVENTS=1000",
		"MAX_REPLAY_WINDOW_HOURS=24",
		"SCHEMA_VALIDATION_MODE=reject",
		"SCHEMA_REGISTRATION_MODE=startup",
		"SCHEMA_MAX_PAYLOAD_BYTES=262144",
		"GRAPH_MAX_NODES=5000",
		"GRAPH_MAX_EDGES=20000",
		"GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS=500",
		"GRAPH_EXECUTION_MAX_DEPTH=25",
		"GRAPH_DEFAULT_LIMIT=500",
		"STRIPE_WEBHOOK_TOLERANCE_SECONDS=300",
		"RESEND_API_KEY=test",
		"RESEND_API_BASE_URL=https://api.resend.test",
		"RESEND_WEBHOOK_PUBLIC_URL=https://example.test/webhooks/resend",
		"RESEND_RECEIVING_DOMAIN=example.resend.test",
		"RESEND_WEBHOOK_EVENTS=email.received",
		"SLACK_API_BASE_URL=https://slack.test/api",
		"SLACK_SIGNING_SECRET=secret",
		"NOTION_API_BASE_URL=https://notion.test/v1",
		"NOTION_API_VERSION=2022-06-28",
		"OPENAI_API_KEY=test",
		"OPENAI_API_BASE_URL=https://openai.test/v1",
		"ANTHROPIC_API_KEY=test",
		"ANTHROPIC_API_BASE_URL=https://anthropic.test",
		"LLM_DEFAULT_PROVIDER=openai",
		"LLM_DEFAULT_CLASSIFY_MODEL=gpt-4o-mini",
		"LLM_DEFAULT_EXTRACT_MODEL=gpt-4o-mini",
		"LLM_TIMEOUT_SECONDS=10",
		"AGENT_MAX_STEPS=6",
		"AGENT_STEP_TIMEOUT_SECONDS=15",
		"AGENT_TOTAL_TIMEOUT_SECONDS=60",
		"AGENT_MAX_TOOL_CALLS=6",
		"AGENT_MAX_TOOL_OUTPUT_BYTES=16384",
		"AGENT_RUNTIME_ENABLED=false",
		"AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090",
		"AGENT_RUNTIME_TIMEOUT_SECONDS=10",
		"AGENT_SESSION_AUTO_CREATE=true",
		"AGENT_SESSION_MAX_IDLE_DAYS=30",
		"AGENT_MEMORY_MODE=runtime_managed",
		"AGENT_MEMORY_SUMMARY_MAX_BYTES=16384",
		"AGENT_RUNTIME_SHARED_SECRET=phase29-runtime-secret",
		"DELIVERY_MAX_ATTEMPTS=3",
		"DELIVERY_INITIAL_INTERVAL=1s",
		"DELIVERY_MAX_INTERVAL=2s",
	}
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected startup failure")
	}
	return logs.String()
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
}

func waitForDeliveryOutcome(t *testing.T, h *helpers.Harness, tenantID string, eventID string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		delivery, ok := h.LatestDeliveryForEvent(tenantID, eventID)
		if ok {
			status, _ := delivery["status"].(string)
			switch status {
			case "succeeded":
				return delivery
			case "failed", "dead_letter":
				t.Fatalf("delivery failed: %#v last_error=%s\nlogs:\n%s", delivery, latestDeliveryError(t, h, delivery["id"].(string)), h.Logs())
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if delivery, ok := h.LatestDeliveryForEvent(tenantID, eventID); ok {
		t.Fatalf("timed out waiting for succeeded delivery: %#v last_error=%s\nlogs:\n%s", delivery, latestDeliveryError(t, h, delivery["id"].(string)), h.Logs())
	}
	t.Fatalf("timed out waiting for delivery row\nlogs:\n%s", h.Logs())
	return nil
}

func latestDeliveryError(t *testing.T, h *helpers.Harness, deliveryID string) string {
	t.Helper()
	row := h.DB.QueryRow(`SELECT last_error FROM delivery_jobs WHERE id = $1`, deliveryID)
	var lastError sql.NullString
	if err := row.Scan(&lastError); err != nil {
		return ""
	}
	if lastError.Valid {
		return lastError.String
	}
	return ""
}

func duplicateIntegrationPluginSource(name string) string {
	return fmt.Sprintf(`package main

import (
	"context"
	sdkintegration "groot/sdk/integration"
)

type duplicateIntegration struct{}

var Integration sdkintegration.Integration = &duplicateIntegration{}

func (duplicateIntegration) Spec() sdkintegration.IntegrationSpec {
	return sdkintegration.IntegrationSpec{
		Name: %q,
		SupportsTenantScope: true,
		Config: sdkintegration.ConfigSpec{
			Fields: []sdkintegration.ConfigField{{Name: "token"}},
		},
		Operations: []sdkintegration.OperationSpec{{Name: "echo", Description: "echo"}},
	}
}

func (duplicateIntegration) ValidateConfig(config map[string]any) error { return nil }

func (duplicateIntegration) ExecuteOperation(context.Context, sdkintegration.OperationRequest) (sdkintegration.OperationResult, error) {
	return sdkintegration.OperationResult{}, nil
}
`, name)
}

func invalidSymbolPluginSource() string {
	return `package main

var Integration = "bad"
`
}
