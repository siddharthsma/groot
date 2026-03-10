//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"groot/tests/helpers"
)

func TestPhase35WorkflowPublish(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	_, legacyKey := h.CreateTenant("phase35-workflows")
	actionConnectionID := createSlackConnection(t, h, legacyKey, "xoxb-phase35-workflow")
	agentID := createAgent(t, h, legacyKey, map[string]any{
		"name":           "phase35-agent",
		"instructions":   "Handle workflow nodes",
		"allowed_tools":  []string{"slack.post_message"},
		"tool_bindings":  map[string]any{},
		"memory_enabled": false,
	})
	agentVersionID := latestAgentVersionID(t, h, agentID)

	resp, body := h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name":        "Phase 35 Flow",
		"description": "publishable workflow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	workflowID := mustString(t, decodeBody(t, body)["id"])

	versionOne := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-1", "stripe", "stripe.payment_intent.succeeded.v1"),
			conditionNode("condition-1", `{"path":"payload.amount","op":">=","value":100}`),
			actionNode("action-1", "slack", actionConnectionID, "post_message", map[string]any{
				"channel": "#phase35",
				"text":    "workflow publish v1",
			}),
			agentNode("agent-1", agentID, agentVersionID),
		},
		"edges": []map[string]any{
			{"id": "edge-1", "source": "trigger-1", "target": "condition-1"},
			{"id": "edge-2", "source": "condition-1", "target": "action-1"},
			{"id": "edge-3", "source": "trigger-1", "target": "agent-1"},
		},
	})
	validateAndCompileWorkflowVersion(t, h, legacyKey, versionOne)

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionOne+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	publishPayload := decodeBody(t, body)
	if okValue, ok := publishPayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("publish response = %s, want ok=true", body)
	}
	if got := int(publishPayload["entry_bindings_activated"].(float64)); got != 1 {
		t.Fatalf("entry_bindings_activated = %d, want 1", got)
	}
	if got := int(publishPayload["artifacts_created"].(float64)); got != 3 {
		t.Fatalf("artifacts_created = %d, want 3", got)
	}
	if got := int(publishPayload["artifacts_superseded"].(float64)); got != 0 {
		t.Fatalf("artifacts_superseded = %d, want 0", got)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-versions/"+versionOne+"/artifacts", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	versionArtifacts := decodeBody(t, body)
	subscriptions := mustJSONSlice(t, versionArtifacts["subscriptions"])
	if len(subscriptions) != 2 {
		t.Fatalf("len(subscriptions) = %d, want 2", len(subscriptions))
	}
	var workflowSubscriptionID string
	foundAgentVersion := false
	for _, item := range subscriptions {
		sub, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("subscription artifact is %T, want map[string]any", item)
		}
		workflowSubscriptionID = mustString(t, sub["subscription_id"])
		if value, ok := sub["agent_version_id"].(string); ok && value == agentVersionID {
			foundAgentVersion = true
		}
	}
	if !foundAgentVersion {
		t.Fatalf("workflow artifacts missing agent_version_id %s: %s", agentVersionID, body)
	}

	resp, body = h.JSONRequest(http.MethodPut, "/subscriptions/"+workflowSubscriptionID, bearerHeader(legacyKey), map[string]any{
		"destination_type":   "connection",
		"connection_id":      actionConnectionID,
		"operation":          "post_message",
		"operation_params":   map[string]any{"channel": "#phase35", "text": "manual update"},
		"filter":             map[string]any{},
		"event_type":         "stripe.payment_intent.succeeded.v1",
		"emit_success_event": true,
		"emit_failure_event": true,
	})
	mustStatus(t, resp, body, http.StatusBadRequest)
	if !strings.Contains(string(body), "workflow-managed subscriptions cannot be modified directly") {
		t.Fatalf("replace workflow-managed subscription = %s, want immutable error", body)
	}

	versionTwo := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-1", "stripe", "stripe.payment_intent.succeeded.v1"),
			actionNode("action-2", "slack", actionConnectionID, "post_message", map[string]any{
				"channel": "#phase35",
				"text":    "workflow publish v2",
			}),
		},
		"edges": []map[string]any{
			{"id": "edge-1", "source": "trigger-1", "target": "action-2"},
		},
	})
	validateAndCompileWorkflowVersion(t, h, legacyKey, versionTwo)

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionTwo+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	republishPayload := decodeBody(t, body)
	if okValue, ok := republishPayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("republish response = %s, want ok=true", body)
	}
	if got := int(republishPayload["artifacts_superseded"].(float64)); got < 2 {
		t.Fatalf("republish artifacts_superseded = %d, want >= 2", got)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflows/"+workflowID+"/artifacts", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	workflowArtifacts := decodeBody(t, body)
	workflowSummary, ok := workflowArtifacts["artifacts_summary"].(map[string]any)
	if !ok {
		t.Fatalf("workflow artifacts summary is %T, want map[string]any", workflowArtifacts["artifacts_summary"])
	}
	if got := int(workflowSummary["subscriptions_active"].(float64)); got != 1 {
		t.Fatalf("subscriptions_active after republish = %d, want 1", got)
	}
	if got := int(workflowSummary["subscriptions_superseded"].(float64)); got < 2 {
		t.Fatalf("subscriptions_superseded after republish = %d, want >= 2", got)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflows/"+workflowID+"/unpublish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodGet, "/workflows/"+workflowID+"/artifacts", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	finalArtifacts := decodeBody(t, body)
	finalSummary, ok := finalArtifacts["artifacts_summary"].(map[string]any)
	if !ok {
		t.Fatalf("final artifacts summary is %T, want map[string]any", finalArtifacts["artifacts_summary"])
	}
	if got := int(finalSummary["entry_bindings_active"].(float64)); got != 0 {
		t.Fatalf("entry_bindings_active after unpublish = %d, want 0", got)
	}
	if got := int(finalSummary["subscriptions_active"].(float64)); got != 0 {
		t.Fatalf("subscriptions_active after unpublish = %d, want 0", got)
	}
}

func TestPhase35WorkflowPublishRequiresCompiledVersion(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	_, legacyKey := h.CreateTenant("phase35-uncompiled")
	resp, body := h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name": "Uncompiled Flow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	workflowID := mustString(t, decodeBody(t, body)["id"])

	versionID := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-1", "stripe", "stripe.payment_intent.succeeded.v1"),
		},
		"edges": []map[string]any{},
	})

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusBadRequest)
	if !strings.Contains(string(body), "workflow version must be valid before publish") {
		t.Fatalf("publish uncompiled version = %s, want version valid error", body)
	}
}

