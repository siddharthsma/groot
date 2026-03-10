//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase37WorkflowBuilderAPIs(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	_, legacyKey := h.CreateTenant("phase37-builder")
	actionConnectionID := createSlackConnection(t, h, legacyKey, "xoxb-phase37-workflow")
	agentID := createAgent(t, h, legacyKey, map[string]any{
		"name":           "phase37-agent",
		"instructions":   "Handle workflow builder actions",
		"allowed_tools":  []string{"slack.post_message"},
		"tool_bindings":  map[string]any{},
		"memory_enabled": false,
	})
	agentVersionID := latestAgentVersionID(t, h, agentID)

	resp, body := h.JSONRequest(http.MethodGet, "/workflow-builder/node-types", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	nodeTypesPayload := decodeBody(t, body)
	if _, ok := nodeTypesPayload["node_types"].([]any); !ok {
		t.Fatalf("node types response = %s, want node_types array", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/integrations/triggers", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, `"name":"slack"`) {
		t.Fatalf("trigger integrations response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/integrations/actions", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, `"name":"slack"`) || !containsJSONString(body, `"name":"post_message"`) {
		t.Fatalf("action integrations response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/connections?integration=slack&status=active", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, actionConnectionID) {
		t.Fatalf("builder connections response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/agents", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, agentID) {
		t.Fatalf("builder agents response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/agents/"+agentID+"/versions", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, agentVersionID) {
		t.Fatalf("builder agent versions response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-builder/wait-strategies", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, `"name":"source.connection_id"`) {
		t.Fatalf("wait strategies response = %s", body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name":        "Phase 37 Flow",
		"description": "builder-facing workflow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	workflowID := mustString(t, decodeBody(t, body)["id"])

	invalidVersionID := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-invalid", "slack", "slack.app_mentioned.v1"),
			{
				"id":       "action-invalid",
				"type":     "action",
				"position": map[string]any{"x": 100, "y": 0},
				"config": map[string]any{
					"integration": "slack",
					"operation":   "post_message",
					"inputs": map[string]any{
						"text": "missing connection",
					},
				},
			},
		},
		"edges": []map[string]any{
			{"id": "edge-invalid", "source": "trigger-invalid", "target": "action-invalid"},
		},
	})

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+invalidVersionID+"/validate", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	validatePayload := decodeBody(t, body)
	if okValue, ok := validatePayload["ok"].(bool); !ok || okValue {
		t.Fatalf("invalid validate response = %s, want ok=false", body)
	}
	errorsValue, ok := validatePayload["errors"].([]any)
	if !ok || len(errorsValue) == 0 {
		t.Fatalf("invalid validate response missing errors: %s", body)
	}
	firstError, ok := errorsValue[0].(map[string]any)
	if !ok {
		t.Fatalf("validation error entry is %T, want map[string]any", errorsValue[0])
	}
	if got := mustStringValue(firstError["code"]); got != "missing_action_connection" {
		t.Fatalf("validation error code = %s, want missing_action_connection", got)
	}
	if got := mustStringValue(firstError["workflow_node_id"]); got != "action-invalid" {
		t.Fatalf("validation error workflow_node_id = %s, want action-invalid", got)
	}
	if got := mustStringValue(firstError["field_path"]); got != "config.connection_id" {
		t.Fatalf("validation error field_path = %s, want config.connection_id", got)
	}
	if got := mustStringValue(firstError["severity"]); got != "error" {
		t.Fatalf("validation error severity = %s, want error", got)
	}

	versionID := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-1", "slack", "slack.app_mentioned.v1"),
			waitNode("wait-1", "slack", "slack.reaction.added.v1", "source.connection_id", "10m"),
			actionNode("action-1", "slack", actionConnectionID, "post_message", map[string]any{
				"channel": "#phase37",
				"text":    "builder workflow resumed",
			}),
			endNode("end-1"),
		},
		"edges": []map[string]any{
			{"id": "edge-1", "source": "trigger-1", "target": "wait-1"},
			{"id": "edge-2", "source": "wait-1", "target": "action-1"},
			{"id": "edge-3", "source": "action-1", "target": "end-1"},
		},
	})

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/validate", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	validatePayload = decodeBody(t, body)
	if okValue, ok := validatePayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("valid validate response = %s, want ok=true", body)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/compile", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	compilePayload := decodeBody(t, body)
	if okValue, ok := compilePayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("compile response = %s, want ok=true", body)
	}
	nodeSummary := mustJSONMap(t, compilePayload["node_summary"])
	if got := int(nodeSummary["wait"].(float64)); got != 1 {
		t.Fatalf("compile wait node summary = %d, want 1", got)
	}
	artifactSummary := mustJSONMap(t, compilePayload["artifact_summary"])
	if got := int(artifactSummary["entry_bindings"].(float64)); got != 1 {
		t.Fatalf("compile entry_bindings = %d, want 1", got)
	}
	if got := int(artifactSummary["wait_bindings"].(float64)); got != 1 {
		t.Fatalf("compile wait_bindings = %d, want 1", got)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	publishPayload := decodeBody(t, body)
	if okValue, ok := publishPayload["ok"].(bool); !ok || !okValue {
		t.Fatalf("publish response = %s, want ok=true", body)
	}
	if got := int(publishPayload["entry_bindings_activated"].(float64)); got != 1 {
		t.Fatalf("publish entry_bindings_activated = %d, want 1", got)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-versions/"+versionID+"/artifact-map", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !containsJSONString(body, `"workflow_node_id":"wait-1"`) || !containsJSONString(body, `"wait_bindings"`) {
		t.Fatalf("artifact map response = %s", body)
	}

	triggerEventID := publishStructuredEvent(t, h, legacyKey, "slack.app_mentioned.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": actionConnectionID,
	}, map[string]any{
		"user":    "U-phase37",
		"channel": "C-phase37",
		"text":    "@groot builder",
		"ts":      "1710000200.000100",
	})
	waitRunID := waitForWorkflowRunStatus(t, h, workflowID, legacyKey, "waiting", 20*time.Second)

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/steps", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	steps := mustJSONArray(t, body)
	waitStep := stepByNodeID(t, steps, "wait-1")
	if mustStringValue(waitStep["node_type"]) != "wait" {
		t.Fatalf("wait step node_type = %v, want wait", waitStep["node_type"])
	}
	if mustStringValue(waitStep["wait_id"]) == "" {
		t.Fatalf("wait step missing wait_id: %s", body)
	}
	if mustStringValue(waitStep["input_event_id"]) != triggerEventID {
		t.Fatalf("wait step input_event_id = %v, want %s", waitStep["input_event_id"], triggerEventID)
	}

	resumeEventID := publishStructuredEvent(t, h, legacyKey, "slack.reaction.added.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": actionConnectionID,
	}, map[string]any{
		"user":    "U-phase37",
		"channel": "C-phase37",
		"text":    ":white_check_mark:",
		"ts":      "1710000201.000100",
	})
	waitForHTTPRequest(t, 20*time.Second, func() bool {
		resp, body := h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/steps", bearerHeader(legacyKey), nil)
		if resp.StatusCode != http.StatusOK {
			return false
		}
		steps := mustJSONArray(t, body)
		waitStep := stepByNodeID(t, steps, "wait-1")
		return mustStringValue(waitStep["output_event_id"]) == resumeEventID
	})

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/steps", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	steps = mustJSONArray(t, body)
	if mustStringValue(stepByNodeID(t, steps, "wait-1")["output_event_id"]) != resumeEventID {
		t.Fatalf("wait step output_event_id = %v, want %s", stepByNodeID(t, steps, "wait-1")["output_event_id"], resumeEventID)
	}
}

func stepByNodeID(t *testing.T, steps []map[string]any, nodeID string) map[string]any {
	t.Helper()
	for _, step := range steps {
		if mustStringValue(step["workflow_node_id"]) == nodeID {
			return step
		}
	}
	t.Fatalf("step with workflow_node_id %s not found", nodeID)
	return nil
}

func containsJSONString(body []byte, needle string) bool {
	return string(body) != "" && containsString(string(body), needle)
}

func containsString(body string, needle string) bool {
	return len(body) > 0 && len(needle) > 0 && (body == needle || len(body) >= len(needle) && (func() bool {
		return stringIndex(body, needle) >= 0
	})())
}

func stringIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
