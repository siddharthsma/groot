package llm

import (
	"context"

	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
)

type Provider struct{}

func init() {
	registry.RegisterProvider(Provider{})
}

func (Provider) Spec() provider.ProviderSpec {
	return provider.ProviderSpec{
		Name:                ConnectorName,
		SupportsTenantScope: false,
		SupportsGlobalScope: true,
		Config: provider.ConfigSpec{
			Fields: []provider.ConfigField{
				{Name: "default_provider"},
				{Name: "providers", Required: true},
			},
		},
		Operations: []provider.OperationSpec{
			{Name: OperationGenerate, Description: "Generate text"},
			{Name: OperationSummarize, Description: "Summarize text"},
			{Name: OperationClassify, Description: "Classify text"},
			{Name: OperationExtract, Description: "Extract structured data"},
			{Name: OperationAgent, Description: "Run agent workflows"},
		},
		Schemas: Schemas(),
	}
}

func (Provider) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Provider) ExecuteOperation(ctx context.Context, req provider.OperationRequest) (provider.OperationResult, error) {
	var client HTTPClient
	if req.HTTPClient != nil {
		client = req.HTTPClient
	}
	return New(req.Runtime.LLM, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
