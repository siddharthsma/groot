package pluginloader

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sdkprovider "groot/sdk/provider"

	"groot/internal/connectors/outbound"
	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
	"groot/internal/event"
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
	Register(string, provider.Provider, string, string) error
}

type pluginOpener interface {
	Open(string) (provider.Provider, error)
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
	loaded := make([]loadedProvider, 0, len(files))
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
		loaded = append(loaded, loadedProvider{
			path:     file,
			provider: registered,
		})
		if l.logger != nil {
			l.logger.Info("provider_plugin_loaded",
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
	for _, loadedProvider := range loaded {
		out = append(out, Result{
			Name: loadedProvider.provider.Spec().Name,
			Path: loadedProvider.path,
		})
	}
	return out, nil
}

type loadedProvider struct {
	path     string
	provider provider.Provider
}

func pluginBundles(loaded []loadedProvider) []schema.Bundle {
	bundles := make([]schema.Bundle, 0, len(loaded))
	for _, entry := range loaded {
		spec := entry.provider.Spec()
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

func (registryAdapter) Register(path string, p provider.Provider, version string, publisher string) error {
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
	plugin sdkprovider.Provider
}

func (a sdkAdapter) Spec() provider.ProviderSpec {
	spec := a.plugin.Spec()
	return provider.ProviderSpec{
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

func (a sdkAdapter) ExecuteOperation(ctx context.Context, req provider.OperationRequest) (provider.OperationResult, error) {
	result, err := a.plugin.ExecuteOperation(ctx, sdkprovider.OperationRequest{
		Operation:  req.Operation,
		Config:     req.Config,
		Params:     req.Params,
		Event:      adaptEvent(req.Event),
		HTTPClient: req.HTTPClient,
		Runtime:    adaptRuntimeConfig(req.Runtime),
	})
	if err != nil {
		return provider.OperationResult{}, err
	}
	return provider.OperationResult{
		ExternalID: result.ExternalID,
		StatusCode: result.StatusCode,
		Channel:    result.Channel,
		Text:       result.Text,
		Output:     result.Output,
		Provider:   result.Provider,
		Model:      result.Model,
		Usage: outbound.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
	}, nil
}

func adaptConfigSpec(spec sdkprovider.ConfigSpec) provider.ConfigSpec {
	fields := make([]provider.ConfigField, 0, len(spec.Fields))
	for _, field := range spec.Fields {
		fields = append(fields, provider.ConfigField{
			Name:     field.Name,
			Required: field.Required,
			Secret:   field.Secret,
		})
	}
	return provider.ConfigSpec{Fields: fields}
}

func adaptInboundSpec(spec *sdkprovider.InboundSpec) *provider.InboundSpec {
	if spec == nil {
		return nil
	}
	return &provider.InboundSpec{
		RouteKeyStrategy: spec.RouteKeyStrategy,
		EventTypes:       append([]string(nil), spec.EventTypes...),
	}
}

func adaptOperations(specs []sdkprovider.OperationSpec) []provider.OperationSpec {
	out := make([]provider.OperationSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, provider.OperationSpec{Name: spec.Name, Description: spec.Description})
	}
	return out
}

func adaptSchemas(specs []sdkprovider.SchemaSpec) []provider.SchemaSpec {
	out := make([]provider.SchemaSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, provider.SchemaSpec{
			EventType:  spec.EventType,
			Version:    spec.Version,
			SourceKind: spec.SourceKind,
			SchemaJSON: spec.SchemaJSON,
		})
	}
	return out
}

func adaptEvent(eventValue event.Event) sdkprovider.Event {
	return sdkprovider.Event{
		EventID:    eventValue.EventID.String(),
		TenantID:   eventValue.TenantID.String(),
		Type:       eventValue.Type,
		Source:     eventValue.Source,
		SourceKind: eventValue.SourceKind,
		ChainDepth: eventValue.ChainDepth,
		Timestamp:  eventValue.Timestamp,
		Payload:    eventValue.Payload,
	}
}

func adaptRuntimeConfig(cfg provider.RuntimeConfig) sdkprovider.RuntimeConfig {
	return sdkprovider.RuntimeConfig{
		Slack: sdkprovider.SlackRuntimeConfig{
			APIBaseURL:    cfg.Slack.APIBaseURL,
			SigningSecret: cfg.Slack.SigningSecret,
		},
		Resend: sdkprovider.ResendRuntimeConfig{
			APIKey:           cfg.Resend.APIKey,
			APIBaseURL:       cfg.Resend.APIBaseURL,
			WebhookPublicURL: cfg.Resend.WebhookPublicURL,
			ReceivingDomain:  cfg.Resend.ReceivingDomain,
			WebhookEvents:    append([]string(nil), cfg.Resend.WebhookEvents...),
		},
		Notion: sdkprovider.NotionRuntimeConfig{
			APIBaseURL: cfg.Notion.APIBaseURL,
			APIVersion: cfg.Notion.APIVersion,
		},
		LLM: sdkprovider.LLMRuntimeConfig{
			OpenAIAPIKey:         cfg.LLM.OpenAIAPIKey,
			OpenAIAPIBaseURL:     cfg.LLM.OpenAIAPIBaseURL,
			AnthropicAPIKey:      cfg.LLM.AnthropicAPIKey,
			AnthropicAPIBaseURL:  cfg.LLM.AnthropicAPIBaseURL,
			DefaultProvider:      cfg.LLM.DefaultProvider,
			DefaultClassifyModel: cfg.LLM.DefaultClassifyModel,
			DefaultExtractModel:  cfg.LLM.DefaultExtractModel,
			TimeoutSeconds:       cfg.LLM.TimeoutSeconds,
		},
	}
}
