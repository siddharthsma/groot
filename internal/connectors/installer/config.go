package installer

import (
	"os"
	"strings"
)

func ConfigFromEnv(currentVersion string) Config {
	cfg := Config{
		PluginDir:       strings.TrimSpace(os.Getenv("GROOT_PROVIDER_PLUGIN_DIR")),
		TrustedKeysPath: strings.TrimSpace(os.Getenv("GROOT_PROVIDER_TRUSTED_KEYS_PATH")),
		InstalledPath:   strings.TrimSpace(os.Getenv("GROOT_PROVIDER_INSTALLED_PATH")),
		CacheDir:        strings.TrimSpace(os.Getenv("GROOT_PROVIDER_CACHE_DIR")),
		RegistryURL:     strings.TrimSpace(os.Getenv("GROOT_PROVIDER_REGISTRY_URL")),
		CurrentVersion:  strings.TrimSpace(currentVersion),
	}
	if cfg.PluginDir == "" {
		cfg.PluginDir = "providers/plugins"
	}
	if cfg.TrustedKeysPath == "" {
		cfg.TrustedKeysPath = "providers/trusted_keys.json"
	}
	if cfg.InstalledPath == "" {
		cfg.InstalledPath = "providers/installed.json"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = "providers/cache"
	}
	if cfg.CurrentVersion == "" {
		cfg.CurrentVersion = "dev"
	}
	return cfg
}
