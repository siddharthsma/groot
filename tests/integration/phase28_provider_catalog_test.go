//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"groot/tests/helpers"
)

func TestPhase28ProviderCatalog(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	resp, body := h.Request(http.MethodGet, "/providers", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)

	var summaries []map[string]any
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("unmarshal providers: %v", err)
	}
	if len(summaries) != 5 {
		t.Fatalf("provider count = %d, want 5", len(summaries))
	}

	resp, body = h.Request(http.MethodGet, "/providers/slack", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONField(body, "name", "slack") {
		t.Fatalf("body = %s", string(body))
	}

	resp, body = h.Request(http.MethodGet, "/providers/slack/operations", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsBody(body, `"name":"post_message"`) {
		t.Fatalf("body = %s", string(body))
	}

	resp, body = h.Request(http.MethodGet, "/providers/slack/schemas", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsBody(body, `"event_type":"slack.message.created"`) {
		t.Fatalf("body = %s", string(body))
	}

	resp, body = h.Request(http.MethodGet, "/providers/slack/config", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if containsBody(body, `"config"`) {
		t.Fatalf("unexpected config wrapper body = %s", string(body))
	}
	if !containsBody(body, `"name":"bot_token"`) || !containsBody(body, `"secret":true`) {
		t.Fatalf("body = %s", string(body))
	}
}

func containsBody(body []byte, needle string) bool {
	return strings.Contains(string(body), needle)
}

func containsJSONField(body []byte, key, value string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false
	}
	got, _ := decoded[key].(string)
	return got == value
}
