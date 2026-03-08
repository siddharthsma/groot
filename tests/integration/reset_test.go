//go:build integration

package integration

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"groot/tests/helpers"
)

func TestCheckpointReset(t *testing.T) {
	root := helpers.RepoRoot()
	db, err := sql.Open("pgx", "postgres://groot:groot@localhost:5432/groot?sslmode=disable")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := helpers.ResetDatabase(root, db); err != nil {
		t.Fatalf("reset database: %v", err)
	}
	_ = os.Remove(filepath.Join(root, "artifacts", "phase20_audit_report.md"))
}