func createWorkflowVersion(t *testing.T, h *helpers.Harness, legacyKey string, workflowID string, definition map[string]any) string {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/workflows/"+workflowID+"/versions", bearerHeader(legacyKey), map[string]any{
		"definition_json": definition,
	})
	mustStatus(t, resp, body, http.StatusCreated)
	return mustString(t, decodeBody(t, body)["id"])
}

func validateAndCompileWorkflowVersion(t *testing.T, h *helpers.Harness, legacyKey string, versionID string) {
	t.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/validate", bearerHeader(legacyKey), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate workflow version status=%d body=%s logs=\n%s", resp.StatusCode, body, h.Logs())
	}
	validatePayload := decodeBody(t, body)
	if okValue, ok := validatePayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("validate workflow version = %s, want ok=true", body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/compile", bearerHeader(legacyKey), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compile workflow version status=%d body=%s logs=\n%s", resp.StatusCode, body, h.Logs())
	}
	compilePayload := decodeBody(t, body)
	if okValue, ok := compilePayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("compile workflow version = %s, want ok=true", body)
	}
}

func triggerNode(id, integration, eventType string) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "trigger",
		"position": map[string]any{"x": 0, "y": 0},
		"config": map[string]any{
			"integration": integration,
			"event_type":  eventType,
		},
	}
}

func conditionNode(id, expression string) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "condition",
		"position": map[string]any{"x": 100, "y": 0},
		"config": map[string]any{
			"expression": expression,
		},
	}
}

func actionNode(id, integration, connectionID, operation string, inputs map[string]any) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "action",
		"position": map[string]any{"x": 200, "y": 0},
		"config": map[string]any{
			"integration":   integration,
			"connection_id": connectionID,
			"operation":     operation,
			"inputs":        inputs,
		},
	}
}

func agentNode(id, agentID, agentVersionID string) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "agent",
		"position": map[string]any{"x": 300, "y": 0},
		"config": map[string]any{
			"agent_id":             agentID,
			"agent_version_id":     agentVersionID,
			"session_key_template": "{{event_id}}",
			"session_mode":         "reuse_or_create",
		},
	}
}
