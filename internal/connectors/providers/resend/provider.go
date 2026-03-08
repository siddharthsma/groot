package resend

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
		Config:              provider.ConfigSpec{},
		Inbound: &provider.InboundSpec{
			RouteKeyStrategy: "email_token",
			EventTypes:       []string{EventTypeEmailReceived},
		},
		Operations: []provider.OperationSpec{
			{Name: OperationSendEmail, Description: "Send an email through Resend"},
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
	return New(req.Runtime.Resend, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
