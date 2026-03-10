//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"groot/tests/helpers"
)

func TestPhase36WorkflowRunsWaitResumeAndCancel(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"WORKFLOW_WAIT_TIMEOUT_SWEEP_INTERVAL": "500ms",
		},
	})

	tenantID, legacyKey := h.CreateTenant("phase36-workflows")
	connectionID := createSlackConnection(t, h, legacyKey, "xoxb-phase36")

	resp, body := h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name": "Phase 36 Wait Flow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	waitWorkflowID := mustString(t, decodeBody(t, body)["id"])

	waitVersionID := createWorkflowVersion(t, h, legacyKey, waitWorkflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-wait", "slack", "slack.app_mentioned.v1"),
			waitNode("wait-1", "slack", "slack.reaction.added.v1", "source.connection_id", "10m"),
			endNode("end-wait"),
		},
		"edges": []map[string]any{
			{"id": "edge-w1", "source": "trigger-wait", "target": "wait-1"},
			{"id": "edge-w2", "source": "wait-1", "target": "end-wait"},
		},
	})
	validateAndCompileWorkflowVersion(t, h, legacyKey, waitVersionID)
	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+waitVersionID+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	waitTriggerEventID := publishStructuredEvent(t, h, legacyKey, "slack.app_mentioned.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": connectionID,
	}, map[string]any{
		"user":    "U456",
		"channel": "C456",
		"text":    "@groot wait here",
		"ts":      "1710000001.000100",
	})

	waitRunID := waitForWorkflowRunStatus(t, h, waitWorkflowID, legacyKey, "waiting", 20*time.Second)

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID, bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	runPayload := decodeBody(t, body)
	if got := mustStringValue(runPayload["trigger_event_id"]); got != waitTriggerEventID {
		t.Fatalf("wait trigger_event_id = %s, want %s", got, waitTriggerEventID)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/steps", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	steps := mustJSONArray(t, body)
	assertStepStatus(t, steps, "trigger-wait", "succeeded")
	assertStepStatus(t, steps, "wait-1", "waiting")

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/waits", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	waits := mustJSONArray(t, body)
	if len(waits) != 1 {
		t.Fatalf("len(waits) = %d, want 1", len(waits))
	}
	if got := mustStringValue(waits[0]["status"]); got != "waiting" {
		t.Fatalf("wait status = %s, want waiting", got)
	}
	if got := mustStringValue(waits[0]["correlation_strategy"]); got != "source.connection_id" {
		t.Fatalf("correlation_strategy = %s, want source.connection_id", got)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflows/"+waitWorkflowID+"/runs", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	runs := mustJSONArray(t, body)
	foundRun := false
	for _, run := range runs {
		if mustStringValue(run["id"]) == waitRunID {
			foundRun = true
			break
		}
	}
	if !foundRun {
		t.Fatalf("workflow run %s not present in list", waitRunID)
	}

	resumeEventID := publishStructuredEvent(t, h, legacyKey, "slack.reaction.added.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": connectionID,
	}, map[string]any{
		"user":    "U456",
		"channel": "C123",
		"text":    ":eyes:",
		"ts":      "1710000002.000100",
	})

	waitForWorkflowRunStatus(t, h, waitWorkflowID, legacyKey, "succeeded", 20*time.Second)

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/steps", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	steps = mustJSONArray(t, body)
	assertStepStatus(t, steps, "wait-1", "succeeded")
	assertStepStatus(t, steps, "end-wait", "succeeded")

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+waitRunID+"/waits", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	waits = mustJSONArray(t, body)
	if len(waits) != 1 {
		t.Fatalf("len(waits) after resume = %d, want 1", len(waits))
	}
	if got := mustStringValue(waits[0]["status"]); got != "matched" {
		t.Fatalf("matched wait status = %s, want matched", got)
	}
	if got := mustStringValue(waits[0]["matched_event_id"]); got != resumeEventID {
		t.Fatalf("matched_event_id = %s, want %s", got, resumeEventID)
	}

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-runs/"+waitRunID+"/cancel", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusBadRequest)

	publishStructuredEvent(t, h, legacyKey, "slack.app_mentioned.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": connectionID,
	}, map[string]any{
		"user":    "U789",
		"channel": "C789",
		"text":    "@groot cancel flow",
		"ts":      "1710000002.000100",
	})

	cancelRunID := waitForWorkflowRunStatus(t, h, waitWorkflowID, legacyKey, "waiting", 20*time.Second, waitRunID)
	resp, body = h.JSONRequest(http.MethodPost, "/workflow-runs/"+cancelRunID+"/cancel", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+cancelRunID, bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	if got := mustStringValue(decodeBody(t, body)["status"]); got != "cancelled" {
		t.Fatalf("cancelled run status = %s, want cancelled", got)
	}

	resp, body = h.JSONRequest(http.MethodGet, "/workflow-runs/"+cancelRunID+"/waits", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	cancelWaits := mustJSONArray(t, body)
	if len(cancelWaits) != 1 {
		t.Fatalf("len(cancel waits) = %d, want 1", len(cancelWaits))
	}
	if got := mustStringValue(cancelWaits[0]["status"]); got != "cancelled" {
		t.Fatalf("cancel wait status = %s, want cancelled", got)
	}

	if !hasWorkflowNodeEvent(t, h, tenantID, waitRunID, "workflow.node.waiting.v1") {
		t.Fatalf("missing workflow.node.waiting.v1 for run %s", waitRunID)
	}
	if !hasWorkflowNodeEvent(t, h, tenantID, waitRunID, "workflow.node.completed.v1") {
		t.Fatalf("missing workflow.node.completed.v1 for run %s", waitRunID)
	}
}

func waitNode(id, expectedIntegration, expectedEventType, correlationStrategy, timeoutValue string) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "wait",
		"position": map[string]any{"x": 150, "y": 0},
		"config": map[string]any{
			"expected_integration": expectedIntegration,
			"expected_event_type":  expectedEventType,
			"correlation_strategy": correlationStrategy,
			"timeout":              timeoutValue,
		},
	}
}

