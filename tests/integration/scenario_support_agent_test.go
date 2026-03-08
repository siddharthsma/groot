//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestScenarioSupportAgent(t *testing.T) {
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID      string   `json:"tenant_id"`
			AgentID       string   `json:"agent_id"`
			AgentRunID    string   `json:"agent_run_id"`
			SessionID     string   `json:"session_id"`
			AllowedTools  []string `json:"allowed_tools"`
			ToolEndpoint  string   `json:"tool_endpoint_url"`
			ToolAuthToken string   `json:"tool_auth_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode runtime request: %v", err)
		}
		callTool := func(tool string, args map[string]any) {
			resp, body := doJSONRequest(t, http.MethodPost, req.ToolEndpoint, authBearer(req.ToolAuthToken), map[string]any{
				"tenant_id":        req.TenantID,
				"agent_id":         req.AgentID,
				"agent_session_id": req.SessionID,
				"agent_run_id":     req.AgentRunID,
				"tool":             tool,
				"arguments":        args,
			})
			mustStatus(t, resp, body, http.StatusOK)
		}
		callTool("notion.create_page", map[string]any{
			"parent_database_id": "db_123",
			"properties": map[string]any{
				"Title": map[string]any{"title": []any{map[string]any{"text": map[string]any{"content": "Support ticket"}}}},
			},
		})
		callTool("slack.create_thread_reply", map[string]any{
			"channel":   "C123",
			"thread_ts": "1710000000.000100",
			"text":      "Ticket created",
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "succeeded",
			"output":          map[string]any{"summary": "done"},
			"session_summary": "support-thread",
			"tool_calls": []map[string]any{
				{"tool": "notion.create_page", "ok": true, "external_id": "page_123"},
				{"tool": "slack.create_thread_reply", "ok": true, "external_id": "1710000000.000200"},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer runtimeServer.Close()

	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"AGENT_RUNTIME_BASE_URL": runtimeServer.URL,
	}})

	tenantID, legacyKey := h.CreateTenant("phase20-agent")
	llmConnectorID := createGlobalLLMConnector(t, h)

	resp, body := h.JSONRequest(http.MethodPost, "/connector-instances", bearerHeader(legacyKey), map[string]any{
		"connector_name": "slack",
		"config": map[string]any{
			"bot_token":       "xoxb-phase20-agent",
			"default_channel": "C123",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	slackConnectorID := mustString(t, decodeBody(t, body)["id"])

	resp, body = h.JSONRequest(http.MethodPost, "/connector-instances", bearerHeader(legacyKey), map[string]any{
		"connector_name": "notion",
		"config": map[string]any{
			"integration_token": "secret-notion-token",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	notionConnectorID := mustString(t, decodeBody(t, body)["id"])

	resp, body = h.JSONRequest(http.MethodPost, "/routes/inbound", bearerHeader(legacyKey), map[string]any{
		"connector_name":        "slack",
		"route_key":             "T123",
		"connector_instance_id": slackConnectorID,
	})
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":      "connector",
		"connector_instance_id": llmConnectorID,
		"agent_id": createAgent(t, h, legacyKey, map[string]any{
			"name":         "support_agent",
			"instructions": "Help with support workflow",
			"allowed_tools": []string{
				"notion.create_page",
				"slack.create_thread_reply",
			},
			"tool_bindings": map[string]any{},
		}),
		"session_key_template":      "slack:thread:{{payload.channel}}:{{payload.ts}}",
		"session_create_if_missing": true,
		"operation":                 "agent",
		"operation_params":          map[string]any{},
		"event_type":                "slack.app_mentioned.v1",
		"event_source":              "slack",
		"emit_success_event":        true,
	})
	mustStatus(t, resp, body, http.StatusCreated)

	_ = notionConnectorID
	payload := map[string]any{
		"type":    "event_callback",
		"team_id": "T123",
		"event": map[string]any{
			"type":    "app_mention",
			"user":    "U123",
			"channel": "C123",
			"text":    "please create a ticket",
			"ts":      "1710000000.000100",
		},
	}
	raw, _ := json.Marshal(payload)
	resp, body = h.Request(http.MethodPost, "/webhooks/slack/events", h.Mocks.SignSlack(raw, time.Now().UTC()), bytesReader(raw))
	mustStatus(t, resp, body, http.StatusOK)

	rootEvent := eventByType(t, h, tenantID, "slack.app_mentioned.v1")
	rootEventID := mustString(t, rootEvent["event_id"])
	var rootDelivery map[string]any
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if latest, ok := h.LatestDeliveryForEvent(tenantID, rootEventID); ok {
			rootDelivery = latest
			if mustString(t, latest["status"]) == "succeeded" {
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if rootDelivery == nil || mustString(t, rootDelivery["status"]) != "succeeded" {
		t.Fatalf("root delivery did not succeed latest=%v logs=\n%s", rootDelivery, h.Logs())
	}
	var agentResult map[string]any
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if row, ok := h.FindEvent(tenantID, "llm.agent.completed.v1"); ok {
			agentResult = row
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if agentResult == nil {
		latest, _ := h.LatestDeliveryForEvent(tenantID, rootEventID)
		t.Fatalf("missing agent result event latest_delivery=%v logs=\n%s", latest, h.Logs())
	}

	waitForHTTPRequest(t, 20*time.Second, func() bool { return len(h.Mocks.NotionRequests()) >= 1 && len(h.Mocks.SlackMessages()) >= 1 })
	if got := len(h.Mocks.FunctionCalls()); got != 0 {
		t.Fatalf("unexpected function calls: %d", got)
	}
	h.AssertNoSecrets(legacyKey, "secret-notion-token", "xoxb-phase20-agent", h.AdminKey)
}
