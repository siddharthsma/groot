package notion

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
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: provider.ConfigSpec{
			Fields: []provider.ConfigField{
				{Name: "integration_token", Required: true, Secret: true},
			},
		},
		Operations: []provider.OperationSpec{
			{Name: OperationCreatePage, Description: "Create a page in Notion"},
			{Name: OperationAppendBlock, Description: "Append blocks to a Notion block"},
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
	return New(req.Runtime.Notion.APIBaseURL, req.Runtime.Notion.APIVersion, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
