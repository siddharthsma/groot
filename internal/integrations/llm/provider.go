package llm

import (
	"context"

	"groot/internal/integrations"
	"groot/internal/integrations/registry"
)

type Integration struct{}

func init() {
	registry.RegisterIntegration(Integration{})
}

func (Integration) Spec() integration.IntegrationSpec {
	return integration.IntegrationSpec{
		Name:                IntegrationName,
		SupportsTenantScope: false,
		SupportsGlobalScope: true,
		Config: integration.ConfigSpec{
			Fields: []integration.ConfigField{
				{Name: "default_integration"},
				{Name: "integrations", Required: true},
			},
		},
		Operations: []integration.OperationSpec{
			{Name: OperationGenerate, Description: "Generate text"},
			{Name: OperationSummarize, Description: "Summarize text"},
			{Name: OperationClassify, Description: "Classify text"},
			{Name: OperationExtract, Description: "Extract structured data"},
			{Name: OperationAgent, Description: "Run agent workflows"},
		},
		Schemas: Schemas(),
	}
}

func (Integration) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Integration) ExecuteOperation(ctx context.Context, req integration.OperationRequest) (integration.OperationResult, error) {
	var client HTTPClient
	if req.HTTPClient != nil {
		client = req.HTTPClient
	}
	return New(req.Runtime.LLM, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
