package installer

import (
	"encoding/json"
	"time"
)

type Config struct {
	PluginDir       string
	TrustedKeysPath string
	InstalledPath   string
	CacheDir        string
	RegistryURL     string
	CurrentVersion  string
}

type Manifest struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	Description      string `json:"description"`
	Author           string `json:"author"`
	GrootVersion     string `json:"groot_version"`
	ProviderSpecHash string `json:"provider_spec_hash"`
	BuildOS          string `json:"build_os"`
	BuildArch        string `json:"build_arch"`
}

type TrustedKeysFile struct {
	TrustedPublishers []TrustedPublisher `json:"trusted_publishers"`
}

type TrustedPublisher struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

type InstalledFile struct {
	Providers []InstalledProvider `json:"providers"`
}

type InstalledProvider struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Publisher   string    `json:"publisher"`
	Description string    `json:"description,omitempty"`
	Author      string    `json:"author,omitempty"`
	PluginPath  string    `json:"plugin_path"`
	PackagePath string    `json:"package_path"`
	InstalledAt time.Time `json:"installed_at"`
}

type RegistryIndex struct {
	Providers []RegistryProvider `json:"providers"`
}

type RegistryProvider struct {
	Name     string            `json:"name"`
	Versions []RegistryVersion `json:"versions"`
}

type RegistryVersion struct {
	Version    string `json:"version"`
	PackageURL string `json:"package_url"`
	Checksum   string `json:"checksum"`
}

type PackageContents struct {
	PluginBytes    []byte
	Manifest       Manifest
	ManifestBytes  []byte
	SignatureBytes []byte
}

type InstallResult struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Publisher   string `json:"publisher"`
	PluginPath  string `json:"plugin_path"`
	PackagePath string `json:"package_path"`
}

func (f InstalledFile) Marshal() ([]byte, error) {
	return json.MarshalIndent(f, "", "  ")
}
