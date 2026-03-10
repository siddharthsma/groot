package helpers

import (
	"os"
	"path/filepath"
	"strings"
)

func WriteAuditReport(root string, lines []string) error {
	return WriteNamedAuditReport(root, "phase20_audit_report.md", lines)
}

func WriteNamedAuditReport(root, filename string, lines []string) error {
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0o755); err != nil {
		return err
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(root, "artifacts", filename), []byte(content), 0o644)
}
