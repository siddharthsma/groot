//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestScenarioReplayAndGraph(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	tenantID, legacyKey := h.CreateTenant("phase20-replay")

	resp, body := h.JSONRequest(http.MethodPost, "/functions", bearerHeader(legacyKey), map[string]any{
		"name": "phase20-fn",
		"url":  h.Mocks.FunctionServer.URL,
	})
	mustStatus(t, resp, body, http.StatusOK)
	functionPayload := decodeBody(t, body)
	functionID := mustString(t, functionPayload["id"])
	functionSecret := mustString(t, functionPayload["secret"])

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":        "function",
		"function_destination_id": functionID,
		"event_type":              "example.phase20.v1",
		"event_source":            "manual",
		"emit_success_event":      true,
	})
	mustStatus(t, resp, body, http.StatusCreated)

	resp, body = h.JSONRequest(http.MethodPost, "/events", bearerHeader(legacyKey), map[string]any{
		"type":   "example.phase20.v1",
		"source": "manual",
		"payload": map[string]any{
			"value": "graph",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	eventID := mustString(t, decodeBody(t, body)["event_id"])
	var rootDelivery map[string]any
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if latest, ok := h.LatestDeliveryForEvent(tenantID, eventID); ok {
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
	waitForHTTPRequest(t, 15*time.Second, func() bool { return len(h.Mocks.FunctionCalls()) >= 1 })

	resp, body = h.JSONRequest(http.MethodPost, "/events/"+eventID+"/replay", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.Request(http.MethodGet, "/admin/events/"+eventID+"/execution-graph", adminHeader(h.AdminKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	graphBody := decodeBody(t, body)
	if mustString(t, graphBody["event_id"]) != eventID {
		t.Fatalf("execution graph event_id mismatch")
	}
	if len(mustJSONSlice(t, graphBody["nodes"])) < 2 {
		t.Fatalf("expected execution graph nodes, body=%s", body)
	}

	resp, body = h.Request(http.MethodGet, "/admin/topology?tenant_id="+tenantID+"&include_global=false", adminHeader(h.AdminKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if len(mustJSONSlice(t, decodeBody(t, body)["nodes"])) == 0 {
		t.Fatalf("expected topology nodes")
	}

	resp, body = h.Request(http.MethodGet, "/admin/events?tenant_id="+tenantID, adminHeader(h.AdminKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if strings.Contains(jsonString(body), `"payload"`) {
		t.Fatalf("admin event response unexpectedly exposed payload: %s", body)
	}

	h.AssertNoSecrets(legacyKey, functionSecret, h.AdminKey)
}
