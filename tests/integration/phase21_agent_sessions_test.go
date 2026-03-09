//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase21SameSessionReuse(t *testing.T) {
	var (
		mu                sync.Mutex
		runCountBySession = map[string]int{}
	)
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode runtime request: %v", err)
		}
		mu.Lock()
		runCountBySession[req.SessionID]++
		count := runCountBySession[req.SessionID]
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":          "succeeded",
			"output":          map[string]any{"count": count},
			"session_summary": "run-count:" + mustJSONIntString(count),
			"tool_calls":      []any{},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		})
	}))
	defer runtimeServer.Close()

	h := helpers.NewHarness(t, helpers.HarnessOptions{ExtraEnv: map[string]string{
		"AGENT_RUNTIME_BASE_URL": runtimeServer.URL,
	}})
	tenantID, legacyKey := h.CreateTenant("phase21-sessions")
	llmConnectionID := createGlobalLLMConnection(t, h)
	agentID := createAgent(t, h, legacyKey, map[string]any{
		"name":          "task_chaser",
		"instructions":  "Follow up on task updates",
		"allowed_tools": []string{"slack.post_message"},
		"tool_bindings": map[string]any{},
	})

	resp, body := h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":          "connection",
		"connection_id":             llmConnectionID,
		"agent_id":                  agentID,
		"session_key_template":      "salesforce:task:{{payload.task.id}}",
		"session_create_if_missing": true,
		"operation":                 "agent",
		"operation_params":          map[string]any{},
		"event_type":                "salesforce.task.updated.v1",
		"emit_success_event":        true,
	})
	mustStatus(t, resp, body, http.StatusCreated)

	postEvent := func(taskID string) string {
		resp, body := h.JSONRequest(http.MethodPost, "/events", bearerHeader(legacyKey), map[string]any{
			"type":   "salesforce.task.updated.v1",
			"source": "salesforce",
			"payload": map[string]any{
				"task": map[string]any{"id": taskID},
			},
		})
		mustStatus(t, resp, body, http.StatusOK)
		return mustString(t, decodeBody(t, body)["event_id"])
	}

	eventID1 := postEvent("T123")
	waitForDeliverySucceeded(t, h, tenantID, eventID1)
	eventID2 := postEvent("T123")
	waitForDeliverySucceeded(t, h, tenantID, eventID2)

	resp, body = h.Request(http.MethodGet, "/agent-sessions?agent_id="+agentID, bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	var sessions []map[string]any
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatalf("decode sessions body: %v body=%s", err, body)
	}
	if len(sessions) != 1 {
		t.Fatalf("session count = %d, want 1 body=%s logs=\n%s", len(sessions), body, h.Logs())
	}
}

func waitForDeliverySucceeded(t *testing.T, h *helpers.Harness, tenantID string, eventID string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if latest, ok := h.LatestDeliveryForEvent(tenantID, eventID); ok && mustString(t, latest["status"]) == "succeeded" {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("delivery for event %s did not succeed logs=\n%s", eventID, h.Logs())
}

func mustJSONIntString(value int) string {
	body, _ := json.Marshal(value)
	return string(body)
}
