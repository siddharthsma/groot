package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	BindingTypeConnector = "connector"
	BindingTypeFunction  = "function"
)

type Config struct {
	Instructions string                 `json:"instructions"`
	AllowedTools []string               `json:"allowed_tools"`
	MaxSteps     int                    `json:"max_steps,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
	Temperature  *float64               `json:"temperature,omitempty"`
	MaxTokens    *int                   `json:"max_tokens,omitempty"`
	ToolBindings map[string]ToolBinding `json:"tool_bindings,omitempty"`
}

type ToolBinding struct {
	Type                  string     `json:"type"`
	ConnectorName         string     `json:"connector_name,omitempty"`
	Operation             string     `json:"operation,omitempty"`
	FunctionDestinationID *uuid.UUID `json:"function_destination_id,omitempty"`
}

func ParseConfig(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode agent config: %w", err)
	}
	cfg.Instructions = strings.TrimSpace(cfg.Instructions)
	if cfg.Instructions == "" {
		return Config{}, fmt.Errorf("instructions is required")
	}
	if len(cfg.AllowedTools) == 0 {
		return Config{}, fmt.Errorf("allowed_tools is required")
	}
	normalizedAllowed := make([]string, 0, len(cfg.AllowedTools))
	for _, toolName := range cfg.AllowedTools {
		trimmed := strings.TrimSpace(toolName)
		if trimmed == "" {
			return Config{}, fmt.Errorf("allowed_tools contains empty value")
		}
		normalizedAllowed = append(normalizedAllowed, trimmed)
	}
	cfg.AllowedTools = normalizedAllowed
	if cfg.ToolBindings == nil {
		cfg.ToolBindings = map[string]ToolBinding{}
	}
	normalizedBindings := make(map[string]ToolBinding, len(cfg.ToolBindings))
	for name, binding := range cfg.ToolBindings {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return Config{}, fmt.Errorf("tool_bindings contains empty key")
		}
		binding.Type = strings.TrimSpace(binding.Type)
		binding.ConnectorName = strings.TrimSpace(binding.ConnectorName)
		binding.Operation = strings.TrimSpace(binding.Operation)
		switch binding.Type {
		case BindingTypeConnector:
			if binding.ConnectorName == "" || binding.Operation == "" {
				return Config{}, fmt.Errorf("connector tool binding requires connector_name and operation")
			}
		case BindingTypeFunction:
			if binding.FunctionDestinationID == nil {
				return Config{}, fmt.Errorf("function tool binding requires function_destination_id")
			}
		default:
			return Config{}, fmt.Errorf("invalid tool binding type")
		}
		normalizedBindings[trimmedName] = binding
	}
	cfg.ToolBindings = normalizedBindings
	return cfg, nil
}
