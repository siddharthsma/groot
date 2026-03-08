package installer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"groot/internal/connectors/pluginloader"
	"groot/internal/connectors/provider"
	"groot/internal/connectors/registryclient"
)

type Service struct {
	cfg  Config
	http *http.Client
	now  func() time.Time
}

func New(cfg Config, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Service{
		cfg:  cfg,
		http: client,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) InstallFile(ctx context.Context, packagePath string) (InstallResult, error) {
	body, err := os.ReadFile(packagePath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("read package: %w", err)
	}
	return s.installPackage(ctx, body, filepath.Base(packagePath), "")
}

func (s *Service) InstallName(ctx context.Context, name string) (InstallResult, error) {
	client := registryclient.New(s.cfg.RegistryURL, s.http)
	index, err := client.FetchIndex(ctx)
	if err != nil {
		return InstallResult{}, err
	}
	record, ok := client.Find(index, name)
	if !ok {
		return InstallResult{}, fmt.Errorf("provider %s not found in registry", name)
	}
	candidates := append([]registryclient.Version(nil), record.Versions...)
	sort.Slice(candidates, func(i, j int) bool {
		return compareRegistryVersion(candidates[i].Version, candidates[j].Version) > 0
	})
	var lastErr error
	for _, candidate := range candidates {
		body, err := download(ctx, s.http, candidate.PackageURL)
		if err != nil {
			lastErr = fmt.Errorf("download %s %s: %w", record.Name, candidate.Version, err)
			continue
		}
		result, err := s.installPackage(ctx, body, filepath.Base(candidate.PackageURL), candidate.Checksum)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return InstallResult{}, lastErr
	}
	return InstallResult{}, fmt.Errorf("provider %s has no installable versions", name)
}

func (s *Service) Remove(name string) error {
	installed, err := LoadInstalled(s.cfg.InstalledPath)
	if err != nil {
		return err
	}
	record, ok := LookupInstalled(installed, name)
	if !ok {
		return fmt.Errorf("provider %s is not installed", name)
	}
	if err := os.Remove(record.PluginPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plugin binary: %w", err)
	}
	installed = RemoveInstalled(installed, name)
	if err := SaveInstalled(s.cfg.InstalledPath, installed); err != nil {
		return err
	}
	return nil
}

func (s *Service) List() ([]InstalledProvider, error) {
	installed, err := LoadInstalled(s.cfg.InstalledPath)
	if err != nil {
		return nil, err
	}
	return installed.Providers, nil
}

func (s *Service) Info(name string) (InstalledProvider, error) {
	installed, err := LoadInstalled(s.cfg.InstalledPath)
	if err != nil {
		return InstalledProvider{}, err
	}
	record, ok := LookupInstalled(installed, name)
	if !ok {
		return InstalledProvider{}, fmt.Errorf("provider %s is not installed", name)
	}
	return record, nil
}

func (s *Service) installPackage(ctx context.Context, body []byte, packageFilename string, checksum string) (InstallResult, error) {
	if err := verifyPackageChecksum(body, checksum); err != nil {
		return InstallResult{}, err
	}
	contents, err := extractPackage(body)
	if err != nil {
		return InstallResult{}, err
	}
	if err := verifyManifest(contents.Manifest, s.cfg.CurrentVersion); err != nil {
		return InstallResult{}, err
	}
	trusted, err := loadTrustedKeys(s.cfg.TrustedKeysPath)
	if err != nil {
		return InstallResult{}, err
	}
	publisher, err := verifyPublisher(contents, trusted)
	if err != nil {
		return InstallResult{}, err
	}
	tempDir, err := os.MkdirTemp("", "groot-provider-install-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	tempPluginPath := filepath.Join(tempDir, contents.Manifest.Name+".so")
	if err := os.WriteFile(tempPluginPath, contents.PluginBytes, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("write temp plugin: %w", err)
	}
	loaded, err := pluginloader.Open(tempPluginPath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("load plugin: %w", err)
	}
	if err := provider.ValidateSpec(loaded.Spec()); err != nil {
		return InstallResult{}, fmt.Errorf("validate provider spec: %w", err)
	}
	if strings.TrimSpace(loaded.Spec().Name) != strings.TrimSpace(contents.Manifest.Name) {
		return InstallResult{}, fmt.Errorf("manifest.name does not match plugin provider name")
	}
	if err := verifyProviderSpecHash(contents.Manifest, loaded.Spec()); err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(s.cfg.CacheDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.MkdirAll(s.cfg.PluginDir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create plugin dir: %w", err)
	}
	cachePath := filepath.Join(s.cfg.CacheDir, packageFilename)
	if err := os.WriteFile(cachePath, body, 0o644); err != nil {
		return InstallResult{}, fmt.Errorf("write package cache: %w", err)
	}
	pluginPath := filepath.Join(s.cfg.PluginDir, contents.Manifest.Name+".so")
	if err := os.WriteFile(pluginPath, contents.PluginBytes, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("write plugin binary: %w", err)
	}
	installed, err := LoadInstalled(s.cfg.InstalledPath)
	if err != nil {
		return InstallResult{}, err
	}
	record := InstalledProvider{
		Name:        contents.Manifest.Name,
		Version:     contents.Manifest.Version,
		Publisher:   publisher,
		Description: contents.Manifest.Description,
		Author:      contents.Manifest.Author,
		PluginPath:  pluginPath,
		PackagePath: cachePath,
		InstalledAt: s.now(),
	}
	installed = UpsertInstalled(installed, record)
	if err := SaveInstalled(s.cfg.InstalledPath, installed); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		Name:        record.Name,
		Version:     record.Version,
		Publisher:   record.Publisher,
		PluginPath:  record.PluginPath,
		PackagePath: record.PackagePath,
	}, nil
}

func compareRegistryVersion(left, right string) int {
	lv, lerr := parseSemver(left)
	rv, rerr := parseSemver(right)
	if lerr != nil || rerr != nil {
		return strings.Compare(left, right)
	}
	return compareSemver(lv, rv)
}
