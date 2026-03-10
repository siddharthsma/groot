//go:build integration

package integration

import (
	"net/http"
	"testing"

	"groot/tests/helpers"
)

func TestPhase34Workflows(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	_, legacyKey := h.CreateTenant("phase34-workflows")
	actionConnectionID := createSlackConnection(t, h, legacyKey, "xoxb-phase34-workflow")
	agentID := createAgent(t, h, legacyKey, map[string]any{
		"name":           "phase34-agent",
		"instructions":   "Handle workflow nodes",
		"allowed_tools":  []string{"slack.post_message"},
		"tool_bindings":  map[string]any{},
		"memory_enabled": false,
	})
	agentVersionID := latestAgentVersionID(t, h, agentID)

	resp, body := h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name":        "Order Flow",
		"description": "Phase 34 workflow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	workflowID := mustString(t, decodeBody(t, body)["id"])

	resp, body = h.JSONRequest(http.MethodGet, "/workflows", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	definition := map[string]any{
		"nodes": []map[string]any{
			{
				"id":   "trigger-1",
				"type": "trigger",
				"position": map[string]any{
					"x": 0,
					"y": 0,
				},
				"config": map[string]any{
					"integration": "stripe",
					"event_type":  "stripe.payment_intent.succeeded.v1",
				},
			},
			{
				"id":   "agent-1",
				"type": "agent",
				"position": map[string]any{
					"x": 100,
					"y": 0,
				},
				"config": map[string]any{
					"agent_id":         agentID,
					"agent_version_id": agentVersionID,
				},
			},
			{
				"id":   "action-1",
				"type": "action",
				"position": map[string]any{
					"x": 200,
					"y": 0,
				},
				"config": map[string]any{
					"integration":   "slack",
					"connection_id": actionConnectionID,
					"operation":     "post_message",
					"inputs": map[string]any{
						"channel": "#phase34",
						"text":    "workflow action",
					},
				},
			},
			{
				"id":   "end-1",
				"type": "end",
				"position": map[string]any{
					"x": 300,
					"y": 0,
				},
				"config": map[string]any{
					"terminal_status": "succeeded",
				},
			},
		},
		"edges": []map[string]any{
			{"id": "edge-1", "source": "trigger-1", "target": "agent-1"},
			{"id": "edge-2", "source": "agent-1", "target": "action-1"},
			{"id": "edge-3", "source": "action-1", "target": "end-1"},
		},
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflows/"+workflowID+"/versions", bearerHeader(legacyKey), map[string]any{
		"definition_json": definition,
	})
	mustStatus(t, resp, body, http.StatusCreated)
	versionPayload := decodeBody(t, body)
	versionID := mustString(t, versionPayload["id"])

	resp, body = h.JSONRequest(http.MethodGet, "/workflows/"+workflowID+"/versions", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/validate", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	validatePayload := decodeBody(t, body)
	if okValue, ok := validatePayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("validate response = %s, want ok=true", body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/compile", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	compiledPayload := decodeBody(t, body)
	if okValue, ok := compiledPayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("compile response = %s, want ok=true", body)
	}
	if _, ok := compiledPayload["node_summary"].(map[string]any); !ok {
		t.Fatalf("compile response missing node_summary: %s", body)
	}
	if _, ok := compiledPayload["artifact_summary"].(map[string]any); !ok {
		t.Fatalf("compile response missing artifact_summary: %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-versions/"+versionID, bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodPut, "/workflow-versions/"+versionID, bearerHeader(legacyKey), map[string]any{
		"definition_json": map[string]any{
			"nodes": []map[string]any{
				{
					"id":       "trigger-1",
					"type":     "trigger",
					"position": map[string]any{"x": 0, "y": 0},
					"config": map[string]any{
						"integration": "stripe",
						"event_type":  "stripe.payment_intent.succeeded.v1",
					},
				},
			},
			"edges": []map[string]any{},
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
}

func latestAgentVersionID(t *testing.T, h *helpers.Harness, agentID string) string {
	t.Helper()
	row := h.DB.QueryRow(`SELECT id FROM agent_versions WHERE agent_id = $1 ORDER BY version_number DESC LIMIT 1`, agentID)
	var versionID string
	if err := row.Scan(&versionID); err != nil {
		t.Fatalf("query latest agent version: %v", err)
	}
	return versionID
}