func endNode(id string) map[string]any {
	return map[string]any{
		"id":       id,
		"type":     "end",
		"position": map[string]any{"x": 300, "y": 0},
		"config":   map[string]any{},
	}
}

func waitForWorkflowRunStatus(t *testing.T, h *helpers.Harness, workflowID, legacyKey, status string, timeout time.Duration, excludeIDs ...string) string {
	t.Helper()
	excluded := make(map[string]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		excluded[id] = struct{}{}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, body := h.JSONRequest(http.MethodGet, "/workflows/"+workflowID+"/runs", bearerHeader(legacyKey), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list workflow runs status=%d body=%s logs=\n%s", resp.StatusCode, body, h.Logs())
		}
		runs := mustJSONArray(t, body)
		for _, run := range runs {
			runID := mustStringValue(run["id"])
			if _, skip := excluded[runID]; skip {
				continue
			}
			if mustStringValue(run["status"]) == status {
				return runID
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for workflow run status %s\nlogs:\n%s", status, h.Logs())
	return ""
}

func assertStepStatus(t *testing.T, steps []map[string]any, workflowNodeID, wantStatus string) {
	t.Helper()
	for _, step := range steps {
		if mustStringValue(step["workflow_node_id"]) == workflowNodeID {
			if got := mustStringValue(step["status"]); got != wantStatus {
				t.Fatalf("step %s status = %s, want %s", workflowNodeID, got, wantStatus)
			}
			return
		}
	}
	t.Fatalf("step %s not found", workflowNodeID)
}

func waitForSlackRequest(t *testing.T, h *helpers.Harness, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(h.Mocks.SlackMessages()) > 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for slack request\nlogs:\n%s", h.Logs())
}

func hasWorkflowNodeEvent(t *testing.T, h *helpers.Harness, tenantID, workflowRunID, eventType string) bool {
	t.Helper()
	row := h.DB.QueryRow(`
		SELECT COUNT(*)
		FROM events
		WHERE tenant_id = $1
		  AND workflow_run_id = $2
		  AND type = $3
	`, tenantID, mustUUID(t, workflowRunID), eventType)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count workflow node events: %v", err)
	}
	return count > 0
}

func mustUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}
