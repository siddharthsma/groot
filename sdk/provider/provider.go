package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ProviderSpec struct {
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

type Event struct {
	EventID    string          `json:"event_id"`
	TenantID   string          `json:"tenant_id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	SourceKind string          `json:"source_kind"`
	ChainDepth int             `json:"chain_depth"`
	Timestamp  time.Time       `json:"timestamp"`
	Payload    json.RawMessage `json:"payload"`
}

type SlackRuntimeConfig struct {
	APIBaseURL    string
	SigningSecret string
}

type ResendRuntimeConfig struct {
	APIKey           string
	APIBaseURL       string
	WebhookPublicURL string
	ReceivingDomain  string
	WebhookEvents    []string
}

type NotionRuntimeConfig struct {
	APIBaseURL string
	APIVersion string
}

type LLMRuntimeConfig struct {
	OpenAIAPIKey         string
	OpenAIAPIBaseURL     string
	AnthropicAPIKey      string
	AnthropicAPIBaseURL  string
	DefaultProvider      string
	DefaultClassifyModel string
	DefaultExtractModel  string
	TimeoutSeconds       int
}

type RuntimeConfig struct {
	Slack  SlackRuntimeConfig
	Resend ResendRuntimeConfig
	Notion NotionRuntimeConfig
	LLM    LLMRuntimeConfig
}

type OperationRequest struct {
	Operation  string
	Config     map[string]any
	Params     json.RawMessage
	Event      Event
	HTTPClient *http.Client
	Runtime    RuntimeConfig
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type OperationResult struct {
	ExternalID string          `json:"external_id,omitempty"`
	StatusCode int             `json:"status_code,omitempty"`
	Channel    string          `json:"channel,omitempty"`
	Text       string          `json:"text,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Usage      Usage           `json:"usage,omitempty"`
}

type Provider interface {
	Spec() ProviderSpec
	ValidateConfig(config map[string]any) error
	ExecuteOperation(context.Context, OperationRequest) (OperationResult, error)
}

func ValidateSpec(spec ProviderSpec) error {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return errors.New("provider name is required")
	}
	if !spec.SupportsTenantScope && !spec.SupportsGlobalScope {
		return fmt.Errorf("provider %s must support at least one scope", name)
	}
	if err := ValidateConfigFields(name, spec.Config.Fields); err != nil {
		return err
	}
	if spec.Inbound != nil {
		if strings.TrimSpace(spec.Inbound.RouteKeyStrategy) == "" {
			return fmt.Errorf("provider %s inbound route key strategy is required", name)
		}
		if len(spec.Inbound.EventTypes) == 0 {
			return fmt.Errorf("provider %s inbound event types are required", name)
		}
	}
	operations := make(map[string]struct{}, len(spec.Operations))
	for _, op := range spec.Operations {
		opName := strings.TrimSpace(op.Name)
		if opName == "" {
			return fmt.Errorf("provider %s has empty operation name", name)
		}
		if _, exists := operations[opName]; exists {
			return fmt.Errorf("provider %s has duplicate operation %s", name, opName)
		}
		operations[opName] = struct{}{}
	}
	schemas := make(map[string]struct{}, len(spec.Schemas))
	for _, declared := range spec.Schemas {
		if strings.TrimSpace(declared.EventType) == "" || declared.Version <= 0 {
			return fmt.Errorf("provider %s has invalid schema declaration", name)
		}
		if strings.TrimSpace(declared.SourceKind) == "" {
			return fmt.Errorf("provider %s schema %s missing source kind", name, declared.EventType)
		}
		if len(declared.SchemaJSON) == 0 || !json.Valid(declared.SchemaJSON) {
			return fmt.Errorf("provider %s schema %s has invalid schema json", name, declared.EventType)
		}
		key := FullSchemaName(declared.EventType, declared.Version)
		if _, exists := schemas[key]; exists {
			return fmt.Errorf("provider %s has duplicate schema %s", name, key)
		}
		schemas[key] = struct{}{}
	}
	return nil
}

func FullSchemaName(eventType string, version int) string {
	return strings.TrimSpace(eventType) + fmt.Sprintf(".v%d", version)
}

func ValidateConfigFields(providerName string, fields []ConfigField) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return fmt.Errorf("provider %s has empty config field name", providerName)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("provider %s has duplicate config field %s", providerName, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
