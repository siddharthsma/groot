package provider

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func DecodeInto(config map[string]any, target any) error {
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

func RewriteConfig(config map[string]any, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal normalized config: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(body, &normalized); err != nil {
		return fmt.Errorf("decode normalized config: %w", err)
	}
	clear(config)
	for key, value := range normalized {
		config[key] = value
	}
	return nil
}

func CanonicalSpecJSON(spec ProviderSpec) ([]byte, error) {
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal provider spec: %w", err)
	}
	return body, nil
}

func SpecHash(spec ProviderSpec) (string, error) {
	body, err := CanonicalSpecJSON(spec)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}
