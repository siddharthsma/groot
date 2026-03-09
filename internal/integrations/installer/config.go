package installer

import (
	"os"
	"strings"
)

func ConfigFromEnv(currentVersion string) Config {
	cfg := Config{
		PluginDir:       strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_PLUGIN_DIR")),
		TrustedKeysPath: strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_TRUSTED_KEYS_PATH")),
		InstalledPath:   strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_INSTALLED_PATH")),
		CacheDir:        strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_CACHE_DIR")),
		RegistryURL:     strings.TrimSpace(os.Getenv("GROOT_INTEGRATION_REGISTRY_URL")),
		CurrentVersion:  strings.TrimSpace(currentVersion),
	}
	if cfg.PluginDir == "" {
		cfg.PluginDir = "integrations/plugins"
	}
	if cfg.TrustedKeysPath == "" {
		cfg.TrustedKeysPath = "integrations/trusted_keys.json"
	}
	if cfg.InstalledPath == "" {
		cfg.InstalledPath = "integrations/installed.json"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = "integrations/cache"
	}
	if cfg.CurrentVersion == "" {
		cfg.CurrentVersion = "dev"
	}
	return cfg
}
