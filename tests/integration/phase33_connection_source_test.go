//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase33ConnectionAwareEvents(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	tenantID, legacyKey := h.CreateTenant("phase33-source")
	slackConnectionA := createSlackConnection(t, h, legacyKey, "xoxb-phase33-a")
	slackConnectionB := createSlackConnection(t, h, legacyKey, "xoxb-phase33-b")
	llmConnectionID := createGlobalLLMConnection(t, h)

	resp, body := h.JSONRequest(http.MethodGet, "/connections", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	connections := mustJSONArray(t, body)
	if countConnections(connections, "slack") != 2 {
		t.Fatalf("expected two slack connections, body=%s", body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type": "connection",
		"operation":        "post_message",
		"operation_params": map[string]any{
			"channel": "#phase33",
			"text":    "phase33 filtered {{source.connection_id}}",
		},
		"event_type":   "example.phase33.root.v1",
		"event_source": "slack",
		"filter": map[string]any{
			"path":  "source.connection_id",
			"op":    "==",
			"value": slackConnectionB,
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s\nlogs:\n%s", resp.StatusCode, http.StatusCreated, body, h.Logs())
	}

	eventAID := publishStructuredEvent(t, h, legacyKey, "example.phase33.root.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": slackConnectionA,
	}, map[string]any{"message": "from-a"})

	time.Sleep(2 * time.Second)
	if got := countDeliveriesForEvent(t, h, eventAID); got != 0 {
		t.Fatalf("deliveries for connection A event = %d, want 0", got)
	}

	eventBID := publishStructuredEvent(t, h, legacyKey, "example.phase33.root.v1", map[string]any{
		"kind":                "external",
		"integration":         "slack",
		"connection_id":       slackConnectionB,
		"external_account_id": "T-phase33",
	}, map[string]any{"message": "from-b"})

	rootEvent := eventByType(t, h, tenantID, "example.phase33.root.v1")
	rootSource := mustJSONMap(t, rootEvent["source"])
	if mustString(t, rootSource["connection_id"]) != slackConnectionB {
		t.Fatalf("root source connection_id = %v, want %s", rootSource["connection_id"], slackConnectionB)
	}
	if mustString(t, rootSource["external_account_id"]) != "T-phase33" {
		t.Fatalf("root source external_account_id = %v", rootSource["external_account_id"])
	}
	deliveryByEventStatus(t, h, tenantID, eventBID, "succeeded")

	if len(h.Mocks.SlackMessages()) != 1 {
		t.Fatalf("slack messages = %d, want 1", len(h.Mocks.SlackMessages()))
	}

	h.Mocks.QueueLLMResponses("phase33 summary")
	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":   "connection",
		"connection_id":      llmConnectionID,
		"operation":          "generate",
		"operation_params":   map[string]any{"prompt": "Summarize {{payload.message}}", "integration": "openai"},
		"event_type":         "example.phase33.chain.v1",
		"event_source":       "slack",
		"emit_success_event": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s\nlogs:\n%s", resp.StatusCode, http.StatusCreated, body, h.Logs())
	}

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type": "connection",
		"operation":        "post_message",
		"operation_params": map[string]any{
			"channel": "#phase33",
			"text":    "phase33 chained {{payload.output.text}}",
		},
		"event_type":   "llm.generate.completed.v1",
		"event_source": "llm",
	})
	mustStatus(t, resp, body, http.StatusCreated)

	chainEventID := publishStructuredEvent(t, h, legacyKey, "example.phase33.chain.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": slackConnectionA,
	}, map[string]any{"message": "chain-root"})

	deliveryByEventStatus(t, h, tenantID, chainEventID, "succeeded")
	resultEvent := waitForEventWithLogs(t, h, tenantID, "llm.generate.completed.v1", 20*time.Second)
	resultSource := mustJSONMap(t, resultEvent["source"])
	if mustString(t, resultSource["kind"]) != "internal" {
		t.Fatalf("result source kind = %v, want internal", resultSource["kind"])
	}
	resultLineage := mustJSONMap(t, resultEvent["lineage"])
	if mustString(t, resultLineage["connection_id"]) != slackConnectionA {
		t.Fatalf("result lineage connection_id = %v, want %s", resultLineage["connection_id"], slackConnectionA)
	}

	resultEventID := mustString(t, resultEvent["event_id"])
	deliveryByEventStatus(t, h, tenantID, resultEventID, "succeeded")

	waitForHTTPRequest(t, 20*time.Second, func() bool {
		return len(h.Mocks.SlackMessages()) >= 2
	})

	resp, body = h.JSONRequest(http.MethodPost, "/events/"+eventBID+"/replay", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	deliveryByEventStatus(t, h, tenantID, eventBID, "succeeded")

	replayedRoot := eventByType(t, h, tenantID, "example.phase33.root.v1")
	replayedSource := mustJSONMap(t, replayedRoot["source"])
	if mustString(t, replayedSource["connection_id"]) != slackConnectionB {
		t.Fatalf("replayed root source connection_id = %v, want %s", replayedSource["connection_id"], slackConnectionB)
	}
}

func createSlackConnection(t *testing.T, h *helpers.Harness, legacyKey string, botToken string) string {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/connections", bearerHeader(legacyKey), map[string]any{
		"integration_name": "slack",
		"config": map[string]any{
			"bot_token":       botToken,
			"default_channel": "#phase33",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	return mustString(t, decodeBody(t, body)["id"])
}

func publishStructuredEvent(t *testing.T, h *helpers.Harness, legacyKey string, eventType string, source map[string]any, payload map[string]any) string {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/events", bearerHeader(legacyKey), map[string]any{
		"type":    eventType,
		"source":  source,
		"payload": payload,
	})
	mustStatus(t, resp, body, http.StatusOK)
	return mustString(t, decodeBody(t, body)["event_id"])
}

func countConnections(rows []map[string]any, integration string) int {
	count := 0
	for _, row := range rows {
		if mustStringValue(row["integration_name"]) == integration {
			count++
		}
	}
	return count
}

func countDeliveriesForEvent(t *testing.T, h *helpers.Harness, eventID string) int {
	t.Helper()
	row := h.DB.QueryRow(`SELECT COUNT(*) FROM delivery_jobs WHERE event_id = $1`, eventID)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	return count
}

func waitForEventWithLogs(t *testing.T, h *helpers.Harness, tenantID, eventType string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if event, ok := h.FindEvent(tenantID, eventType); ok {
			return event
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for event %s\nlogs:\n%s", eventType, h.Logs())
	return nil
}

func mustJSONArray(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode array body: %v body=%s", err, body)
	}
	return rows
}

func mustStringValue(value any) string {
	typed, _ := value.(string)
	return typed
}
