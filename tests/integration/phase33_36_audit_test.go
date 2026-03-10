//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestPhase33To36Audit(t *testing.T) {
	reportPath := filepath.Join(checkpointArtifactsRoot(), "phase33_36_audit_report.md")
	_ = os.Remove(reportPath)

	root := helpers.RepoRoot()
	lines := []string{
		"# Phase 33-36 Audit Report",
		"",
		"## Command Checks",
	}

	commandChecks := []struct {
		name string
		args []string
	}{
		{name: "go_build", args: []string{"go", "build", "./..."}},
		{name: "go_test", args: []string{"go", "test", "./..."}},
		{name: "go_vet", args: []string{"go", "vet", "./..."}},
		{name: "checkpoint_fast", args: []string{"make", "checkpoint-fast"}},
	}
	for _, check := range commandChecks {
		output, err := runAuditCommand(root, check.args...)
		if err != nil {
			lines = append(lines, summaryLine(check.name, false, sanitizeAuditDetail(output)))
			writeRecentAuditReport(t, root, lines)
			t.Fatalf("%s failed: %v\n%s", check.name, err, output)
		}
		lines = append(lines, summaryLine(check.name, true, "ok"))
	}

	lines = append(lines, "", "## Targeted Integration Tests")
	targetedTests := []struct {
		name    string
		pattern string
	}{
		{name: "phase32_post_refactor", pattern: "^TestPhase32PostRefactorContract$"},
		{name: "phase33_connection_source", pattern: "^TestPhase33ConnectionAwareEvents$"},
		{name: "phase34_workflows", pattern: "^TestPhase34Workflows$"},
		{name: "phase35_workflow_publish", pattern: "^(TestPhase35WorkflowPublish|TestPhase35WorkflowPublishRequiresCompiledVersion)$"},
		{name: "phase36_workflow_runs", pattern: "^TestPhase36WorkflowRunsWaitResumeAndCancel$"},
	}
	for _, check := range targetedTests {
		output, err := runAuditCommand(root, "go", "test", "-tags=integration", "-count=1", "-p", "1", "./tests/integration", "-run", check.pattern)
		if err != nil {
			lines = append(lines, summaryLine(check.name, false, sanitizeAuditDetail(output)))
			writeRecentAuditReport(t, root, lines)
			t.Fatalf("%s failed: %v\n%s", check.name, err, output)
		}
		lines = append(lines, summaryLine(check.name, true, check.pattern))
	}

	h := helpers.NewHarness(t, helpers.HarnessOptions{})

	lines = append(lines, "", "## Route Probes")
	probeTenantRoutes(t, h, &lines)

	lines = append(lines,
		"",
		"## Exact Tests Executed",
		"- TestPhase32PostRefactorContract",
		"- TestPhase33ConnectionAwareEvents",
		"- TestPhase34Workflows",
		"- TestPhase35WorkflowPublish",
		"- TestPhase35WorkflowPublishRequiresCompiledVersion",
		"- TestPhase36WorkflowRunsWaitResumeAndCancel",
		"",
		"## Residual Gaps",
		"- No frontend checks are included in this recent-phase backend audit.",
		"- This report focuses on Phase 33-36 behavior, with Phase 32 included only as a terminology and route compatibility guard.",
	)

	writeRecentAuditReport(t, root, lines)
}

func probeTenantRoutes(t *testing.T, h *helpers.Harness, lines *[]string) {
	t.Helper()
	_, legacyKey := h.CreateTenant("phase33-36-audit")

	resp, body := h.JSONRequest(http.MethodPost, "/workflows", bearerHeader(legacyKey), map[string]any{
		"name": "audit workflow",
	})
	mustStatus(t, resp, body, http.StatusCreated)
	workflowID := mustString(t, decodeBody(t, body)["id"])

	versionID := createWorkflowVersion(t, h, legacyKey, workflowID, map[string]any{
		"nodes": []map[string]any{
			triggerNode("trigger-1", "slack", "slack.app_mentioned.v1"),
			waitNode("wait-1", "slack", "slack.reaction.added.v1", "source.connection_id", "10m"),
			endNode("end-1"),
		},
		"edges": []map[string]any{
			{"id": "edge-1", "source": "trigger-1", "target": "wait-1"},
			{"id": "edge-2", "source": "wait-1", "target": "end-1"},
		},
	})
	validateAndCompileWorkflowVersion(t, h, legacyKey, versionID)

	resp, body = h.JSONRequest(http.MethodPost, "/workflow-versions/"+versionID+"/publish", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)

	connectionID := createSlackConnection(t, h, legacyKey, "xoxb-audit-phase36")
	publishStructuredEvent(t, h, legacyKey, "slack.app_mentioned.v1", map[string]any{
		"kind":          "external",
		"integration":   "slack",
		"connection_id": connectionID,
	}, map[string]any{
		"user":    "U-audit",
		"channel": "C-audit",
		"text":    "@groot audit",
		"ts":      "1710000100.000100",
	})
	runID := waitForWorkflowRunStatus(t, h, workflowID, legacyKey, "waiting", 20*time.Second)

	probes := []struct {
		name    string
		method  string
		path    string
		headers http.Header
	}{
		{name: "route_workflows", method: http.MethodGet, path: "/workflows", headers: bearerHeader(legacyKey)},
		{name: "route_publish", method: http.MethodPost, path: "/workflow-versions/" + versionID + "/publish", headers: bearerHeader(legacyKey)},
		{name: "route_workflow_artifacts", method: http.MethodGet, path: "/workflows/" + workflowID + "/artifacts", headers: bearerHeader(legacyKey)},
		{name: "route_workflow_runs", method: http.MethodGet, path: "/workflows/" + workflowID + "/runs", headers: bearerHeader(legacyKey)},
		{name: "route_workflow_run", method: http.MethodGet, path: "/workflow-runs/" + runID, headers: bearerHeader(legacyKey)},
		{name: "route_workflow_run_steps", method: http.MethodGet, path: "/workflow-runs/" + runID + "/steps", headers: bearerHeader(legacyKey)},
		{name: "route_workflow_run_waits", method: http.MethodGet, path: "/workflow-runs/" + runID + "/waits", headers: bearerHeader(legacyKey)},
		{name: "route_workflow_run_cancel", method: http.MethodPost, path: "/workflow-runs/" + runID + "/cancel", headers: bearerHeader(legacyKey)},
	}
	for _, probe := range probes {
		status := mustPathNon404(t, h, probe.method, probe.path, probe.headers)
		*lines = append(*lines, summaryLine(probe.name, true, "status="+fmt.Sprint(status)))
	}

	legacyGone := []struct {
		name string
		path string
	}{
		{name: "legacy_providers_removed", path: "/providers"},
		{name: "legacy_connector_instances_removed", path: "/connector-instances"},
	}
	for _, probe := range legacyGone {
		resp, body := h.Request(http.MethodGet, probe.path, nil, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s returned %d, want 404 body=%s", probe.path, resp.StatusCode, body)
		}
		*lines = append(*lines, summaryLine(probe.name, true, "status=404"))
	}
}

func runAuditCommand(root string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	return output, err
}

func sanitizeAuditDetail(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "no output"
	}
	trimmed = strings.ReplaceAll(trimmed, "\n", " | ")
	if len(trimmed) > 300 {
		return trimmed[:300] + "..."
	}
	return trimmed
}

func writeRecentAuditReport(t *testing.T, root string, lines []string) {
	t.Helper()
	if err := helpers.WriteNamedAuditReport(root, "phase33_36_audit_report.md", lines); err != nil {
		t.Fatalf("write recent audit report: %v", err)
	}
}
