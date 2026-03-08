package pluginloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirectoryFiltersSOFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.so"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write one.so: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write two.txt: %v", err)
	}
	files, err := scanDirectory(dir)
	if err != nil {
		t.Fatalf("scanDirectory() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("file count = %d, want 1", len(files))
	}
	if filepath.Base(files[0]) != "one.so" {
		t.Fatalf("file = %q", files[0])
	}
}
