package resend

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
		Config:              integration.ConfigSpec{},
		Inbound: &integration.InboundSpec{
			RouteKeyStrategy: "email_token",
			EventTypes:       []string{EventTypeEmailReceived},
		},
		Operations: []integration.OperationSpec{
			{Name: OperationSendEmail, Description: "Send an email through Resend"},
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
	return New(req.Runtime.Resend, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
