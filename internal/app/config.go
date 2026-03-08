package app

import (
	"net/url"
	"strings"

	"groot/internal/config"
)

type Config struct {
	Runtime                     config.Config
	BuildEdition                string
	BuildLicensePublicKeyBase64 string
}

func LoadConfig(buildEdition, buildLicensePublicKeyBase64 string) (Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return Config{}, err
	}
	return Config{
		Runtime:                     cfg,
		BuildEdition:                buildEdition,
		BuildLicensePublicKeyBase64: buildLicensePublicKeyBase64,
	}, nil
}

func deriveInternalToolEndpoint(httpAddr string) string {
	trimmed := strings.TrimSpace(httpAddr)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.TrimRight(parsed.String(), "/") + "/internal/agent-runtime/tool-calls"
		}
	}
	host := "127.0.0.1"
	port := trimmed
	if strings.HasPrefix(trimmed, ":") {
		port = strings.TrimPrefix(trimmed, ":")
	} else if strings.Contains(trimmed, ":") {
		parts := strings.Split(trimmed, ":")
		if len(parts) >= 2 {
			if parts[0] != "" && parts[0] != "0.0.0.0" && parts[0] != "::" {
				host = parts[0]
			}
			port = parts[len(parts)-1]
		}
	}
	if port == "" {
		port = "8081"
	}
	return "http://" + host + ":" + port + "/internal/agent-runtime/tool-calls"
}
