package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"groot/internal/config"
	"groot/internal/connectors/outbound"
	"groot/internal/event"
)

type IntegrationSpec struct {
	Name                string
	SupportsTenantScope bool
	SupportsGlobalScope bool
	Config              ConfigSpec
	Inbound             *InboundSpec
	Operations          []OperationSpec
	Schemas             []SchemaSpec
}

type ConfigSpec struct {
	Fields []ConfigField
}

type ConfigField struct {
	Name     string
	Required bool
	Secret   bool
}

type InboundSpec struct {
	RouteKeyStrategy string
	EventTypes       []string
}

type OperationSpec struct {
	Name        string
	Description string
}

type SchemaSpec struct {
	EventType  string
	Version    int
	SourceKind string
	SchemaJSON json.RawMessage
}

type Integration interface {
	Spec() IntegrationSpec
	ValidateConfig(config map[string]any) error
	ExecuteOperation(context.Context, OperationRequest) (OperationResult, error)
}

type OperationRequest struct {
	Operation  string
	Config     map[string]any
	Params     json.RawMessage
	Event      event.Event
	HTTPClient *http.Client
	Runtime    RuntimeConfig
}

type RuntimeConfig struct {
	Slack  config.SlackConfig
	Resend config.ResendConfig
	Notion config.NotionConfig
	LLM    config.LLMConfig
}

type OperationResult = outbound.Result

func ValidateSpec(spec IntegrationSpec) error {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return errors.New("integration name is required")
	}
	if !spec.SupportsTenantScope && !spec.SupportsGlobalScope {
		return fmt.Errorf("integration %s must support at least one scope", name)
	}
	if err := validateConfigFields(name, spec.Config.Fields); err != nil {
		return err
	}
	if spec.Inbound != nil {
		if strings.TrimSpace(spec.Inbound.RouteKeyStrategy) == "" {
			return fmt.Errorf("integration %s inbound route key strategy is required", name)
		}
		if len(spec.Inbound.EventTypes) == 0 {
			return fmt.Errorf("integration %s inbound event types are required", name)
		}
	}
	operations := make(map[string]struct{}, len(spec.Operations))
	for _, op := range spec.Operations {
		opName := strings.TrimSpace(op.Name)
		if opName == "" {
			return fmt.Errorf("integration %s has empty operation name", name)
		}
		if _, exists := operations[opName]; exists {
			return fmt.Errorf("integration %s has duplicate operation %s", name, opName)
		}
		operations[opName] = struct{}{}
	}
	schemas := make(map[string]struct{}, len(spec.Schemas))
	for _, declared := range spec.Schemas {
		if strings.TrimSpace(declared.EventType) == "" || declared.Version <= 0 {
			return fmt.Errorf("integration %s has invalid schema declaration", name)
		}
		if strings.TrimSpace(declared.SourceKind) == "" {
			return fmt.Errorf("integration %s schema %s missing source kind", name, declared.EventType)
		}
		if len(declared.SchemaJSON) == 0 || !json.Valid(declared.SchemaJSON) {
			return fmt.Errorf("integration %s schema %s has invalid schema json", name, declared.EventType)
		}
		key := FullSchemaName(declared.EventType, declared.Version)
		if _, exists := schemas[key]; exists {
			return fmt.Errorf("integration %s has duplicate schema %s", name, key)
		}
		schemas[key] = struct{}{}
	}
	return nil
}

func FullSchemaName(eventType string, version int) string {
	return strings.TrimSpace(eventType) + fmt.Sprintf(".v%d", version)
}

func validateConfigFields(integrationName string, fields []ConfigField) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return fmt.Errorf("integration %s has empty config field name", integrationName)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("integration %s has duplicate config field %s", integrationName, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
