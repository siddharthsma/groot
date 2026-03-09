//go:build integration

package integration

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase32PostRefactorContract(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	resp, body := h.Request(http.MethodGet, "/integrations", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"name":"slack"`) {
		t.Fatalf("integrations body = %s", body)
	}

	resp, body = h.Request(http.MethodGet, "/providers", nil, nil)
	mustStatus(t, resp, body, http.StatusNotFound)

	tenantID, legacyKey := h.CreateTenant("phase32-refactor")

	resp, body = h.JSONRequest(http.MethodPost, "/connections", bearerHeader(legacyKey), map[string]any{
		"integration_name": "slack",
		"config": map[string]any{
			"bot_token":       "xoxb-test",
			"default_channel": "C123",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	connectionID := mustString(t, decodeBody(t, body)["id"])

	resp, body = h.JSONRequest(http.MethodGet, "/connections", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), connectionID) {
		t.Fatalf("connections body = %s", body)
	}

	resp, body = h.Request(http.MethodGet, "/connector-instances", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusNotFound)

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type": "connector",
		"connection_id":    connectionID,
		"operation":        "post_message",
		"operation_params": map[string]any{
			"text": "legacy should fail",
		},
		"event_type": "example.phase32.refactor.v1",
	})
	mustStatus(t, resp, body, http.StatusBadRequest)

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type": "connection",
		"connection_id":    connectionID,
		"operation":        "post_message",
		"operation_params": map[string]any{
			"text": "phase32 {{payload.message}}",
		},
		"event_type":   "example.phase32.refactor.v1",
		"event_source": "manual",
	})
	mustStatus(t, resp, body, http.StatusCreated)

	resp, body = h.JSONRequest(http.MethodPost, "/events", bearerHeader(legacyKey), map[string]any{
		"type":   "example.phase32.refactor.v1",
		"source": "manual",
		"payload": map[string]any{
			"message": "works",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	eventID := mustString(t, decodeBody(t, body)["event_id"])
	_ = waitForDeliveryOutcome(t, h, tenantID, eventID, 20*time.Second)

	waitForHTTPRequest(t, 15*time.Second, func() bool {
		return len(h.Mocks.SlackMessages()) >= 1
	})
	if got := mustString(t, h.Mocks.SlackMessages()[0]["text"]); got != "phase32 works" {
		t.Fatalf("slack text = %q", got)
	}
}

func TestPhase32CLIUsesIntegrationCommandsOnly(t *testing.T) {
	cli := buildGrootCLI(t, "1.2.3")
	paths := phase30Paths(t)
	writeTrustedKeysFile(t, paths)

	output := runCLI(t, cli, phase30Env(paths, ""), "integration", "list")
	if !strings.Contains(output, `"integrations": []`) && !strings.Contains(output, `"integrations": null`) {
		t.Fatalf("integration list output = %s", output)
	}

	errOutput := runCLIExpectFailure(t, cli, phase30Env(paths, ""), "integration", "info", "missing")
	if !strings.Contains(errOutput, "not installed") {
		t.Fatalf("integration info stderr = %s", errOutput)
	}

	errOutput = runCLIExpectFailure(t, cli, phase30Env(paths, ""), "provider", "list")
	if !strings.Contains(errOutput, "usage: groot integration") {
		t.Fatalf("provider list stderr = %s", errOutput)
	}
}

func TestPhase32OnlyNewIntegrationEnvNamesAreActive(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}

	pluginDir := t.TempDir()
	buildExamplePlugin(t, pluginDir)

	h := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"GROOT_INTEGRATION_PLUGIN_DIR": "",
			"GROOT_PROVIDER_PLUGIN_DIR":    pluginDir,
		},
	})

	resp, body := h.Request(http.MethodGet, "/integrations/example_echo_integration", nil, nil)
	mustStatus(t, resp, body, http.StatusNotFound)

	h2 := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"GROOT_INTEGRATION_PLUGIN_DIR": pluginDir,
		},
	})
	resp, body = h2.Request(http.MethodGet, "/integrations/example_echo_integration", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
}

func writeTrustedKeysFile(t *testing.T, paths integrationPaths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.trustedKeysPath), 0o755); err != nil {
		t.Fatalf("mkdir trusted keys dir: %v", err)
	}
	if err := os.WriteFile(paths.trustedKeysPath, []byte("{\"trusted_publishers\":[]}\n"), 0o644); err != nil {
		t.Fatalf("write trusted keys file: %v", err)
	}
}
