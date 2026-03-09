package pluginloader

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sdkintegration "groot/sdk/integration"

	"groot/internal/connectors/outbound"
	"groot/internal/event"
	"groot/internal/integrations"
	"groot/internal/integrations/registry"
	"groot/internal/schema"
)

type SchemaRegistrar interface {
	RegisterBundles(context.Context, []schema.Bundle) error
}

type Loader struct {
	dir       string
	installed []InstalledMetadata
	logger    *slog.Logger
	schemas   SchemaRegistrar
	registry  pluginRegistry
	opener    pluginOpener
}

type InstalledMetadata struct {
	Name       string
	Version    string
	Publisher  string
	PluginPath string
}

type pluginRegistry interface {
	Register(string, integration.Integration, string, string) error
}

type pluginOpener interface {
	Open(string) (integration.Integration, error)
}

type Result struct {
	Name string
	Path string
}

func New(dir string, installed []InstalledMetadata, logger *slog.Logger, schemas SchemaRegistrar) *Loader {
	return &Loader{
		dir:       strings.TrimSpace(dir),
		installed: append([]InstalledMetadata(nil), installed...),
		logger:    logger,
		schemas:   schemas,
		registry:  registryAdapter{},
		opener:    stdlibOpener{},
	}
}

func (l *Loader) Load(ctx context.Context) ([]Result, error) {
	files, err := scanDirectory(l.dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	loaded := make([]loadedIntegration, 0, len(files))
	for _, file := range files {
		registered, err := l.opener.Open(file)
		if err != nil {
			return nil, fmt.Errorf("load plugin %s: %w", file, err)
		}
		spec := registered.Spec()
		if err := validatePluginSpec(spec); err != nil {
			return nil, fmt.Errorf("validate plugin %s: %w", file, err)
		}
		version, publisher := lookupInstalledMetadata(l.installed, spec.Name, file)
		if err := l.registry.Register(file, registered, version, publisher); err != nil {
			return nil, fmt.Errorf("register plugin %s: %w", file, err)
		}
		loaded = append(loaded, loadedIntegration{
			path:        file,
			integration: registered,
		})
		if l.logger != nil {
			l.logger.Info("integration_plugin_loaded",
				slog.String("name", spec.Name),
				slog.String("path", file),
			)
		}
	}
	if l.schemas != nil {
		bundles := pluginBundles(loaded)
		if len(bundles) > 0 {
			if err := l.schemas.RegisterBundles(ctx, bundles); err != nil {
				return nil, fmt.Errorf("register plugin schemas: %w", err)
			}
		}
	}
	out := make([]Result, 0, len(loaded))
	for _, loadedIntegration := range loaded {
		out = append(out, Result{
			Name: loadedIntegration.integration.Spec().Name,
			Path: loadedIntegration.path,
		})
	}
	return out, nil
}

type loadedIntegration struct {
	path        string
	integration integration.Integration
}

func pluginBundles(loaded []loadedIntegration) []schema.Bundle {
	bundles := make([]schema.Bundle, 0, len(loaded))
	for _, entry := range loaded {
		spec := entry.integration.Spec()
		bundle := schema.Bundle{Name: spec.Name}
		for _, declared := range spec.Schemas {
			bundle.Schemas = append(bundle.Schemas, schema.Spec{
				EventType:  strings.TrimSpace(declared.EventType),
				Version:    declared.Version,
				Source:     spec.Name,
				SourceKind: strings.TrimSpace(declared.SourceKind),
				SchemaJSON: declared.SchemaJSON,
			})
		}
		bundles = append(bundles, bundle)
	}
	return bundles
}

type registryAdapter struct{}

func (registryAdapter) Register(path string, p integration.Integration, version string, publisher string) error {
	return registry.RegisterPluginWithMetadata(path, p, version, publisher)
}

func lookupInstalledMetadata(installed []InstalledMetadata, name string, path string) (string, string) {
	for _, current := range installed {
		if strings.TrimSpace(current.Name) == strings.TrimSpace(name) {
			return strings.TrimSpace(current.Version), strings.TrimSpace(current.Publisher)
		}
		if strings.TrimSpace(current.PluginPath) != "" && strings.TrimSpace(current.PluginPath) == strings.TrimSpace(path) {
			return strings.TrimSpace(current.Version), strings.TrimSpace(current.Publisher)
		}
	}
	return "", ""
}

type sdkAdapter struct {
	plugin sdkintegration.Integration
}

func (a sdkAdapter) Spec() integration.IntegrationSpec {
	spec := a.plugin.Spec()
	return integration.IntegrationSpec{
		Name:                spec.Name,
		SupportsTenantScope: spec.SupportsTenantScope,
		SupportsGlobalScope: spec.SupportsGlobalScope,
		Config:              adaptConfigSpec(spec.Config),
		Inbound:             adaptInboundSpec(spec.Inbound),
		Operations:          adaptOperations(spec.Operations),
		Schemas:             adaptSchemas(spec.Schemas),
	}
}

func (a sdkAdapter) ValidateConfig(config map[string]any) error {
	return a.plugin.ValidateConfig(config)
}

func (a sdkAdapter) ExecuteOperation(ctx context.Context, req integration.OperationRequest) (integration.OperationResult, error) {
	result, err := a.plugin.ExecuteOperation(ctx, sdkintegration.OperationRequest{
		Operation:  req.Operation,
		Config:     req.Config,
		Params:     req.Params,
		Event:      adaptEvent(req.Event),
		HTTPClient: req.HTTPClient,
		Runtime:    adaptRuntimeConfig(req.Runtime),
	})
	if err != nil {
		return integration.OperationResult{}, err
	}
	return integration.OperationResult{
		ExternalID:  result.ExternalID,
		StatusCode:  result.StatusCode,
		Channel:     result.Channel,
		Text:        result.Text,
		Output:      result.Output,
		Integration: result.Integration,
		Model:       result.Model,
		Usage: outbound.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

func adaptConfigSpec(spec sdkintegration.ConfigSpec) integration.ConfigSpec {
	fields := make([]integration.ConfigField, 0, len(spec.Fields))
	for _, field := range spec.Fields {
		fields = append(fields, integration.ConfigField{
			Name:     field.Name,
			Required: field.Required,
			Secret:   field.Secret,
		})
	}
	return integration.ConfigSpec{Fields: fields}
}

func adaptInboundSpec(spec *sdkintegration.InboundSpec) *integration.InboundSpec {
	if spec == nil {
		return nil
	}
	return &integration.InboundSpec{
		RouteKeyStrategy: spec.RouteKeyStrategy,
		EventTypes:       append([]string(nil), spec.EventTypes...),
	}
}

func adaptOperations(specs []sdkintegration.OperationSpec) []integration.OperationSpec {
	out := make([]integration.OperationSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, integration.OperationSpec{Name: spec.Name, Description: spec.Description})
	}
	return out
}

func adaptSchemas(specs []sdkintegration.SchemaSpec) []integration.SchemaSpec {
	out := make([]integration.SchemaSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, integration.SchemaSpec{
			EventType:  spec.EventType,
			Version:    spec.Version,
			SourceKind: spec.SourceKind,
			SchemaJSON: spec.SchemaJSON,
		})
	}
	return out
}

func adaptEvent(eventValue event.Event) sdkintegration.Event {
	return sdkintegration.Event{
		EventID:    eventValue.EventID.String(),
		TenantID:   eventValue.TenantID.String(),
		Type:       eventValue.Type,
		Source:     eventValue.SourceIntegration(),
		SourceKind: eventValue.SourceKind,
		ChainDepth: eventValue.ChainDepth,
		Timestamp:  eventValue.Timestamp,
		Payload:    eventValue.Payload,
	}
}

func adaptRuntimeConfig(cfg integration.RuntimeConfig) sdkintegration.RuntimeConfig {
	return sdkintegration.RuntimeConfig{
		Slack: sdkintegration.SlackRuntimeConfig{
			APIBaseURL:    cfg.Slack.APIBaseURL,
			SigningSecret: cfg.Slack.SigningSecret,
		},
		Resend: sdkintegration.ResendRuntimeConfig{
			APIKey:           cfg.Resend.APIKey,
			APIBaseURL:       cfg.Resend.APIBaseURL,
			WebhookPublicURL: cfg.Resend.WebhookPublicURL,
			ReceivingDomain:  cfg.Resend.ReceivingDomain,
			WebhookEvents:    append([]string(nil), cfg.Resend.WebhookEvents...),
		},
		Notion: sdkintegration.NotionRuntimeConfig{
			APIBaseURL: cfg.Notion.APIBaseURL,
			APIVersion: cfg.Notion.APIVersion,
		},
		LLM: sdkintegration.LLMRuntimeConfig{
			OpenAIAPIKey:         cfg.LLM.OpenAIAPIKey,
			OpenAIAPIBaseURL:     cfg.LLM.OpenAIAPIBaseURL,
			AnthropicAPIKey:      cfg.LLM.AnthropicAPIKey,
			AnthropicAPIBaseURL:  cfg.LLM.AnthropicAPIBaseURL,
			DefaultIntegration:   cfg.LLM.DefaultIntegration,
			DefaultClassifyModel: cfg.LLM.DefaultClassifyModel,
			DefaultExtractModel:  cfg.LLM.DefaultExtractModel,
			TimeoutSeconds:       cfg.LLM.TimeoutSeconds,
		},
	}
}
