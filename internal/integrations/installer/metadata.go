package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadInstalled(path string) (InstalledFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return InstalledFile{}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstalledFile{}, nil
		}
		return InstalledFile{}, fmt.Errorf("read installed metadata: %w", err)
	}
	var installed InstalledFile
	if err := json.Unmarshal(body, &installed); err != nil {
		return InstalledFile{}, fmt.Errorf("decode installed metadata: %w", err)
	}
	sort.Slice(installed.Integrations, func(i, j int) bool {
		return installed.Integrations[i].Name < installed.Integrations[j].Name
	})
	return installed, nil
}

func SaveInstalled(path string, installed InstalledFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create installed metadata directory: %w", err)
	}
	body, err := installed.Marshal()
	if err != nil {
		return fmt.Errorf("marshal installed metadata: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write installed metadata: %w", err)
	}
	return nil
}

func UpsertInstalled(installed InstalledFile, record InstalledIntegration) InstalledFile {
	next := make([]InstalledIntegration, 0, len(installed.Integrations)+1)
	replaced := false
	for _, current := range installed.Integrations {
		if current.Name == record.Name {
			next = append(next, record)
			replaced = true
			continue
		}
		next = append(next, current)
	}
	if !replaced {
		next = append(next, record)
	}
	sort.Slice(next, func(i, j int) bool {
		return next[i].Name < next[j].Name
	})
	installed.Integrations = next
	return installed
}

func RemoveInstalled(installed InstalledFile, name string) InstalledFile {
	filtered := make([]InstalledIntegration, 0, len(installed.Integrations))
	for _, current := range installed.Integrations {
		if current.Name == name {
			continue
		}
		filtered = append(filtered, current)
	}
	installed.Integrations = filtered
	return installed
}

func LookupInstalled(installed InstalledFile, name string) (InstalledIntegration, bool) {
	for _, current := range installed.Integrations {
		if current.Name == name {
			return current, true
		}
	}
	return InstalledIntegration{}, false
}
