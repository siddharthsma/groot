package pluginloader

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func scanDirectory(dir string) ([]string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return nil, nil
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat plugin directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugin directory is not a directory")
	}
	entries, err := os.ReadDir(trimmed)
	if err != nil {
		return nil, fmt.Errorf("read plugin directory: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".so" {
			continue
		}
		out = append(out, filepath.Join(trimmed, entry.Name()))
	}
	sort.Strings(out)
	return out, nil
}
