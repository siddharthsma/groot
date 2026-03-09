package notion

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
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: integration.ConfigSpec{
			Fields: []integration.ConfigField{
				{Name: "integration_token", Required: true, Secret: true},
			},
		},
		Operations: []integration.OperationSpec{
			{Name: OperationCreatePage, Description: "Create a page in Notion"},
			{Name: OperationAppendBlock, Description: "Append blocks to a Notion block"},
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
	return New(req.Runtime.Notion.APIBaseURL, req.Runtime.Notion.APIVersion, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
