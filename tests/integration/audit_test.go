//go:build integration

package integration

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"groot/tests/helpers"
)

func TestPhase20Audit(t *testing.T) {
	removeAuditArtifact(t)

	root := helpers.RepoRoot()
	db, err := sql.Open("pgx", "postgres://groot:groot@localhost:5432/groot?sslmode=disable")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := helpers.ResetDatabase(root, db); err != nil {
		t.Fatalf("reset database: %v", err)
	}

	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	lines := []string{
		"# Phase 20 Audit Report",
		"",
	}
	migrationFiles, _ := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
	lines = append(lines, summaryLine("migrations_apply_cleanly", len(migrationFiles) > 0, "count="+fmt.Sprint(len(migrationFiles))))

	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("README missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS missing: %v", err)
	}
	lines = append(lines, summaryLine("documentation_presence", true, "README.md and AGENTS.md"))

	makefileBody, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	requiredTargets := []string{"checkpoint-fast", "checkpoint-integration", "checkpoint-reset", "checkpoint-audit"}
	for _, target := range requiredTargets {
		if !strings.Contains(string(makefileBody), target) {
			t.Fatalf("Makefile missing target %s", target)
		}
	}
	lines = append(lines, summaryLine("make_targets", true, strings.Join(requiredTargets, ", ")))

	probes := []string{
		"/healthz",
		"/readyz",
		"/schemas",
		"/tenants",
		"/events",
		"/deliveries",
		"/connectors/resend/enable",
		"/connectors/stripe/enable",
		"/webhooks/resend",
		"/webhooks/slack/events",
		"/webhooks/stripe",
		"/admin/tenants",
		"/admin/topology",
	}
	for _, path := range probes {
		status := mustPathNon404(t, h, http.MethodGet, path, nil)
		lines = append(lines, summaryLine("route "+path, true, "status="+fmt.Sprint(status)))
	}

	if err := helpers.WriteAuditReport(root, lines); err != nil {
		t.Fatalf("write report: %v", err)
	}
}
